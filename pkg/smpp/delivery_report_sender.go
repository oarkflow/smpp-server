package smpp

import (
	"context"
	"fmt"

	"github.com/oarkflow/smpp-server/pkg/encoding"
)

// DeliveryReportSender handles sending delivery reports to clients
type DeliveryReportSender struct {
	connectionManager ConnectionManager
	logger            Logger
	textEncoder       *encoding.TextEncoder
	builder           *PDUBuilder
}

// NewDeliveryReportSender creates a new delivery report sender
func NewDeliveryReportSender(connectionManager ConnectionManager, logger Logger) *DeliveryReportSender {
	return &DeliveryReportSender{
		connectionManager: connectionManager,
		logger:            logger,
		textEncoder:       encoding.NewTextEncoder(),
		builder:           NewPDUBuilder(),
	}
}

// HandleEvent handles delivery report events
func (drs *DeliveryReportSender) HandleEvent(ctx context.Context, event Event) error {
	deliveryEvent, ok := event.(*DeliveryEvent)
	if !ok {
		return nil // Not a delivery event
	}

	if drs.logger != nil {
		drs.logger.Debug("Processing delivery report event",
			"message_id", deliveryEvent.MessageID,
			"report_status", deliveryEvent.Report.Status)
	}

	// Get the message to find the session ID
	// Note: In a real implementation, you'd want to get this from storage
	// For now, we'll assume the report has the session info or we need to look it up

	// For now, we'll broadcast to all connections or find the right session
	// This is a simplified implementation - in production you'd want to track
	// message ID to session ID mapping

	if deliveryEvent.Report == nil {
		return fmt.Errorf("delivery report is nil")
	}

	// Create deliver_sm PDU with delivery report
	reportText := deliveryEvent.Report.Text
	reportBytes, err := drs.textEncoder.Encode(reportText, 0) // Default data coding
	if err != nil {
		if drs.logger != nil {
			drs.logger.Error("Failed to encode delivery report text",
				"message_id", deliveryEvent.MessageID,
				"error", err)
		}
		return fmt.Errorf("failed to encode delivery report: %w", err)
	}

	// Create deliver_sm PDU
	deliverPDU := drs.builder.BuildDeliverSM(
		deliveryEvent.Report.MessageID, // Source address (system ID or message ID)
		"",                             // Destination address (empty for delivery reports)
		reportBytes,
		0, // Data coding
	)

	// Set ESM class to indicate this is a delivery receipt
	deliverPDU.Body.(*DeliverSM).EsmClass = 0x04 // Delivery receipt

	// Get the session ID from the event data
	sessionID, ok := deliveryEvent.Data["session_id"].(string)
	if !ok {
		if drs.logger != nil {
			drs.logger.Warn("No session_id in delivery event data",
				"message_id", deliveryEvent.MessageID)
		}
		return nil // Can't send without session ID
	}

	// Get the connection for this session
	conn, err := drs.connectionManager.GetConnection(ctx, sessionID)
	if err != nil {
		if drs.logger != nil {
			drs.logger.Warn("Failed to get connection for session",
				"session_id", sessionID,
				"message_id", deliveryEvent.MessageID,
				"error", err)
		}
		return nil // Connection not found or not active
	}

	// Send the delivery report to the specific connection
	if err := conn.Send(ctx, deliverPDU); err != nil {
		if drs.logger != nil {
			drs.logger.Error("Failed to send delivery report",
				"session_id", sessionID,
				"message_id", deliveryEvent.MessageID,
				"error", err)
		}
		return fmt.Errorf("failed to send delivery report: %w", err)
	}

	if drs.logger != nil {
		drs.logger.Info("Sent delivery report",
			"session_id", sessionID,
			"message_id", deliveryEvent.MessageID)
	}

	return nil
}

// GetHandlerID returns the handler ID
func (drs *DeliveryReportSender) GetHandlerID() string {
	return "delivery_report_sender"
}
