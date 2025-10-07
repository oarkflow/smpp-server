package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/oarkflow/smpp-server/pkg/smpp"
)

// DefaultUserAuth implements the UserAuth interface
type DefaultUserAuth struct {
	users    map[string]*smpp.User
	sessions map[string]*Session
	mutex    sync.RWMutex
	logger   smpp.Logger
}

// Session represents an authenticated user session
type Session struct {
	UserID      string
	SystemID    string
	CreatedAt   time.Time
	LastAccess  time.Time
	IPAddress   string
	Permissions map[smpp.Operation]bool
}

// NewDefaultUserAuth creates a new authentication provider
func NewDefaultUserAuth(logger smpp.Logger) *DefaultUserAuth {
	auth := &DefaultUserAuth{
		users:    make(map[string]*smpp.User),
		sessions: make(map[string]*Session),
		logger:   logger,
	}

	// Add default users
	auth.addDefaultUsers()

	return auth
}

// Authenticate validates user credentials and returns user info
func (a *DefaultUserAuth) Authenticate(ctx context.Context, systemID, password string) (*smpp.User, error) {
	a.mutex.RLock()
	user, exists := a.users[systemID]
	a.mutex.RUnlock()

	if !exists {
		a.logger.Warn("Authentication failed: user not found", "systemID", systemID)
		return nil, fmt.Errorf("user not found")
	}

	if !user.Active {
		a.logger.Warn("Authentication failed: user inactive", "systemID", systemID)
		return nil, fmt.Errorf("user inactive")
	}

	// Check password hash
	expectedHash := a.hashPassword(password, user.Salt)
	if expectedHash != user.PasswordHash {
		a.logger.Warn("Authentication failed: invalid password", "systemID", systemID)
		return nil, fmt.Errorf("invalid credentials")
	}

	// Update last login
	a.mutex.Lock()
	user.LastLogin = time.Now()
	user.LoginCount++
	a.mutex.Unlock()

	a.logger.Info("Authentication successful", "systemID", systemID, "userID", user.ID)
	return user, nil
}

// CreateUser creates a new user
func (a *DefaultUserAuth) CreateUser(ctx context.Context, user *smpp.User) error {
	if user.SystemID == "" {
		return fmt.Errorf("system ID cannot be empty")
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	if _, exists := a.users[user.SystemID]; exists {
		return fmt.Errorf("user already exists")
	}

	// Generate user ID if not provided
	if user.ID == "" {
		user.ID = a.generateID()
	}

	// Hash password
	if user.Password != "" {
		user.Salt = a.generateSalt()
		user.PasswordHash = a.hashPassword(user.Password, user.Salt)
		user.Password = "" // Clear plaintext password
	}

	// Set defaults
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}
	user.UpdatedAt = time.Now()

	if user.Permissions == nil {
		user.Permissions = make(map[smpp.Operation]bool)
		// Default permissions
		user.Permissions[smpp.OperationSubmit] = true
		user.Permissions[smpp.OperationBind] = true
		user.Permissions[smpp.OperationDeliver] = true
	}

	a.users[user.SystemID] = user
	a.logger.Info("User created", "systemID", user.SystemID, "userID", user.ID)

	return nil
}

// UpdateUser updates user information
func (a *DefaultUserAuth) UpdateUser(ctx context.Context, user *smpp.User) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	existing, exists := a.users[user.SystemID]
	if !exists {
		return fmt.Errorf("user not found")
	}

	// Update fields
	existing.Name = user.Name
	existing.Email = user.Email
	existing.Active = user.Active
	existing.MaxConnections = user.MaxConnections
	existing.RateLimit = user.RateLimit
	existing.UpdatedAt = time.Now()

	// Update permissions if provided
	if user.Permissions != nil {
		existing.Permissions = user.Permissions
	}

	// Update password if provided
	if user.Password != "" {
		existing.Salt = a.generateSalt()
		existing.PasswordHash = a.hashPassword(user.Password, existing.Salt)
	}

	a.logger.Info("User updated", "systemID", user.SystemID, "userID", user.ID)
	return nil
}

// DeleteUser deletes a user
func (a *DefaultUserAuth) DeleteUser(ctx context.Context, systemID string) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if _, exists := a.users[systemID]; !exists {
		return fmt.Errorf("user not found")
	}

	delete(a.users, systemID)

	// Remove all sessions for this user
	for sessionID, session := range a.sessions {
		if session.SystemID == systemID {
			delete(a.sessions, sessionID)
		}
	}

	a.logger.Info("User deleted", "systemID", systemID)
	return nil
}

// GetUser retrieves user information
func (a *DefaultUserAuth) GetUser(ctx context.Context, systemID string) (*smpp.User, error) {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	user, exists := a.users[systemID]
	if !exists {
		return nil, fmt.Errorf("user not found")
	}

	// Return a copy to prevent external modification
	userCopy := *user
	userCopy.Password = ""     // Never expose password
	userCopy.PasswordHash = "" // Never expose password hash
	userCopy.Salt = ""         // Never expose salt

	return &userCopy, nil
}

// ListUsers returns all users
func (a *DefaultUserAuth) ListUsers(ctx context.Context) ([]*smpp.User, error) {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	users := make([]*smpp.User, 0, len(a.users))
	for _, user := range a.users {
		userCopy := *user
		userCopy.Password = ""     // Never expose password
		userCopy.PasswordHash = "" // Never expose password hash
		userCopy.Salt = ""         // Never expose salt
		users = append(users, &userCopy)
	}

	return users, nil
}

// IsAuthorized checks if a user is authorized for a specific operation
func (a *DefaultUserAuth) IsAuthorized(ctx context.Context, systemID string, operation smpp.Operation) (bool, error) {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	user, exists := a.users[systemID]
	if !exists {
		return false, fmt.Errorf("user not found")
	}

	if !user.Active {
		return false, fmt.Errorf("user inactive")
	}

	authorized, exists := user.Permissions[operation]
	if !exists {
		return false, nil // Default to not authorized
	}

	return authorized, nil
}

// CreateSession creates a new user session
func (a *DefaultUserAuth) CreateSession(ctx context.Context, systemID, ipAddress string) (string, error) {
	a.mutex.RLock()
	user, exists := a.users[systemID]
	a.mutex.RUnlock()

	if !exists {
		return "", fmt.Errorf("user not found")
	}

	sessionID := a.generateID()
	session := &Session{
		UserID:      user.ID,
		SystemID:    systemID,
		CreatedAt:   time.Now(),
		LastAccess:  time.Now(),
		IPAddress:   ipAddress,
		Permissions: user.Permissions,
	}

	a.mutex.Lock()
	a.sessions[sessionID] = session
	a.mutex.Unlock()

	a.logger.Info("Session created", "systemID", systemID, "sessionID", sessionID, "ip", ipAddress)
	return sessionID, nil
}

// GetSession retrieves session information
func (a *DefaultUserAuth) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found")
	}

	// Update last access
	session.LastAccess = time.Now()

	return session, nil
}

// DeleteSession removes a user session
func (a *DefaultUserAuth) DeleteSession(ctx context.Context, sessionID string) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found")
	}

	delete(a.sessions, sessionID)
	a.logger.Info("Session deleted", "systemID", session.SystemID, "sessionID", sessionID)

	return nil
}

// CleanupExpiredSessions removes expired sessions
func (a *DefaultUserAuth) CleanupExpiredSessions(maxAge time.Duration) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	now := time.Now()
	for sessionID, session := range a.sessions {
		if now.Sub(session.LastAccess) > maxAge {
			delete(a.sessions, sessionID)
			a.logger.Info("Expired session removed", "systemID", session.SystemID, "sessionID", sessionID)
		}
	}
}

// GetActiveSessionsCount returns the number of active sessions for a user
func (a *DefaultUserAuth) GetActiveSessionsCount(systemID string) int {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	count := 0
	for _, session := range a.sessions {
		if session.SystemID == systemID {
			count++
		}
	}

	return count
}

// addDefaultUsers adds some default users for testing
func (a *DefaultUserAuth) addDefaultUsers() {
	defaultUsers := []*smpp.User{
		{
			ID:             "user1",
			SystemID:       "test",
			Password:       "test",
			Name:           "Test User",
			Email:          "test@example.com",
			Active:         true,
			MaxConnections: 5,
			RateLimit:      100, // messages per minute
			Permissions: map[smpp.Operation]bool{
				smpp.OperationBind:    true,
				smpp.OperationSubmit:  true,
				smpp.OperationDeliver: true,
				smpp.OperationQuery:   true,
			},
		},
		{
			ID:             "user2",
			SystemID:       "client1",
			Password:       "password1",
			Name:           "Client One",
			Email:          "client1@example.com",
			Active:         true,
			MaxConnections: 10,
			RateLimit:      500,
			Permissions: map[smpp.Operation]bool{
				smpp.OperationBind:    true,
				smpp.OperationSubmit:  true,
				smpp.OperationDeliver: true,
				smpp.OperationQuery:   true,
				smpp.OperationCancel:  true,
				smpp.OperationReplace: true,
			},
		},
		{
			ID:             "user3",
			SystemID:       "esme",
			Password:       "esme123",
			Name:           "ESME Client",
			Email:          "esme@example.com",
			Active:         true,
			MaxConnections: 3,
			RateLimit:      50,
			Permissions: map[smpp.Operation]bool{
				smpp.OperationBind:    true,
				smpp.OperationSubmit:  true,
				smpp.OperationDeliver: true,
			},
		},
	}

	for _, user := range defaultUsers {
		if err := a.CreateUser(context.Background(), user); err != nil {
			a.logger.Error("Failed to create default user", "systemID", user.SystemID, "error", err)
		}
	}
}

// hashPassword creates a hash of the password with salt
func (a *DefaultUserAuth) hashPassword(password, salt string) string {
	h := sha256.New()
	h.Write([]byte(password + salt))
	return hex.EncodeToString(h.Sum(nil))
}

// generateSalt generates a random salt
func (a *DefaultUserAuth) generateSalt() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// generateID generates a random ID
func (a *DefaultUserAuth) generateID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
