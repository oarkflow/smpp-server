package smpp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oarkflow/smpp-server/pkg/encoding"
)

// MessageRouter handles message routing logic
type MessageRouter interface {
	RouteMessage(ctx context.Context, message *Message) (*RouteInfo, error)
}

// RouteInfo contains routing information for a message
type RouteInfo struct {
	Gateway     string
	Priority    int
	RetryCount  int
	Destination string
	Route       string
}

// MessageQueue handles message queuing and delivery
type MessageQueue interface {
	EnqueueMessage(ctx context.Context, message *Message, route *RouteInfo) error
	DequeueMessage(ctx context.Context) (*QueuedMessage, error)
	UpdateMessageStatus(ctx context.Context, messageID string, status MessageStatus) error
}

// QueuedMessage represents a message in the queue
type QueuedMessage struct {
	Message   *Message
	Route     *RouteInfo
	Attempts  int
	NextRetry time.Time
	Created   time.Time
}

// MessageSplitter handles long message splitting
type MessageSplitter interface {
	SplitMessage(message *Message) ([]*Message, error)
}

// MessageHandlerImpl implements the MessageHandler interface
type MessageHandlerImpl struct {
	smsStorage     SMSStorage
	reportStorage  ReportStorage
	eventPublisher EventPublisher
	textEncoder    *encoding.TextEncoder
	logger         Logger
	metrics        MetricsCollector
	router         MessageRouter
	queue          MessageQueue
	splitter       MessageSplitter
}

// NewMessageHandler creates a new message handler
func NewMessageHandler(deps MessageHandlerDependencies) *MessageHandlerImpl {
	return &MessageHandlerImpl{
		smsStorage:     deps.SMSStorage,
		reportStorage:  deps.ReportStorage,
		eventPublisher: deps.EventPublisher,
		textEncoder:    encoding.NewTextEncoder(),
		logger:         deps.Logger,
		metrics:        deps.MetricsCollector,
		router:         deps.Router,
		queue:          deps.Queue,
		splitter:       deps.Splitter,
	}
}

// MessageHandlerDependencies holds dependencies for the message handler
type MessageHandlerDependencies struct {
	SMSStorage       SMSStorage
	ReportStorage    ReportStorage
	EventPublisher   EventPublisher
	Logger           Logger
	MetricsCollector MetricsCollector
	Router           MessageRouter
	Queue            MessageQueue
	Splitter         MessageSplitter
}

// HandleSubmitSM processes a submit_sm PDU
func (mh *MessageHandlerImpl) HandleSubmitSM(ctx context.Context, session *Session, pdu *SubmitSM) (*SubmitSMResp, error) {
	// Generate message ID
	messageID := mh.generateMessageID()

	// Get message bytes from ShortMessage or message_payload
	var messageBytes []byte
	if len(pdu.ShortMessage) > 0 {
		messageBytes = pdu.ShortMessage
	} else {
		// Check for message_payload optional parameter
		for _, param := range pdu.OptionalParams {
			if param.Tag == TagMessagePayload {
				messageBytes = param.Value
				break
			}
		}
	}

	if len(messageBytes) == 0 {
		return &SubmitSMResp{MessageID: ""}, fmt.Errorf("no message content provided")
	}

	// Decode message text
	messageText, err := mh.textEncoder.Decode(messageBytes, pdu.DataCoding)
	if err != nil {
		if mh.logger != nil {
			mh.logger.Error("Failed to decode message text",
				"message_id", messageID,
				"data_coding", pdu.DataCoding,
				"error", err)
		}
		return &SubmitSMResp{MessageID: ""}, fmt.Errorf("failed to decode message: %w", err)
	}

	// Create message object
	message := &Message{
		ID:        messageID,
		SessionID: session.ID, // Store session ID for delivery reports
		SourceAddr: Address{
			TON:  pdu.SourceAddrTON,
			NPI:  pdu.SourceAddrNPI,
			Addr: pdu.SourceAddr,
		},
		DestAddr: Address{
			TON:  pdu.DestAddrTON,
			NPI:  pdu.DestAddrNPI,
			Addr: pdu.DestAddr,
		},
		ShortMessage:         pdu.ShortMessage,
		DataCoding:           pdu.DataCoding,
		EsmClass:             pdu.EsmClass,
		RegisteredDelivery:   pdu.RegisteredDelivery,
		ValidityPeriod:       pdu.ValidityPeriod,
		ScheduleDeliveryTime: pdu.ScheduleDeliveryTime,
		ServiceType:          pdu.ServiceType,
		ProtocolID:           pdu.ProtocolID,
		PriorityFlag:         pdu.PriorityFlag,
		Status:               MessageStatusSubmitted,
		SubmitTime:           time.Now(),
		Text:                 messageText,
	}

	// Validate message
	if err := mh.ValidateMessage(ctx, message); err != nil {
		if mh.logger != nil {
			mh.logger.Warn("Message validation failed",
				"message_id", messageID,
				"error", err)
		}
		return &SubmitSMResp{MessageID: ""}, err
	}

	// Store message
	if mh.smsStorage != nil {
		if _, err := mh.smsStorage.StoreSMS(ctx, message); err != nil {
			if mh.logger != nil {
				mh.logger.Error("Failed to store SMS",
					"message_id", messageID,
					"error", err)
			}
			return &SubmitSMResp{MessageID: ""}, fmt.Errorf("failed to store SMS: %w", err)
		}
	}

	// Log successful submission
	if mh.logger != nil {
		mh.logger.Info("SMS submitted successfully",
			"message_id", messageID,
			"source", pdu.SourceAddr,
			"dest", pdu.DestAddr,
			"data_coding", pdu.DataCoding,
			"text_length", len(messageText),
			"session_id", session.ID)
	}

	// Update metrics
	if mh.metrics != nil {
		mh.metrics.IncCounter("sms_submitted_total", map[string]string{
			"source_ton":  strconv.Itoa(int(pdu.SourceAddrTON)),
			"dest_ton":    strconv.Itoa(int(pdu.DestAddrTON)),
			"data_coding": strconv.Itoa(int(pdu.DataCoding)),
		})
	}

	// Publish SMS event
	if mh.eventPublisher != nil {
		event := &SMSEvent{
			Type:      EventTypeSMSSubmitted,
			Timestamp: time.Now(),
			MessageID: messageID,
			Session:   session,
			Message:   message,
			Data:      make(map[string]interface{}),
		}
		mh.eventPublisher.PublishSMSEvent(ctx, event)
	}

	// Process message for delivery (asynchronously)
	go func() {
		if err := mh.ProcessMessage(context.Background(), message); err != nil {
			if mh.logger != nil {
				mh.logger.Error("Failed to process message for delivery",
					"message_id", messageID,
					"error", err)
			}
		}
	}()

	return &SubmitSMResp{MessageID: messageID}, nil
}

// HandleQuerySM processes a query_sm PDU
func (mh *MessageHandlerImpl) HandleQuerySM(ctx context.Context, session *Session, pdu *QuerySM) (*QuerySMResp, error) {
	messageID := pdu.MessageID.GetString()

	if mh.logger != nil {
		mh.logger.Info("Received query_sm",
			"session_id", session.ID,
			"message_id", messageID,
			"source_addr", pdu.SourceAddr.Addr)
	}

	// Get message from storage
	_, err := mh.smsStorage.GetSMS(ctx, messageID)
	if err != nil {
		if mh.logger != nil {
			mh.logger.Warn("Message not found for query",
				"message_id", messageID,
				"error", err)
		}
		emptyDate := NewCString(17) // Max length for date string
		return &QuerySMResp{
			MessageID:    pdu.MessageID,
			FinalDate:    *emptyDate,
			MessageState: 2, // MessageStateExpired (message not found)
			ErrorCode:    1, // Error code for message not found
		}, nil
	}

	// Get delivery report if available
	report, err := mh.reportStorage.GetReport(ctx, messageID)
	messageState := uint8(1) // MessageStateEnroute (default)
	errorCode := uint8(0)

	if err == nil && report != nil {
		// Map delivery status to message state
		switch report.Status {
		case "DELIVRD":
			messageState = 3 // MessageStateDelivered
		case "EXPIRED":
			messageState = 2 // MessageStateExpired
		case "DELETED":
			messageState = 4 // MessageStateDeleted
		case "UNDELIV":
			messageState = 5 // MessageStateUndeliverable
		case "ACCEPTD":
			messageState = 1 // MessageStateEnroute
		case "UNKNOWN":
			messageState = 7 // MessageStateUnknown
		case "REJECTD":
			messageState = 6 // MessageStateRejected
		default:
			messageState = 1 // MessageStateEnroute
		}

		// Set error code if present
		if report.Error != "" {
			if code, parseErr := strconv.Atoi(report.Error); parseErr == nil {
				errorCode = uint8(code)
			}
		}
	}

	// Format final date
	finalDateStr := ""
	if report != nil && !report.DoneTime.IsZero() {
		finalDateStr = report.DoneTime.Format("0601021504") // YYMMDDHHMM format
	}
	finalDate := NewCString(17)
	finalDate.SetString(finalDateStr)

	return &QuerySMResp{
		MessageID:    pdu.MessageID,
		FinalDate:    *finalDate,
		MessageState: messageState,
		ErrorCode:    errorCode,
	}, nil
}

// HandleReplaceSM processes a replace_sm PDU
func (mh *MessageHandlerImpl) HandleReplaceSM(ctx context.Context, session *Session, pdu *ReplaceSM) (*ReplaceSMResp, error) {
	messageID := pdu.MessageID.GetString()

	if mh.logger != nil {
		mh.logger.Info("Received replace_sm",
			"session_id", session.ID,
			"message_id", messageID,
			"source_addr", pdu.SourceAddr.Addr)
	}

	// Get the original message
	message, err := mh.smsStorage.GetSMS(ctx, messageID)
	if err != nil {
		if mh.logger != nil {
			mh.logger.Warn("Message not found for replacement",
				"message_id", messageID,
				"error", err)
		}
		return &ReplaceSMResp{}, nil // Empty response for not found
	}

	// Check if message can be replaced (must be in enroute state)
	report, _ := mh.reportStorage.GetReport(ctx, messageID)
	if report != nil && report.Status != "ACCEPTD" && report.Status != "ENROUTE" {
		if mh.logger != nil {
			mh.logger.Warn("Message cannot be replaced - not in enroute state",
				"message_id", messageID,
				"status", report.Status)
		}
		return &ReplaceSMResp{}, nil
	}

	// Update the message with new content
	message.Text = string(pdu.ShortMessage)
	message.DataCoding = pdu.SMDefaultMsgID // Use SMDefaultMsgID as data coding

	// Update schedule delivery time if provided
	if scheduleTime := pdu.ScheduleDeliveryTime.GetString(); scheduleTime != "" {
		message.ScheduleDeliveryTime = scheduleTime
	}

	// Update validity period if provided
	if validityPeriod := pdu.ValidityPeriod.GetString(); validityPeriod != "" {
		message.ValidityPeriod = validityPeriod
	}

	// Update registered delivery
	message.RegisteredDelivery = pdu.RegisteredDelivery

	// Save the updated message
	err = mh.smsStorage.UpdateSMS(ctx, message)
	if err != nil {
		if mh.logger != nil {
			mh.logger.Error("Failed to update message for replacement",
				"message_id", messageID,
				"error", err)
		}
		return &ReplaceSMResp{}, nil
	}

	if mh.logger != nil {
		mh.logger.Info("Message replaced successfully",
			"message_id", messageID)
	}

	return &ReplaceSMResp{}, nil
}

// HandleCancelSM processes a cancel_sm PDU
func (mh *MessageHandlerImpl) HandleCancelSM(ctx context.Context, session *Session, pdu *CancelSM) (*CancelSMResp, error) {
	messageID := pdu.MessageID.GetString()

	if mh.logger != nil {
		mh.logger.Info("Received cancel_sm",
			"session_id", session.ID,
			"message_id", messageID,
			"source_addr", pdu.SourceAddr.GetString())
	}

	// Get the message to cancel
	_, err := mh.smsStorage.GetSMS(ctx, messageID)
	if err != nil {
		if mh.logger != nil {
			mh.logger.Warn("Message not found for cancellation",
				"message_id", messageID,
				"error", err)
		}
		return &CancelSMResp{}, nil // Empty response for not found
	}

	// Check if message can be cancelled (must be in enroute state)
	report, _ := mh.reportStorage.GetReport(ctx, messageID)
	if report != nil && report.Status != "ACCEPTD" && report.Status != "ENROUTE" {
		if mh.logger != nil {
			mh.logger.Warn("Message cannot be cancelled - not in enroute state",
				"message_id", messageID,
				"status", report.Status)
		}
		return &CancelSMResp{}, nil
	}

	// Delete the message
	err = mh.smsStorage.DeleteSMS(ctx, messageID)
	if err != nil {
		if mh.logger != nil {
			mh.logger.Error("Failed to delete message for cancellation",
				"message_id", messageID,
				"error", err)
		}
		return &CancelSMResp{}, nil
	}

	if mh.logger != nil {
		mh.logger.Info("Message cancelled successfully",
			"message_id", messageID)
	}

	return &CancelSMResp{}, nil
}

// HandleSubmitMulti processes a submit_multi PDU
func (mh *MessageHandlerImpl) HandleSubmitMulti(ctx context.Context, session *Session, pdu *SubmitMulti) (*SubmitMultiResp, error) {
	// Generate base message ID
	baseMessageID := mh.generateMessageID()

	// Get message bytes from ShortMessage or message_payload
	var messageBytes []byte
	if len(pdu.ShortMessage) > 0 {
		messageBytes = pdu.ShortMessage
	} else {
		// Check for message_payload optional parameter
		for _, param := range pdu.OptionalParameters {
			if param.Tag == TagMessagePayload {
				messageBytes = param.Value
				break
			}
		}
	}

	if len(messageBytes) == 0 {
		emptyID := NewCString(65)
		emptyID.SetString("")
		return &SubmitMultiResp{MessageID: *emptyID, NoUnsuccess: uint8(len(pdu.DestAddresses))}, fmt.Errorf("no message content provided")
	}

	// Decode message text
	messageText, err := mh.textEncoder.Decode(messageBytes, pdu.DataCoding)
	if err != nil {
		if mh.logger != nil {
			mh.logger.Error("Failed to decode message text",
				"data_coding", pdu.DataCoding,
				"error", err)
		}
		emptyID := NewCString(65)
		emptyID.SetString("")
		return &SubmitMultiResp{MessageID: *emptyID, NoUnsuccess: uint8(len(pdu.DestAddresses))}, fmt.Errorf("failed to decode message: %w", err)
	}

	var unsuccessfulSMEs []UnsuccessfulSME
	var successfulCount int

	// Process each destination
	for i, dest := range pdu.DestAddresses {
		messageID := fmt.Sprintf("%s-%d", baseMessageID, i+1)

		var destAddr Address
		if dest.DestFlag == 1 { // Individual SME address
			destAddr = Address{
				TON:  dest.DestAddrTON,
				NPI:  dest.DestAddrNPI,
				Addr: dest.DestinationAddr.GetString(),
			}
		} else if dest.DestFlag == 2 { // Distribution list
			// For distribution lists, we'd need to expand the list
			// For now, treat as unsuccessful
			unsuccessfulSMEs = append(unsuccessfulSMEs, UnsuccessfulSME{
				DestAddrTON:     dest.DestAddrTON,
				DestAddrNPI:     dest.DestAddrNPI,
				DestinationAddr: dest.DestinationAddr,
				ErrorStatusCode: StatusInvDstAdr,
			})
			continue
		} else {
			// Invalid destination flag
			unsuccessfulSMEs = append(unsuccessfulSMEs, UnsuccessfulSME{
				DestAddrTON:     dest.DestAddrTON,
				DestAddrNPI:     dest.DestAddrNPI,
				DestinationAddr: dest.DestinationAddr,
				ErrorStatusCode: StatusInvDstAdr,
			})
			continue
		}

		// Create message object
		message := &Message{
			ID:        messageID,
			SessionID: session.ID,
			SourceAddr: Address{
				TON:  pdu.SourceAddr.TON,
				NPI:  pdu.SourceAddr.NPI,
				Addr: pdu.SourceAddr.Addr,
			},
			DestAddr:             destAddr,
			ShortMessage:         pdu.ShortMessage,
			DataCoding:           pdu.DataCoding,
			EsmClass:             pdu.ESMClass,
			RegisteredDelivery:   pdu.RegisteredDelivery,
			ValidityPeriod:       pdu.ValidityPeriod.GetString(),
			ScheduleDeliveryTime: pdu.ScheduleDeliveryTime.GetString(),
			ServiceType:          pdu.ServiceType.GetString(),
			ProtocolID:           pdu.ProtocolID,
			PriorityFlag:         pdu.PriorityFlag,
			Status:               MessageStatusSubmitted,
			SubmitTime:           time.Now(),
			Text:                 messageText,
		}

		// Validate message
		if err := mh.ValidateMessage(ctx, message); err != nil {
			if mh.logger != nil {
				mh.logger.Warn("Message validation failed",
					"message_id", messageID,
					"error", err)
			}
			unsuccessfulSMEs = append(unsuccessfulSMEs, UnsuccessfulSME{
				DestAddrTON:     dest.DestAddrTON,
				DestAddrNPI:     dest.DestAddrNPI,
				DestinationAddr: dest.DestinationAddr,
				ErrorStatusCode: StatusInvMsgLen,
			})
			continue
		}

		// Store message
		if mh.smsStorage != nil {
			if _, err := mh.smsStorage.StoreSMS(ctx, message); err != nil {
				if mh.logger != nil {
					mh.logger.Error("Failed to store SMS",
						"message_id", messageID,
						"error", err)
				}
				unsuccessfulSMEs = append(unsuccessfulSMEs, UnsuccessfulSME{
					DestAddrTON:     dest.DestAddrTON,
					DestAddrNPI:     dest.DestAddrNPI,
					DestinationAddr: dest.DestinationAddr,
					ErrorStatusCode: StatusSysErr,
				})
				continue
			}
		}

		successfulCount++

		// Log successful submission
		if mh.logger != nil {
			mh.logger.Info("SMS submitted successfully (multi)",
				"message_id", messageID,
				"source", pdu.SourceAddr.Addr,
				"dest", destAddr.Addr,
				"data_coding", pdu.DataCoding,
				"text_length", len(messageText),
				"session_id", session.ID)
		}

		// Publish SMS event
		if mh.eventPublisher != nil {
			event := &SMSEvent{
				Type:      EventTypeSMSSubmitted,
				Timestamp: time.Now(),
				MessageID: messageID,
				Session:   session,
				Message:   message,
				Data:      make(map[string]interface{}),
			}
			mh.eventPublisher.PublishSMSEvent(ctx, event)
		}

		// Process message for delivery (asynchronously)
		go func(msg *Message, msgID string) {
			if err := mh.ProcessMessage(context.Background(), msg); err != nil {
				if mh.logger != nil {
					mh.logger.Error("Failed to process message for delivery",
						"message_id", msgID,
						"error", err)
				}
			}
		}(message, messageID)
	}

	// Create response
	messageIDCStr := NewCString(65)
	messageIDCStr.SetString(baseMessageID)

	response := &SubmitMultiResp{
		MessageID:    *messageIDCStr,
		NoUnsuccess:  uint8(len(unsuccessfulSMEs)),
		UnsuccessSME: unsuccessfulSMEs,
	}

	if mh.logger != nil {
		mh.logger.Info("Submit multi completed",
			"base_message_id", baseMessageID,
			"total_destinations", len(pdu.DestAddresses),
			"successful", successfulCount,
			"unsuccessful", len(unsuccessfulSMEs))
	}

	return response, nil
}

// HandleDataSM processes a data_sm PDU
func (mh *MessageHandlerImpl) HandleDataSM(ctx context.Context, session *Session, pdu *DataSM) (*DataSMResp, error) {
	// Generate message ID
	messageID := mh.generateMessageID()

	if mh.logger != nil {
		mh.logger.Info("Received data_sm",
			"session_id", session.ID,
			"message_id", messageID,
			"source", pdu.SourceAddr.Addr,
			"dest", pdu.DestAddr.Addr)
	}

	// For data_sm, the message content is in optional parameters
	// Look for message_payload parameter
	var messageBytes []byte
	for _, param := range pdu.OptionalParameters {
		if param.Tag == TagMessagePayload {
			messageBytes = param.Value
			break
		}
	}

	if len(messageBytes) == 0 {
		if mh.logger != nil {
			mh.logger.Warn("No message content in data_sm",
				"message_id", messageID)
		}
		// For data_sm, empty content is allowed
		messageBytes = []byte{}
	}

	// Decode message text
	messageText, err := mh.textEncoder.Decode(messageBytes, pdu.DataCoding)
	if err != nil {
		if mh.logger != nil {
			mh.logger.Error("Failed to decode message text",
				"message_id", messageID,
				"data_coding", pdu.DataCoding,
				"error", err)
		}
		return &DataSMResp{MessageID: *NewCString(65)}, nil // Return empty message ID on error
	}

	// Create message object
	message := &Message{
		ID:        messageID,
		SessionID: session.ID,
		SourceAddr: Address{
			TON:  pdu.SourceAddr.TON,
			NPI:  pdu.SourceAddr.NPI,
			Addr: pdu.SourceAddr.Addr,
		},
		DestAddr: Address{
			TON:  pdu.DestAddr.TON,
			NPI:  pdu.DestAddr.NPI,
			Addr: pdu.DestAddr.Addr,
		},
		DataCoding:         pdu.DataCoding,
		EsmClass:           pdu.ESMClass,
		RegisteredDelivery: pdu.RegisteredDelivery,
		Status:             MessageStatusSubmitted,
		SubmitTime:         time.Now(),
		Text:               messageText,
	}

	// Validate message
	if err := mh.ValidateMessage(ctx, message); err != nil {
		if mh.logger != nil {
			mh.logger.Warn("Message validation failed",
				"message_id", messageID,
				"error", err)
		}
		return &DataSMResp{MessageID: *NewCString(65)}, nil
	}

	// Store message
	if mh.smsStorage != nil {
		if _, err := mh.smsStorage.StoreSMS(ctx, message); err != nil {
			if mh.logger != nil {
				mh.logger.Error("Failed to store SMS",
					"message_id", messageID,
					"error", err)
			}
			return &DataSMResp{MessageID: *NewCString(65)}, nil
		}
	}

	// Log successful submission
	if mh.logger != nil {
		mh.logger.Info("Data message submitted successfully",
			"message_id", messageID,
			"source", pdu.SourceAddr.Addr,
			"dest", pdu.DestAddr.Addr,
			"data_coding", pdu.DataCoding,
			"text_length", len(messageText),
			"session_id", session.ID)
	}

	// Publish SMS event
	if mh.eventPublisher != nil {
		event := &SMSEvent{
			Type:      EventTypeSMSSubmitted,
			Timestamp: time.Now(),
			MessageID: messageID,
			Session:   session,
			Message:   message,
			Data:      make(map[string]interface{}),
		}
		mh.eventPublisher.PublishSMSEvent(ctx, event)
	}

	// Process message for delivery (asynchronously)
	go func() {
		if err := mh.ProcessMessage(context.Background(), message); err != nil {
			if mh.logger != nil {
				mh.logger.Error("Failed to process message for delivery",
					"message_id", messageID,
					"error", err)
			}
		}
	}()

	messageIDCStr := NewCString(65)
	messageIDCStr.SetString(messageID)

	return &DataSMResp{MessageID: *messageIDCStr}, nil
}

// HandleDeliverSM processes a deliver_sm PDU
func (mh *MessageHandlerImpl) HandleDeliverSM(ctx context.Context, session *Session, pdu *DeliverSM) (*DeliverSMResp, error) {
	messageID := mh.generateMessageID()

	// Check if this is a delivery receipt
	if pdu.EsmClass&0x04 != 0 {
		// Handle delivery receipt
		return mh.handleDeliveryReceipt(ctx, session, pdu, messageID)
	}

	// Handle regular SMS delivery
	return mh.handleSMSDelivery(ctx, session, pdu, messageID)
}

// handleDeliveryReceipt handles a delivery receipt
func (mh *MessageHandlerImpl) handleDeliveryReceipt(ctx context.Context, session *Session, pdu *DeliverSM, messageID string) (*DeliverSMResp, error) {
	// Parse delivery receipt from message text
	receiptText := string(pdu.ShortMessage)
	receipt, err := mh.parseDeliveryReceiptText(receiptText)
	if err != nil {
		if mh.logger != nil {
			mh.logger.Error("Failed to parse delivery receipt",
				"receipt_text", receiptText,
				"error", err)
		}
		return &DeliverSMResp{MessageID: messageID}, err
	}

	// Extract original message ID from optional parameters
	originalMessageID := ""
	for _, param := range pdu.OptionalParams {
		if param.Tag == TagReceiptedMessageID {
			originalMessageID = string(param.Value)
			break
		}
	}

	if originalMessageID != "" {
		receipt.MessageID = originalMessageID
	}

	// Store delivery report
	if mh.reportStorage != nil {
		if err := mh.reportStorage.StoreReport(ctx, receipt); err != nil {
			if mh.logger != nil {
				mh.logger.Error("Failed to store delivery report",
					"message_id", originalMessageID,
					"error", err)
			}
		}
	}

	// Update original message status
	if mh.smsStorage != nil && originalMessageID != "" {
		status := mh.mapReceiptStatusToMessageStatus(receipt.Status)
		if err := mh.smsStorage.UpdateSMSStatus(ctx, originalMessageID, status); err != nil {
			if mh.logger != nil {
				mh.logger.Error("Failed to update SMS status",
					"message_id", originalMessageID,
					"status", status,
					"error", err)
			}
		}
	}

	// Log delivery receipt
	if mh.logger != nil {
		mh.logger.Info("Delivery receipt received",
			"original_message_id", originalMessageID,
			"status", receipt.Status,
			"session_id", session.ID)
	}

	// Publish delivery event
	if mh.eventPublisher != nil {
		event := &DeliveryEvent{
			Type:      EventTypeDeliveryReport,
			Timestamp: time.Now(),
			MessageID: originalMessageID,
			Report:    receipt,
			Session:   session,
			Data:      make(map[string]interface{}),
		}
		mh.eventPublisher.PublishDeliveryEvent(ctx, event)
	}

	return &DeliverSMResp{MessageID: messageID}, nil
}

// handleSMSDelivery handles regular SMS delivery
func (mh *MessageHandlerImpl) handleSMSDelivery(ctx context.Context, session *Session, pdu *DeliverSM, messageID string) (*DeliverSMResp, error) {
	// Decode message text
	messageText, err := mh.textEncoder.Decode(pdu.ShortMessage, pdu.DataCoding)
	if err != nil {
		if mh.logger != nil {
			mh.logger.Error("Failed to decode delivered message text",
				"message_id", messageID,
				"data_coding", pdu.DataCoding,
				"error", err)
		}
		return &DeliverSMResp{MessageID: messageID}, err
	}

	// Create message object for incoming SMS
	message := &Message{
		ID: messageID,
		SourceAddr: Address{
			TON:  pdu.SourceAddrTON,
			NPI:  pdu.SourceAddrNPI,
			Addr: pdu.SourceAddr,
		},
		DestAddr: Address{
			TON:  pdu.DestAddrTON,
			NPI:  pdu.DestAddrNPI,
			Addr: pdu.DestAddr,
		},
		ShortMessage: pdu.ShortMessage,
		DataCoding:   pdu.DataCoding,
		EsmClass:     pdu.EsmClass,
		ServiceType:  pdu.ServiceType,
		ProtocolID:   pdu.ProtocolID,
		PriorityFlag: pdu.PriorityFlag,
		Status:       MessageStatusDelivered,
		SubmitTime:   time.Now(),
		Text:         messageText,
	}

	// Store incoming message
	if mh.smsStorage != nil {
		if _, err := mh.smsStorage.StoreSMS(ctx, message); err != nil {
			if mh.logger != nil {
				mh.logger.Error("Failed to store incoming SMS",
					"message_id", messageID,
					"error", err)
			}
		}
	}

	// Log incoming SMS
	if mh.logger != nil {
		mh.logger.Info("SMS delivered",
			"message_id", messageID,
			"source", pdu.SourceAddr,
			"dest", pdu.DestAddr,
			"text_length", len(messageText),
			"session_id", session.ID)
	}

	// Publish SMS delivery event
	if mh.eventPublisher != nil {
		event := &SMSEvent{
			Type:      EventTypeSMSDelivered,
			Timestamp: time.Now(),
			MessageID: messageID,
			Session:   session,
			Message:   message,
			Data:      make(map[string]interface{}),
		}
		mh.eventPublisher.PublishSMSEvent(ctx, event)
	}

	return &DeliverSMResp{MessageID: messageID}, nil
}

// ProcessMessage processes an SMS message for delivery
func (mh *MessageHandlerImpl) ProcessMessage(ctx context.Context, message *Message) error {
	if mh.logger != nil {
		mh.logger.Debug("Processing message for delivery", "message_id", message.ID)
	}

	// 1. Route the message to the appropriate gateway
	route, err := mh.routeMessage(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to route message: %w", err)
	}

	// 2. Handle message splitting for long messages
	messages, err := mh.splitMessage(message)
	if err != nil {
		return fmt.Errorf("failed to split message: %w", err)
	}

	// 3. Queue each message part for delivery
	for _, msgPart := range messages {
		if err := mh.enqueueMessage(ctx, msgPart, route); err != nil {
			if mh.logger != nil {
				mh.logger.Error("Failed to enqueue message",
					"message_id", msgPart.ID,
					"error", err)
			}
			return fmt.Errorf("failed to enqueue message %s: %w", msgPart.ID, err)
		}
	}

	// 4. Update original message status
	if mh.smsStorage != nil {
		status := MessageStatusAccepted
		if len(messages) > 1 {
			status = MessageStatusSplit
		}
		if err := mh.smsStorage.UpdateSMSStatus(ctx, message.ID, status); err != nil {
			return fmt.Errorf("failed to update message status: %w", err)
		}
	}

	// 5. Generate delivery reports if requested (for immediate delivery simulation)
	if message.RegisteredDelivery&0x01 != 0 {
		// For split messages, generate reports for each part
		for _, msgPart := range messages {
			report, err := mh.GenerateDeliveryReport(ctx, msgPart, MessageStatusDelivered)
			if err != nil {
				if mh.logger != nil {
					mh.logger.Error("Failed to generate delivery report",
						"message_id", msgPart.ID,
						"error", err)
				}
				// Don't fail the entire process for delivery report errors
				continue
			}

			// Store delivery report
			if mh.reportStorage != nil {
				if err := mh.reportStorage.StoreReport(ctx, report); err != nil {
					if mh.logger != nil {
						mh.logger.Error("Failed to store delivery report",
							"message_id", msgPart.ID,
							"error", err)
					}
				}
			}

			// Publish delivery event
			if mh.eventPublisher != nil {
				event := &DeliveryEvent{
					Type:      EventTypeDeliveryReport,
					Timestamp: time.Now(),
					MessageID: msgPart.ID,
					Report:    report,
					Data: map[string]interface{}{
						"session_id": message.SessionID,
					},
				}
				mh.eventPublisher.PublishDeliveryEvent(ctx, event)
			}
		}
	}

	return nil
}

// GenerateDeliveryReport generates a delivery report for a message
func (mh *MessageHandlerImpl) GenerateDeliveryReport(ctx context.Context, message *Message, status MessageStatus) (*DeliveryReport, error) {
	now := time.Now()
	submitDate := message.SubmitTime.Format("0601021504")
	doneDate := now.Format("0601021504")

	statusText := mh.mapMessageStatusToReceiptStatus(status)

	// For delivery receipts, truncate the message text if it's too long
	reportText := message.Text
	if len(reportText) > 20 {
		reportText = reportText[:17] + "..."
	}

	report := &DeliveryReport{
		MessageID:  message.ID,
		SubmitDate: submitDate,
		DoneDate:   doneDate,
		Status:     statusText,
		Error:      "000",
		Text: fmt.Sprintf("id:%s sub:%s dlvrd:%s submit date:%s done date:%s stat:%s err:%s text:%s",
			message.ID, "001", "001", submitDate, doneDate, statusText, "000", reportText),
		SubmittedParts: 1,
		DeliveredParts: 1,
		Timestamp:      now,
		SubmitTime:     message.SubmitTime,
		DoneTime:       now,
	}

	return report, nil
}

// ValidateMessage validates an SMS message
func (mh *MessageHandlerImpl) ValidateMessage(ctx context.Context, message *Message) error {
	// Validate source address
	if message.SourceAddr.Addr == "" {
		return fmt.Errorf("source address cannot be empty")
	}

	// Validate destination address
	if message.DestAddr.Addr == "" {
		return fmt.Errorf("destination address cannot be empty")
	}

	// Validate message text encoding
	if err := mh.textEncoder.ValidateText(message.Text, message.DataCoding); err != nil {
		return fmt.Errorf("invalid message encoding: %w", err)
	}

	// Validate message length - allow longer messages that will be split
	maxLength := mh.textEncoder.GetMaxLength(message.DataCoding, message.EsmClass&EsmClassUDHI != 0)
	textLength := len([]rune(message.Text))

	// For single SMS, enforce the limit
	// For long messages, we'll split them later in ProcessMessage
	if textLength > maxLength {
		// Check if this message can be split (must be text-based, not binary)
		if message.DataCoding == 0 || message.DataCoding == 1 { // GSM 7-bit or 8-bit
			// Allow messages up to a reasonable maximum (e.g., 10 SMS parts)
			maxConcatLength := maxLength * 10
			if textLength > maxConcatLength {
				return fmt.Errorf("message too long: %d characters, max %d (after splitting)", textLength, maxConcatLength)
			}
			// Log that this will be split
			if mh.logger != nil {
				mh.logger.Debug("Long message will be split",
					"message_id", message.ID,
					"text_length", textLength,
					"max_single_length", maxLength)
			}
		} else {
			return fmt.Errorf("message too long: %d characters, max %d", textLength, maxLength)
		}
	}

	// Validate TON/NPI combinations
	if !mh.isValidTONNPI(message.SourceAddr.TON, message.SourceAddr.NPI) {
		return fmt.Errorf("invalid source TON/NPI combination: %d/%d", message.SourceAddr.TON, message.SourceAddr.NPI)
	}

	if !mh.isValidTONNPI(message.DestAddr.TON, message.DestAddr.NPI) {
		return fmt.Errorf("invalid destination TON/NPI combination: %d/%d", message.DestAddr.TON, message.DestAddr.NPI)
	}

	return nil
}

// Helper methods

// generateMessageID generates a unique message ID
func (mh *MessageHandlerImpl) generateMessageID() string {
	return fmt.Sprintf("MSG_%d_%d", time.Now().UnixNano(), time.Now().Unix()%100000)
}

// parseDeliveryReceiptText parses delivery receipt text
func (mh *MessageHandlerImpl) parseDeliveryReceiptText(text string) (*DeliveryReport, error) {
	// Parse delivery receipt format: "id:MSG123 sub:001 dlvrd:001 submit date:... done date:... stat:DELIVRD err:000 text:..."
	receipt := &DeliveryReport{
		Text: text,
	}

	// Simple parsing logic
	parts := strings.Fields(text)
	for _, part := range parts {
		if strings.HasPrefix(part, "id:") {
			receipt.MessageID = strings.TrimPrefix(part, "id:")
		} else if strings.HasPrefix(part, "stat:") {
			receipt.Status = strings.TrimPrefix(part, "stat:")
		} else if strings.HasPrefix(part, "err:") {
			receipt.Error = strings.TrimPrefix(part, "err:")
		} else if strings.HasPrefix(part, "submit") && strings.Contains(part, "date:") {
			receipt.SubmitDate = strings.TrimPrefix(part, "date:")
		} else if strings.HasPrefix(part, "done") && strings.Contains(part, "date:") {
			receipt.DoneDate = strings.TrimPrefix(part, "date:")
		}
	}

	return receipt, nil
}

// mapReceiptStatusToMessageStatus maps receipt status to message status
func (mh *MessageHandlerImpl) mapReceiptStatusToMessageStatus(receiptStatus string) MessageStatus {
	switch strings.ToUpper(receiptStatus) {
	case "DELIVRD":
		return MessageStatusDelivered
	case "EXPIRED":
		return MessageStatusExpired
	case "DELETED":
		return MessageStatusDeleted
	case "UNDELIV":
		return MessageStatusUndeliverable
	case "ACCEPTD":
		return MessageStatusAccepted
	case "REJECTD":
		return MessageStatusRejected
	default:
		return MessageStatusUnknown
	}
}

// mapMessageStatusToReceiptStatus maps message status to receipt status
func (mh *MessageHandlerImpl) mapMessageStatusToReceiptStatus(status MessageStatus) string {
	switch status {
	case MessageStatusDelivered:
		return "DELIVRD"
	case MessageStatusExpired:
		return "EXPIRED"
	case MessageStatusDeleted:
		return "DELETED"
	case MessageStatusUndeliverable:
		return "UNDELIV"
	case MessageStatusAccepted:
		return "ACCEPTD"
	case MessageStatusRejected:
		return "REJECTD"
	default:
		return "UNKNOWN"
	}
}

// isValidTONNPI validates TON/NPI combination
func (mh *MessageHandlerImpl) isValidTONNPI(ton, npi uint8) bool {
	// Basic validation - in practice, this would be more comprehensive
	switch ton {
	case TONUnknown:
		return npi == NPIUnknown
	case TONInternational:
		return npi == NPIISDN || npi == NPIUnknown
	case TONNational:
		return npi == NPIISDN || npi == NPIUnknown
	case TONAlphanumeric:
		return npi == NPIUnknown
	default:
		return true // Allow other combinations for flexibility
	}
}

// SMSProcessor handles SMS message processing and routing
type SMSProcessor struct {
	messageHandler MessageHandler
	textEncoder    *encoding.TextEncoder
	logger         Logger
}

// NewSMSProcessor creates a new SMS processor
func NewSMSProcessor(messageHandler MessageHandler, logger Logger) *SMSProcessor {
	return &SMSProcessor{
		messageHandler: messageHandler,
		textEncoder:    encoding.NewTextEncoder(),
		logger:         logger,
	}
}

// ProcessLongSMS handles long SMS messages that need to be split
func (sp *SMSProcessor) ProcessLongSMS(ctx context.Context, message *Message) ([]*Message, error) {
	// Check if message needs to be split
	maxLength := sp.textEncoder.GetMaxLength(message.DataCoding, false)
	textRunes := []rune(message.Text)

	if len(textRunes) <= maxLength {
		return []*Message{message}, nil
	}

	// Split message into parts
	parts, err := sp.textEncoder.SplitMessage(message.Text, message.DataCoding)
	if err != nil {
		return nil, fmt.Errorf("failed to split message: %w", err)
	}

	if len(parts) > 255 {
		return nil, fmt.Errorf("message too long: would require %d parts (max 255)", len(parts))
	}

	// Generate reference number based on message ID hash
	refNum := sp.generateReferenceNumber(message.ID)

	// Create message parts with UDH
	messages := make([]*Message, len(parts))

	for i, part := range parts {
		partMessage := *message // Copy original message
		partMessage.ID = fmt.Sprintf("%s_part_%d", message.ID, i+1)
		partMessage.Text = part

		// Encode part text
		encodedPart, err := sp.textEncoder.Encode(part, message.DataCoding)
		if err != nil {
			return nil, fmt.Errorf("failed to encode message part %d: %w", i+1, err)
		}

		// Create UDH for multipart message
		udh := sp.createMultipartUDH(refNum, uint8(len(parts)), uint8(i+1))

		// Combine UDH and message
		partMessage.ShortMessage = append([]byte{uint8(len(udh))}, udh...)
		partMessage.ShortMessage = append(partMessage.ShortMessage, encodedPart...)
		partMessage.EsmClass |= EsmClassUDHI // Set UDHI flag

		messages[i] = &partMessage
	}

	return messages, nil
}

// generateReferenceNumber generates a reference number based on message ID
func (sp *SMSProcessor) generateReferenceNumber(messageID string) uint16 {
	// Create a hash of the message ID to ensure uniqueness
	hash := uint32(0)
	for _, b := range []byte(messageID) {
		hash = hash*31 + uint32(b)
	}
	return uint16(hash & 0xFFFF)
}

// createMultipartUDH creates User Data Header for multipart messages
func (sp *SMSProcessor) createMultipartUDH(refNum uint16, totalParts, partNum uint8) []byte {
	// Concatenated SMS UDH format:
	// IEI: 0x00 (Information Element Identifier for concatenated SMS)
	// IEDL: 0x03 (Information Element Data Length)
	// Reference number: 2 bytes
	// Total parts: 1 byte
	// Part number: 1 byte
	udh := make([]byte, 6)        // Fixed: was 5, should be 6
	udh[0] = 0x00                 // IEI
	udh[1] = 0x03                 // IEDL
	udh[2] = uint8(refNum >> 8)   // Reference number high byte
	udh[3] = uint8(refNum & 0xFF) // Reference number low byte
	udh[4] = totalParts
	udh[5] = partNum

	return udh
}

// ConcatenateSMSParts concatenates received SMS parts
func (sp *SMSProcessor) ConcatenateSMSParts(parts []*Message) (*Message, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("no parts provided")
	}

	if len(parts) == 1 {
		return parts[0], nil
	}

	// Extract UDH info and sort parts by part number
	type partInfo struct {
		message    *Message
		partNum    uint8
		totalParts uint8
		refNum     uint16
	}

	var partInfos []partInfo
	var expectedTotal uint8
	var refNum uint16

	for _, part := range parts {
		if part.EsmClass&EsmClassUDHI == 0 {
			return nil, fmt.Errorf("message part missing UDHI flag")
		}

		if len(part.ShortMessage) < 7 { // UDH length + 6 bytes min UDH
			return nil, fmt.Errorf("message part too short for UDH")
		}

		udhLength := part.ShortMessage[0]
		if int(udhLength) >= len(part.ShortMessage) {
			return nil, fmt.Errorf("invalid UDH length")
		}

		udh := part.ShortMessage[1 : 1+udhLength]
		if len(udh) < 6 || udh[0] != 0x00 || udh[1] != 0x03 {
			return nil, fmt.Errorf("invalid concatenated SMS UDH")
		}

		partRefNum := uint16(udh[2])<<8 | uint16(udh[3])
		totalParts := udh[4]
		partNumber := udh[5]

		// Validate consistency across parts
		if len(partInfos) == 0 {
			expectedTotal = totalParts
			refNum = partRefNum
		} else {
			if totalParts != expectedTotal {
				return nil, fmt.Errorf("inconsistent total parts: expected %d, got %d", expectedTotal, totalParts)
			}
			if partRefNum != refNum {
				return nil, fmt.Errorf("inconsistent reference number: expected %d, got %d", refNum, partRefNum)
			}
		}

		partInfos = append(partInfos, partInfo{
			message:    part,
			partNum:    partNumber,
			totalParts: totalParts,
			refNum:     partRefNum,
		})
	}

	// Sort by part number
	for i := 0; i < len(partInfos)-1; i++ {
		for j := i + 1; j < len(partInfos); j++ {
			if partInfos[i].partNum > partInfos[j].partNum {
				partInfos[i], partInfos[j] = partInfos[j], partInfos[i]
			}
		}
	}

	// Verify we have all parts
	if len(partInfos) != int(expectedTotal) {
		return nil, fmt.Errorf("missing parts: expected %d, got %d", expectedTotal, len(partInfos))
	}

	for i, info := range partInfos {
		if info.partNum != uint8(i+1) {
			return nil, fmt.Errorf("missing part %d", i+1)
		}
	}

	// Extract text from each part and concatenate
	var fullText strings.Builder
	for _, info := range partInfos {
		// Extract text portion (skip UDH)
		udhLength := info.message.ShortMessage[0]
		textBytes := info.message.ShortMessage[1+udhLength:]

		// Decode text based on data coding
		partText, err := sp.textEncoder.Decode(textBytes, info.message.DataCoding)
		if err != nil {
			return nil, fmt.Errorf("failed to decode part %d: %w", info.partNum, err)
		}

		fullText.WriteString(partText)
	}

	// Create concatenated message based on first part
	concatenated := *partInfos[0].message
	concatenated.Text = fullText.String()
	concatenated.EsmClass &^= EsmClassUDHI // Remove UDHI flag

	// Re-encode the full message
	encoded, err := sp.textEncoder.Encode(concatenated.Text, concatenated.DataCoding)
	if err != nil {
		return nil, fmt.Errorf("failed to encode concatenated message: %w", err)
	}

	concatenated.ShortMessage = encoded

	return &concatenated, nil
}

// routeMessage routes a message and returns routing information
func (mh *MessageHandlerImpl) routeMessage(ctx context.Context, message *Message) (*RouteInfo, error) {
	if mh.router != nil {
		return mh.router.RouteMessage(ctx, message)
	}

	// Default routing logic
	return &RouteInfo{
		Gateway:     "default",
		Priority:    1,
		RetryCount:  3,
		Destination: message.DestAddr.Addr,
		Route:       "direct",
	}, nil
}

// splitMessage splits a long message into multiple parts
func (mh *MessageHandlerImpl) splitMessage(message *Message) ([]*Message, error) {
	if mh.splitter != nil {
		return mh.splitter.SplitMessage(message)
	}

	// Default: no splitting, return original message
	return []*Message{message}, nil
}

// enqueueMessage adds a message to the delivery queue
func (mh *MessageHandlerImpl) enqueueMessage(ctx context.Context, message *Message, route *RouteInfo) error {
	if mh.queue != nil {
		return mh.queue.EnqueueMessage(ctx, message, route)
	}

	// Default: immediate "delivery" by updating status
	if mh.smsStorage != nil {
		return mh.smsStorage.UpdateSMSStatus(ctx, message.ID, MessageStatusDelivered)
	}

	return nil
}
