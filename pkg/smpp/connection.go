package smpp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// ServerConnection implements the Connection interface for server-side connections
type ServerConnection struct {
	netConn      net.Conn
	session      *Session
	config       *ServerConfig
	logger       Logger
	mu           sync.RWMutex
	lastActivity time.Time
	active       bool
	encoder      *PDUEncoder
}

// NewServerConnection creates a new server connection
func NewServerConnection(netConn net.Conn, config *ServerConfig, logger Logger) *ServerConnection {
	return &ServerConnection{
		netConn:      netConn,
		config:       config,
		logger:       logger,
		lastActivity: time.Now(),
		active:       true,
		encoder:      NewPDUEncoder(),
	}
}

// GetSessionID returns the session ID
func (sc *ServerConnection) GetSessionID() string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if sc.session == nil {
		return ""
	}
	return sc.session.ID
}

// GetSession returns the session information
func (sc *ServerConnection) GetSession() *Session {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return sc.session
}

// SetSession sets the session information
func (sc *ServerConnection) SetSession(session *Session) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.session = session
}

// Send sends a PDU to the connection
func (sc *ServerConnection) Send(ctx context.Context, pdu *PDU) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if sc.logger != nil {
				sc.logger.Error("Panic in Send", "panic", r, "session_id", sc.GetSessionID())
			}
			err = fmt.Errorf("panic in Send: %v", r)
		}
	}()

	sc.mu.RLock()
	active := sc.active
	sc.mu.RUnlock()

	if !active {
		return fmt.Errorf("connection is not active")
	}

	// Encode PDU
	data, err := sc.encoder.Encode(pdu)
	if err != nil {
		return fmt.Errorf("failed to encode PDU: %w", err)
	}

	// Set write deadline
	if err := sc.netConn.SetWriteDeadline(time.Now().Add(sc.config.WriteTimeout)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}

	// Write data
	_, err = sc.netConn.Write(data)
	if err != nil {
		sc.mu.Lock()
		sc.active = false
		sc.mu.Unlock()
		return fmt.Errorf("failed to write data: %w", err)
	}

	sc.mu.Lock()
	sc.lastActivity = time.Now()
	sc.mu.Unlock()

	if sc.logger != nil {
		sc.logger.Debug("PDU sent",
			"session_id", sc.GetSessionID(),
			"command_id", fmt.Sprintf("0x%08X", pdu.Header.CommandID),
			"sequence", pdu.Header.SequenceNum)
	}

	return nil
}

// Close closes the connection
func (sc *ServerConnection) Close() error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.active = false
	return sc.netConn.Close()
}

// IsActive returns true if the connection is active
func (sc *ServerConnection) IsActive() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return sc.active
}

// GetRemoteAddr returns the remote address
func (sc *ServerConnection) GetRemoteAddr() string {
	return sc.netConn.RemoteAddr().String()
}

// GetLastActivity returns the last activity time
func (sc *ServerConnection) GetLastActivity() time.Time {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return sc.lastActivity
}

// UpdateLastActivity updates the last activity time
func (sc *ServerConnection) UpdateLastActivity() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.lastActivity = time.Now()
}

// InMemoryConnectionManager implements ConnectionManager interface
type InMemoryConnectionManager struct {
	mu          sync.RWMutex
	connections map[string]Connection
	logger      Logger
}

// NewInMemoryConnectionManager creates a new in-memory connection manager
func NewInMemoryConnectionManager(logger Logger) *InMemoryConnectionManager {
	return &InMemoryConnectionManager{
		connections: make(map[string]Connection),
		logger:      logger,
	}
}

// AddConnection adds a new connection
func (cm *InMemoryConnectionManager) AddConnection(ctx context.Context, conn Connection) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	sessionID := conn.GetSessionID()
	if sessionID == "" {
		return fmt.Errorf("connection must have a session ID")
	}

	if _, exists := cm.connections[sessionID]; exists {
		return fmt.Errorf("connection with session ID %s already exists", sessionID)
	}

	cm.connections[sessionID] = conn

	if cm.logger != nil {
		cm.logger.Debug("Connection added",
			"session_id", sessionID,
			"remote_addr", conn.GetRemoteAddr())
	}

	return nil
}

// RemoveConnection removes a connection
func (cm *InMemoryConnectionManager) RemoveConnection(ctx context.Context, sessionID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.connections[sessionID]; !exists {
		return fmt.Errorf("connection with session ID %s not found", sessionID)
	}

	delete(cm.connections, sessionID)

	if cm.logger != nil {
		cm.logger.Debug("Connection removed", "session_id", sessionID)
	}

	return nil
}

// GetConnection retrieves a connection by session ID
func (cm *InMemoryConnectionManager) GetConnection(ctx context.Context, sessionID string) (Connection, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	conn, exists := cm.connections[sessionID]
	if !exists {
		return nil, fmt.Errorf("connection with session ID %s not found", sessionID)
	}

	return conn, nil
}

// GetAllConnections returns all active connections
func (cm *InMemoryConnectionManager) GetAllConnections(ctx context.Context) ([]Connection, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	connections := make([]Connection, 0, len(cm.connections))
	for _, conn := range cm.connections {
		connections = append(connections, conn)
	}

	return connections, nil
}

// BroadcastMessage broadcasts a message to all connections
func (cm *InMemoryConnectionManager) BroadcastMessage(ctx context.Context, message interface{}) error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	pdu, ok := message.(*PDU)
	if !ok {
		return fmt.Errorf("message must be a PDU")
	}

	var errors []error
	for sessionID, conn := range cm.connections {
		if err := conn.Send(ctx, pdu); err != nil {
			errors = append(errors, fmt.Errorf("failed to send to session %s: %w", sessionID, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("broadcast failed for %d connections: %v", len(errors), errors)
	}

	return nil
}

// GetConnectionCount returns the number of active connections
func (cm *InMemoryConnectionManager) GetConnectionCount(ctx context.Context) int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return len(cm.connections)
}
