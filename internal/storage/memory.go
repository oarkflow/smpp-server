package storage

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/oarkflow/smpp-server/pkg/smpp"
)

// InMemorySMSStorage implements SMSStorage interface using in-memory storage
type InMemorySMSStorage struct {
	mu       sync.RWMutex
	messages map[string]*smpp.Message
	logger   smpp.Logger
}

// NewInMemorySMSStorage creates a new in-memory SMS storage
func NewInMemorySMSStorage(logger smpp.Logger) *InMemorySMSStorage {
	return &InMemorySMSStorage{
		messages: make(map[string]*smpp.Message),
		logger:   logger,
	}
}

// StoreSMS stores an SMS message and returns the message ID
func (s *InMemorySMSStorage) StoreSMS(ctx context.Context, message *smpp.Message) (string, error) {
	if message == nil {
		return "", fmt.Errorf("message cannot be nil")
	}

	if message.ID == "" {
		return "", fmt.Errorf("message ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a copy to avoid external modifications
	messageCopy := *message
	messageCopy.SubmitTime = time.Now()

	s.messages[message.ID] = &messageCopy

	if s.logger != nil {
		s.logger.Debug("SMS stored",
			"message_id", message.ID,
			"source", message.SourceAddr.Addr,
			"dest", message.DestAddr.Addr)
	}

	return message.ID, nil
}

// GetSMS retrieves an SMS message by ID
func (s *InMemorySMSStorage) GetSMS(ctx context.Context, messageID string) (*smpp.Message, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message ID cannot be empty")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	message, exists := s.messages[messageID]
	if !exists {
		return nil, fmt.Errorf("message with ID %s not found", messageID)
	}

	// Return a copy to avoid external modifications
	messageCopy := *message
	return &messageCopy, nil
}

// UpdateSMSStatus updates the status of an SMS message
func (s *InMemorySMSStorage) UpdateSMSStatus(ctx context.Context, messageID string, status smpp.MessageStatus) error {
	if messageID == "" {
		return fmt.Errorf("message ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	message, exists := s.messages[messageID]
	if !exists {
		return fmt.Errorf("message with ID %s not found", messageID)
	}

	message.Status = status

	// Update done time for final statuses
	if status == smpp.MessageStatusDelivered || status == smpp.MessageStatusExpired ||
		status == smpp.MessageStatusDeleted || status == smpp.MessageStatusUndeliverable ||
		status == smpp.MessageStatusRejected {
		now := time.Now()
		message.DoneTime = &now
		message.DoneDate = now.Format("0601021504")
	}

	if s.logger != nil {
		s.logger.Debug("SMS status updated",
			"message_id", messageID,
			"status", status.String())
	}

	return nil
}

// SearchSMS searches for SMS messages based on criteria
func (s *InMemorySMSStorage) SearchSMS(ctx context.Context, criteria *smpp.SMSSearchCriteria) ([]*smpp.Message, error) {
	if criteria == nil {
		return nil, fmt.Errorf("search criteria cannot be nil")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*smpp.Message

	for _, message := range s.messages {
		if s.matchesCriteria(message, criteria) {
			messageCopy := *message
			results = append(results, &messageCopy)
		}
	}

	// Sort by submit time (newest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].SubmitTime.After(results[j].SubmitTime)
	})

	// Apply limit and offset
	start := criteria.Offset
	end := start + criteria.Limit

	if start >= len(results) {
		return []*smpp.Message{}, nil
	}

	if end > len(results) {
		end = len(results)
	}

	if criteria.Limit > 0 {
		results = results[start:end]
	}

	return results, nil
}

// DeleteSMS deletes an SMS message
func (s *InMemorySMSStorage) DeleteSMS(ctx context.Context, messageID string) error {
	if messageID == "" {
		return fmt.Errorf("message ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.messages[messageID]; !exists {
		return fmt.Errorf("message with ID %s not found", messageID)
	}

	delete(s.messages, messageID)

	if s.logger != nil {
		s.logger.Debug("SMS deleted", "message_id", messageID)
	}

	return nil
}

// GetSMSByDateRange retrieves SMS messages within a date range
func (s *InMemorySMSStorage) GetSMSByDateRange(ctx context.Context, from, to time.Time) ([]*smpp.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*smpp.Message

	for _, message := range s.messages {
		if (message.SubmitTime.Equal(from) || message.SubmitTime.After(from)) &&
			(message.SubmitTime.Equal(to) || message.SubmitTime.Before(to)) {
			messageCopy := *message
			results = append(results, &messageCopy)
		}
	}

	// Sort by submit time
	sort.Slice(results, func(i, j int) bool {
		return results[i].SubmitTime.Before(results[j].SubmitTime)
	})

	return results, nil
}

// GetSMSCount returns the total count of SMS messages
func (s *InMemorySMSStorage) GetSMSCount(ctx context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return int64(len(s.messages)), nil
}

// matchesCriteria checks if a message matches the search criteria
func (s *InMemorySMSStorage) matchesCriteria(message *smpp.Message, criteria *smpp.SMSSearchCriteria) bool {
	if criteria.SourceAddr != "" && message.SourceAddr.Addr != criteria.SourceAddr {
		return false
	}

	if criteria.DestAddr != "" && message.DestAddr.Addr != criteria.DestAddr {
		return false
	}

	if criteria.Status != nil && message.Status != *criteria.Status {
		return false
	}

	if criteria.FromDate != nil && message.SubmitTime.Before(*criteria.FromDate) {
		return false
	}

	if criteria.ToDate != nil && message.SubmitTime.After(*criteria.ToDate) {
		return false
	}

	return true
}

// InMemoryReportStorage implements ReportStorage interface using in-memory storage
type InMemoryReportStorage struct {
	mu      sync.RWMutex
	reports map[string]*smpp.DeliveryReport
	logger  smpp.Logger
}

// NewInMemoryReportStorage creates a new in-memory report storage
func NewInMemoryReportStorage(logger smpp.Logger) *InMemoryReportStorage {
	return &InMemoryReportStorage{
		reports: make(map[string]*smpp.DeliveryReport),
		logger:  logger,
	}
}

// StoreReport stores a delivery report
func (r *InMemoryReportStorage) StoreReport(ctx context.Context, report *smpp.DeliveryReport) error {
	if report == nil {
		return fmt.Errorf("report cannot be nil")
	}

	if report.MessageID == "" {
		return fmt.Errorf("report message ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Create a copy to avoid external modifications
	reportCopy := *report
	r.reports[report.MessageID] = &reportCopy

	if r.logger != nil {
		r.logger.Debug("Delivery report stored",
			"message_id", report.MessageID,
			"status", report.Status)
	}

	return nil
}

// GetReport retrieves a delivery report by message ID
func (r *InMemoryReportStorage) GetReport(ctx context.Context, messageID string) (*smpp.DeliveryReport, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message ID cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	report, exists := r.reports[messageID]
	if !exists {
		return nil, fmt.Errorf("report for message ID %s not found", messageID)
	}

	// Return a copy to avoid external modifications
	reportCopy := *report
	return &reportCopy, nil
}

// GetReportsByDateRange retrieves delivery reports within a date range
func (r *InMemoryReportStorage) GetReportsByDateRange(ctx context.Context, from, to time.Time) ([]*smpp.DeliveryReport, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*smpp.DeliveryReport

	// Note: In a real implementation, reports would have timestamps
	// For now, we return all reports
	for _, report := range r.reports {
		reportCopy := *report
		results = append(results, &reportCopy)
	}

	return results, nil
}

// SearchReports searches for delivery reports based on criteria
func (r *InMemoryReportStorage) SearchReports(ctx context.Context, criteria *smpp.ReportSearchCriteria) ([]*smpp.DeliveryReport, error) {
	if criteria == nil {
		return nil, fmt.Errorf("search criteria cannot be nil")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*smpp.DeliveryReport

	for _, report := range r.reports {
		if r.matchesReportCriteria(report, criteria) {
			reportCopy := *report
			results = append(results, &reportCopy)
		}
	}

	// Apply limit and offset
	start := criteria.Offset
	end := start + criteria.Limit

	if start >= len(results) {
		return []*smpp.DeliveryReport{}, nil
	}

	if end > len(results) {
		end = len(results)
	}

	if criteria.Limit > 0 {
		results = results[start:end]
	}

	return results, nil
}

// DeleteReport deletes a delivery report
func (r *InMemoryReportStorage) DeleteReport(ctx context.Context, messageID string) error {
	if messageID == "" {
		return fmt.Errorf("message ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.reports[messageID]; !exists {
		return fmt.Errorf("report for message ID %s not found", messageID)
	}

	delete(r.reports, messageID)

	if r.logger != nil {
		r.logger.Debug("Delivery report deleted", "message_id", messageID)
	}

	return nil
}

// GetReportCount returns the total count of delivery reports
func (r *InMemoryReportStorage) GetReportCount(ctx context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return int64(len(r.reports)), nil
}

// matchesReportCriteria checks if a report matches the search criteria
func (r *InMemoryReportStorage) matchesReportCriteria(report *smpp.DeliveryReport, criteria *smpp.ReportSearchCriteria) bool {
	if criteria.MessageID != "" && report.MessageID != criteria.MessageID {
		return false
	}

	if criteria.Status != "" && report.Status != criteria.Status {
		return false
	}

	// Date range filtering would be implemented here with proper timestamp fields

	return true
}

// InMemoryUserAuth implements UserAuth interface using in-memory storage
type InMemoryUserAuth struct {
	mu     sync.RWMutex
	users  map[string]*smpp.User
	logger smpp.Logger
}

// NewInMemoryUserAuth creates a new in-memory user auth
func NewInMemoryUserAuth(logger smpp.Logger) *InMemoryUserAuth {
	auth := &InMemoryUserAuth{
		users:  make(map[string]*smpp.User),
		logger: logger,
	}

	// Add default admin user
	adminUser := &smpp.User{
		SystemID:       "admin",
		Password:       "admin123",
		SystemType:     "SMPP",
		MaxConnections: 10,
		RateLimit:      1000,
		Enabled:        true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Permissions: map[smpp.Operation]bool{
			smpp.OperationBind:    true,
			smpp.OperationSubmit:  true,
			smpp.OperationDeliver: true,
			smpp.OperationQuery:   true,
			smpp.OperationCancel:  true,
			smpp.OperationReplace: true,
			smpp.OperationUnbind:  true,
		},
	}

	auth.users["admin"] = adminUser

	return auth
}

// Authenticate validates user credentials
func (a *InMemoryUserAuth) Authenticate(ctx context.Context, systemID, password string) (*smpp.User, error) {
	if systemID == "" {
		return nil, fmt.Errorf("system ID cannot be empty")
	}

	if password == "" {
		return nil, fmt.Errorf("password cannot be empty")
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	user, exists := a.users[systemID]
	if !exists {
		if a.logger != nil {
			a.logger.Warn("Authentication failed: user not found", "system_id", systemID)
		}
		return nil, fmt.Errorf("user not found")
	}

	if !user.Enabled {
		if a.logger != nil {
			a.logger.Warn("Authentication failed: user disabled", "system_id", systemID)
		}
		return nil, fmt.Errorf("user disabled")
	}

	if user.Password != password {
		if a.logger != nil {
			a.logger.Warn("Authentication failed: invalid password", "system_id", systemID)
		}
		return nil, fmt.Errorf("invalid password")
	}

	if a.logger != nil {
		a.logger.Debug("Authentication successful", "system_id", systemID)
	}

	// Return a copy to avoid external modifications
	userCopy := *user
	return &userCopy, nil
}

// CreateUser creates a new user
func (a *InMemoryUserAuth) CreateUser(ctx context.Context, user *smpp.User) error {
	if user == nil {
		return fmt.Errorf("user cannot be nil")
	}

	if user.SystemID == "" {
		return fmt.Errorf("user system ID cannot be empty")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.users[user.SystemID]; exists {
		return fmt.Errorf("user with system ID %s already exists", user.SystemID)
	}

	// Create a copy and set timestamps
	userCopy := *user
	userCopy.CreatedAt = time.Now()
	userCopy.UpdatedAt = time.Now()

	a.users[user.SystemID] = &userCopy

	if a.logger != nil {
		a.logger.Info("User created", "system_id", user.SystemID)
	}

	return nil
}

// UpdateUser updates user information
func (a *InMemoryUserAuth) UpdateUser(ctx context.Context, user *smpp.User) error {
	if user == nil {
		return fmt.Errorf("user cannot be nil")
	}

	if user.SystemID == "" {
		return fmt.Errorf("user system ID cannot be empty")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	existingUser, exists := a.users[user.SystemID]
	if !exists {
		return fmt.Errorf("user with system ID %s not found", user.SystemID)
	}

	// Update fields while preserving created timestamp
	userCopy := *user
	userCopy.CreatedAt = existingUser.CreatedAt
	userCopy.UpdatedAt = time.Now()

	a.users[user.SystemID] = &userCopy

	if a.logger != nil {
		a.logger.Info("User updated", "system_id", user.SystemID)
	}

	return nil
}

// DeleteUser deletes a user
func (a *InMemoryUserAuth) DeleteUser(ctx context.Context, systemID string) error {
	if systemID == "" {
		return fmt.Errorf("system ID cannot be empty")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.users[systemID]; !exists {
		return fmt.Errorf("user with system ID %s not found", systemID)
	}

	delete(a.users, systemID)

	if a.logger != nil {
		a.logger.Info("User deleted", "system_id", systemID)
	}

	return nil
}

// GetUser retrieves user information
func (a *InMemoryUserAuth) GetUser(ctx context.Context, systemID string) (*smpp.User, error) {
	if systemID == "" {
		return nil, fmt.Errorf("system ID cannot be empty")
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	user, exists := a.users[systemID]
	if !exists {
		return nil, fmt.Errorf("user with system ID %s not found", systemID)
	}

	// Return a copy to avoid external modifications
	userCopy := *user
	return &userCopy, nil
}

// ListUsers returns all users
func (a *InMemoryUserAuth) ListUsers(ctx context.Context) ([]*smpp.User, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	users := make([]*smpp.User, 0, len(a.users))
	for _, user := range a.users {
		userCopy := *user
		users = append(users, &userCopy)
	}

	// Sort by system ID
	sort.Slice(users, func(i, j int) bool {
		return users[i].SystemID < users[j].SystemID
	})

	return users, nil
}

// IsAuthorized checks if a user is authorized for a specific operation
func (a *InMemoryUserAuth) IsAuthorized(ctx context.Context, systemID string, operation smpp.Operation) (bool, error) {
	user, err := a.GetUser(ctx, systemID)
	if err != nil {
		return false, err
	}

	if !user.Enabled {
		return false, fmt.Errorf("user disabled")
	}

	// Check permissions
	allowed, exists := user.Permissions[operation]
	return exists && allowed, nil
}
