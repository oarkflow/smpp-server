package smpp

import (
	"context"
	"fmt"
)

// ConnectionManagerEventHandler handles connection events to manage connections in the connection manager
type ConnectionManagerEventHandler struct {
	connectionManager ConnectionManager
	logger            Logger
}

// NewConnectionManagerEventHandler creates a new connection manager event handler
func NewConnectionManagerEventHandler(connectionManager ConnectionManager, logger Logger) *ConnectionManagerEventHandler {
	return &ConnectionManagerEventHandler{
		connectionManager: connectionManager,
		logger:            logger,
	}
}

// HandleEvent handles connection events
func (h *ConnectionManagerEventHandler) HandleEvent(ctx context.Context, event Event) error {
	switch event.GetEventType() {
	case EventTypeConnected:
		return h.handleConnected(ctx, event)
	case EventTypeDisconnected:
		return h.handleDisconnected(ctx, event)
	default:
		// Ignore other event types
		return nil
	}
}

// handleConnected adds a connection to the manager when it connects
func (h *ConnectionManagerEventHandler) handleConnected(ctx context.Context, event Event) error {
	connEvent, ok := event.(*ConnectionEvent)
	if !ok {
		return fmt.Errorf("invalid event type for connected event")
	}

	// Create a connection wrapper from the event data
	// Note: We need to store the actual connection object in the event data
	// For now, we'll assume the connection is passed in event data
	conn, ok := connEvent.Data["connection"].(Connection)
	if !ok {
		if h.logger != nil {
			h.logger.Warn("No connection object in connected event data",
				"session_id", connEvent.SessionID)
		}
		return nil // Can't add connection without the connection object
	}

	// Add connection to manager
	if err := h.connectionManager.AddConnection(ctx, conn); err != nil {
		if h.logger != nil {
			h.logger.Error("Failed to add connection to manager",
				"session_id", connEvent.SessionID,
				"error", err)
		}
		return err
	}

	if h.logger != nil {
		h.logger.Debug("Connection added to manager",
			"session_id", connEvent.SessionID,
			"remote_addr", connEvent.RemoteAddr)
	}

	return nil
}

// handleDisconnected removes a connection from the manager when it disconnects
func (h *ConnectionManagerEventHandler) handleDisconnected(ctx context.Context, event Event) error {
	connEvent, ok := event.(*ConnectionEvent)
	if !ok {
		return fmt.Errorf("invalid event type for disconnected event")
	}

	// Remove connection from manager
	if err := h.connectionManager.RemoveConnection(ctx, connEvent.SessionID); err != nil {
		if h.logger != nil {
			h.logger.Error("Failed to remove connection from manager",
				"session_id", connEvent.SessionID,
				"error", err)
		}
		return err
	}

	if h.logger != nil {
		h.logger.Debug("Connection removed from manager",
			"session_id", connEvent.SessionID)
	}

	return nil
}

// GetHandlerID returns the handler ID
func (h *ConnectionManagerEventHandler) GetHandlerID() string {
	return "connection_manager"
}
