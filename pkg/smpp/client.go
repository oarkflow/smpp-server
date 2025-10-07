package smpp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// Client represents an SMPP client
type Client struct {
	config         *ClientConfig
	conn           net.Conn
	session        *Session
	eventPublisher EventPublisher
	logger         Logger
	metrics        MetricsCollector
	encoder        *PDUEncoder
	decoder        *PDUDecoder
	builder        *PDUBuilder

	// Client state
	mu        sync.RWMutex
	connected bool
	bound     bool
	done      chan struct{}
	wg        sync.WaitGroup

	// Reconnection
	reconnectTimer *time.Timer
	reconnectCount int

	// Callbacks
	onConnect        func()
	onDisconnect     func(error)
	onBind           func()
	onUnbind         func()
	onSubmitResponse func(messageID string, err error)
	onMessage        func(*Message)
	onDeliveryReport func(*DeliveryReport)
}

// NewClient creates a new SMPP client
func NewClient(config *ClientConfig, deps ClientDependencies) *Client {
	return &Client{
		config:         config,
		eventPublisher: deps.EventPublisher,
		logger:         deps.Logger,
		metrics:        deps.MetricsCollector,
		encoder:        NewPDUEncoder(),
		decoder:        NewPDUDecoder(),
		builder:        NewPDUBuilder(),
		done:           make(chan struct{}),
	}
}

// ClientDependencies holds all dependencies for the client
type ClientDependencies struct {
	EventPublisher   EventPublisher
	Logger           Logger
	MetricsCollector MetricsCollector
}

// Connect connects to the SMPP server
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return fmt.Errorf("client is already connected")
	}
	c.mu.Unlock()

	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)

	if c.logger != nil {
		c.logger.Info("Connecting to SMPP server", "address", addr)
	}

	// Create connection with timeout
	var conn net.Conn
	var err error

	if c.config.TLSEnabled {
		// Create TLS config
		tlsConfig := &tls.Config{
			InsecureSkipVerify: c.config.TLSSkipVerify,
			ServerName:         c.config.Host,
		}

		// Create TLS dialer
		dialer := &tls.Dialer{
			NetDialer: &net.Dialer{
				Timeout: c.config.ConnectTimeout,
			},
			Config: tlsConfig,
		}

		conn, err = dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to connect to %s with TLS: %w", addr, err)
		}
	} else {
		dialer := &net.Dialer{
			Timeout: c.config.ConnectTimeout,
		}

		conn, err = dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to connect to %s: %w", addr, err)
		}
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.session = &Session{
		ID:           generateSessionID(),
		State:        SessionStateOpen,
		LastActivity: time.Now(),
		SequenceNum:  1,
	}
	c.mu.Unlock()

	if c.logger != nil {
		if c.config.TLSEnabled {
			c.logger.Info("Connected to SMPP server with TLS", "address", addr, "session_id", c.session.ID)
		} else {
			c.logger.Info("Connected to SMPP server", "address", addr, "session_id", c.session.ID)
		}
	}

	// Start goroutines for handling
	c.wg.Add(2)
	go c.readLoop(ctx)
	go c.periodicTasks(ctx)

	// Publish connection event
	if c.eventPublisher != nil {
		event := &ConnectionEvent{
			Type:       EventTypeConnected,
			Timestamp:  time.Now(),
			SessionID:  c.session.ID,
			Session:    c.session,
			RemoteAddr: addr,
			Data:       make(map[string]interface{}),
		}
		c.eventPublisher.PublishConnectionEvent(ctx, event)
	}

	// Call callback
	if c.onConnect != nil {
		c.onConnect()
	}

	return nil
}

// Bind binds to the SMPP server
func (c *Client) Bind(ctx context.Context) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("client is not connected")
	}
	if c.bound {
		c.mu.RUnlock()
		return fmt.Errorf("client is already bound")
	}
	c.mu.RUnlock()

	// Create bind PDU based on configured bind type
	var bindPDU *PDU
	switch c.config.BindType {
	case "transmitter":
		bindPDU = c.builder.BuildBindTransmitter(c.config.SystemID, c.config.Password, c.config.SystemType)
	case "receiver":
		bindPDU = c.builder.BuildBindReceiver(c.config.SystemID, c.config.Password, c.config.SystemType)
	case "transceiver":
		fallthrough
	default:
		bindPDU = c.builder.BuildBindTransceiver(c.config.SystemID, c.config.Password, c.config.SystemType)
	}

	// Send bind request asynchronously
	if err := c.sendPDU(bindPDU); err != nil {
		return fmt.Errorf("failed to send bind request: %w", err)
	}

	if c.logger != nil {
		c.logger.Info("Bind request sent", "system_id", c.config.SystemID, "bind_type", c.config.BindType)
	}

	return nil
}

// OnConnect sets the callback for connection events
func (c *Client) OnConnect(callback func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onConnect = callback
}

// OnDisconnect sets the callback for disconnection events
func (c *Client) OnDisconnect(callback func(error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDisconnect = callback
}

// OnBind sets the callback for bind events
func (c *Client) OnBind(callback func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onBind = callback
}

// OnUnbind sets the callback for unbind events
func (c *Client) OnUnbind(callback func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onUnbind = callback
}

// OnSubmitResponse sets the callback for submit response events
func (c *Client) OnSubmitResponse(callback func(messageID string, err error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onSubmitResponse = callback
}

// OnMessage sets the callback for incoming SMS messages
func (c *Client) OnMessage(callback func(*Message)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onMessage = callback
}

// OnDeliveryReport sets the callback for delivery reports
func (c *Client) OnDeliveryReport(callback func(*DeliveryReport)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDeliveryReport = callback
}

// SendSMS sends an SMS message asynchronously
func (c *Client) SendSMS(ctx context.Context, sourceAddr, destAddr, message string) error {
	c.mu.RLock()
	if !c.bound || (c.session.State != SessionStateBoundTX && c.session.State != SessionStateBoundTRX) {
		c.mu.RUnlock()
		return fmt.Errorf("client is not bound for transmission")
	}
	c.mu.RUnlock()

	if c.logger != nil {
		c.logger.Info("Sending SMS",
			"source", sourceAddr,
			"dest", destAddr,
			"message", message)
	}

	// Create submit_sm PDU
	submitPDU := c.builder.BuildSubmitSM(sourceAddr, destAddr, message)

	if c.logger != nil {
		c.logger.Info("Created submit_sm PDU",
			"sequence", submitPDU.Header.SequenceNum,
			"session_id", c.session.ID)
	}

	// Send PDU asynchronously
	if err := c.sendPDU(submitPDU); err != nil {
		return fmt.Errorf("failed to send SMS PDU: %w", err)
	}

	return nil
}

// Unbind unbinds from the SMPP server
func (c *Client) Unbind(ctx context.Context) error {
	c.mu.RLock()
	if !c.bound {
		c.mu.RUnlock()
		return fmt.Errorf("client is not bound")
	}
	c.mu.RUnlock()

	// Create unbind PDU
	unbindPDU := c.builder.BuildUnbind()

	// Send unbind PDU asynchronously (don't wait for response)
	if err := c.sendPDU(unbindPDU); err != nil {
		return fmt.Errorf("failed to send unbind: %w", err)
	}

	c.mu.Lock()
	c.bound = false
	c.session.State = SessionStateUnbound
	c.mu.Unlock()

	if c.logger != nil {
		c.logger.Info("Unbound from SMPP server", "session_id", c.session.ID)
	}

	// Publish unbind event
	if c.eventPublisher != nil {
		event := &ConnectionEvent{
			Type:      EventTypeUnbound,
			Timestamp: time.Now(),
			SessionID: c.session.ID,
			Session:   c.session,
			Data:      make(map[string]interface{}),
		}
		c.eventPublisher.PublishConnectionEvent(ctx, event)
	}

	// Call callback
	if c.onUnbind != nil {
		c.onUnbind()
	}

	return nil
}

// Disconnect disconnects from the SMPP server
func (c *Client) Disconnect(ctx context.Context) error {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return fmt.Errorf("client is not connected")
	}

	// Close connection
	if c.conn != nil {
		c.conn.Close()
	}

	c.connected = false
	c.bound = false
	c.mu.Unlock()

	// Signal shutdown
	close(c.done)

	// Wait for goroutines to finish
	c.wg.Wait()

	if c.logger != nil {
		c.logger.Info("Disconnected from SMPP server")
	}

	// Publish disconnect event
	if c.eventPublisher != nil && c.session != nil {
		event := &ConnectionEvent{
			Type:      EventTypeDisconnected,
			Timestamp: time.Now(),
			SessionID: c.session.ID,
			Session:   c.session,
			Data:      make(map[string]interface{}),
		}
		c.eventPublisher.PublishConnectionEvent(ctx, event)
	}

	// Call callback
	if c.onDisconnect != nil {
		c.onDisconnect(nil)
	}

	return nil
}

// sendPDU sends a PDU to the server
func (c *Client) sendPDU(pdu *PDU) error {
	c.mu.RLock()
	if !c.connected || c.conn == nil {
		c.mu.RUnlock()
		return fmt.Errorf("client is not connected")
	}
	conn := c.conn
	c.mu.RUnlock()

	// Encode PDU
	data, err := c.encoder.Encode(pdu)
	if err != nil {
		return fmt.Errorf("failed to encode PDU: %w", err)
	}

	// Set write deadline
	if err := conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}

	// Write data
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	if c.logger != nil {
		c.logger.Info("PDU sent",
			"command_id", fmt.Sprintf("0x%08X", pdu.Header.CommandID),
			"sequence", pdu.Header.SequenceNum,
			"session_id", c.session.ID)
	}

	return nil
}

// readLoop reads PDUs from the connection
func (c *Client) readLoop(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-c.done:
			return
		default:
			c.mu.RLock()
			if !c.connected || c.conn == nil {
				c.mu.RUnlock()
				return
			}
			conn := c.conn
			c.mu.RUnlock()

			// Set read deadline
			conn.SetReadDeadline(time.Now().Add(c.config.ReadTimeout))

			// Read PDU
			pdu, err := c.decoder.DecodeFromReader(conn)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}

				if c.logger != nil {
					c.logger.Error("Failed to read PDU", "error", err)
				}

				// Trigger reconnection
				go c.handleDisconnection(ctx, err)
				return
			}

			// Update last activity
			c.mu.Lock()
			if c.session != nil {
				c.session.LastActivity = time.Now()
			}
			c.mu.Unlock()

			// Handle PDU
			if err := c.handlePDU(ctx, pdu); err != nil {
				if c.logger != nil {
					c.logger.Error("Error handling PDU",
						"command_id", fmt.Sprintf("0x%08X", pdu.Header.CommandID),
						"error", err)
				}
			}
		}
	}
}

// handlePDU handles a received PDU
func (c *Client) handlePDU(ctx context.Context, pdu *PDU) error {
	if c.logger != nil {
		c.logger.Debug("PDU received",
			"command_id", fmt.Sprintf("0x%08X", pdu.Header.CommandID),
			"sequence", pdu.Header.SequenceNum,
			"status", fmt.Sprintf("0x%08X", pdu.Header.CommandStatus))
	}

	// Handle response PDUs
	switch pdu.Header.CommandID {
	case CommandBindTransceiverResp, CommandBindReceiverResp, CommandBindTransmitterResp:
		return c.handleBindResponse(ctx, pdu)
	case CommandSubmitSMResp:
		return c.handleSubmitSMResponse(ctx, pdu)
	case CommandDeliverSM:
		return c.handleDeliverSM(ctx, pdu.Body.(*DeliverSM))
	case CommandEnquireLink:
		return c.handleEnquireLink(ctx, pdu.Body.(*EnquireLink))
	default:
		if c.logger != nil {
			c.logger.Warn("Unhandled PDU type",
				"command_id", fmt.Sprintf("0x%08X", pdu.Header.CommandID))
		}
	}

	return nil
}

// handleBindResponse handles bind response PDUs
func (c *Client) handleBindResponse(ctx context.Context, pdu *PDU) error {
	// Check if bind was successful
	if pdu.Header.CommandStatus != StatusOK {
		err := fmt.Errorf("bind failed with status: 0x%08X", pdu.Header.CommandStatus)
		if c.logger != nil {
			c.logger.Error("Bind failed", "error", err)
		}

		// Call bind callback with error
		if c.onBind != nil {
			// For error case, we can't call onBind since it has no error parameter
			// The error would be handled by the caller or through other means
		}
		return nil
	}

	// Update client state
	c.mu.Lock()
	c.bound = true
	c.session.State = SessionStateBoundTRX // Assume transceiver for now
	c.mu.Unlock()

	if c.logger != nil {
		c.logger.Info("Bind successful", "system_id", c.config.SystemID)
	}

	// Publish bind event
	if c.eventPublisher != nil {
		event := &ConnectionEvent{
			Type:      EventTypeBound,
			Timestamp: time.Now(),
			SessionID: c.session.ID,
			Session:   c.session,
			Data: map[string]interface{}{
				"bind_type": "transceiver",
			},
		}
		c.eventPublisher.PublishConnectionEvent(ctx, event)
	}

	// Call bind callback with success
	if c.onBind != nil {
		c.onBind()
	}

	return nil
}

// handleSubmitSMResponse handles submit_sm response PDUs
func (c *Client) handleSubmitSMResponse(ctx context.Context, pdu *PDU) error {
	// Check if submit was successful
	if pdu.Header.CommandStatus != StatusOK {
		err := fmt.Errorf("submit failed with status: 0x%08X", pdu.Header.CommandStatus)
		if c.logger != nil {
			c.logger.Error("Submit failed", "error", err)
		}

		// Call submit callback with error
		if c.onSubmitResponse != nil {
			c.onSubmitResponse("", err)
		}
		return nil
	}

	// Extract message ID from response
	submitResp, ok := pdu.Body.(*SubmitSMResp)
	if !ok {
		err := fmt.Errorf("invalid submit_sm_resp PDU type")
		if c.logger != nil {
			c.logger.Error("Invalid submit response", "error", err)
		}

		if c.onSubmitResponse != nil {
			c.onSubmitResponse("", err)
		}
		return nil
	}

	if c.logger != nil {
		c.logger.Info("SMS submitted successfully", "message_id", submitResp.MessageID)
	}

	// Publish SMS event
	if c.eventPublisher != nil {
		// Note: We don't have source/dest info here, so we create a minimal event
		event := &SMSEvent{
			Type:      EventTypeSMSSubmitted,
			Timestamp: time.Now(),
			MessageID: submitResp.MessageID,
			Session:   c.session,
			Data:      make(map[string]interface{}),
		}
		c.eventPublisher.PublishSMSEvent(ctx, event)
	}

	// Call submit callback with success
	if c.onSubmitResponse != nil {
		c.onSubmitResponse(submitResp.MessageID, nil)
	}

	return nil
}

// handleDeliverSM handles deliver_sm PDU
func (c *Client) handleDeliverSM(ctx context.Context, deliverSM *DeliverSM) error {
	// Send deliver_sm_resp
	respPDU := c.builder.BuildDeliverSMResp("", c.getNextSequence(), StatusOK)
	if err := c.sendPDU(respPDU); err != nil {
		return fmt.Errorf("failed to send deliver_sm_resp: %w", err)
	}

	// Check if this is a delivery receipt
	if deliverSM.EsmClass&0x04 != 0 { // Delivery receipt
		// Parse delivery receipt
		report, err := c.parseDeliveryReceipt(deliverSM)
		if err != nil {
			if c.logger != nil {
				c.logger.Error("Failed to parse delivery receipt", "error", err)
			}
			return err
		}

		if c.logger != nil {
			c.logger.Info("Delivery receipt received",
				"message_id", report.MessageID,
				"status", report.Status)
		}

		// Publish delivery event
		if c.eventPublisher != nil {
			event := &DeliveryEvent{
				Type:      EventTypeDeliveryReport,
				Timestamp: time.Now(),
				MessageID: report.MessageID,
				Report:    report,
				Session:   c.session,
				Data:      make(map[string]interface{}),
			}
			c.eventPublisher.PublishDeliveryEvent(ctx, event)
		}

		// Call delivery report callback
		if c.onDeliveryReport != nil {
			c.onDeliveryReport(report)
		}
	} else {
		// Regular SMS message
		message := &Message{
			SourceAddr:   Address{TON: deliverSM.SourceAddrTON, NPI: deliverSM.SourceAddrNPI, Addr: deliverSM.SourceAddr},
			DestAddr:     Address{TON: deliverSM.DestAddrTON, NPI: deliverSM.DestAddrNPI, Addr: deliverSM.DestAddr},
			ShortMessage: deliverSM.ShortMessage,
			DataCoding:   deliverSM.DataCoding,
			EsmClass:     deliverSM.EsmClass,
		}

		if c.logger != nil {
			c.logger.Info("SMS message received",
				"source", deliverSM.SourceAddr,
				"dest", deliverSM.DestAddr,
				"length", len(deliverSM.ShortMessage))
		}

		// Call callback
		if c.onMessage != nil {
			c.onMessage(message)
		}
	}

	return nil
}

// handleEnquireLink handles enquire_link PDU
func (c *Client) handleEnquireLink(ctx context.Context, enquireLink *EnquireLink) error {
	// Send enquire_link_resp
	respPDU := c.builder.BuildEnquireLinkResp(c.getNextSequence())
	return c.sendPDU(respPDU)
}

// parseDeliveryReceipt parses a delivery receipt from deliver_sm
func (c *Client) parseDeliveryReceipt(deliverSM *DeliverSM) (*DeliveryReport, error) {
	text := string(deliverSM.ShortMessage)

	report := &DeliveryReport{
		Text: text,
	}

	// Parse the delivery receipt format: id:MSG_ID sub:001 dlvrd:001 submit date:YYMMDDHHMM done date:YYMMDDHHMM stat:STATUS err:000 text:...
	// Extract fields from the text using string matching
	if idStart := strings.Index(text, "id:"); idStart >= 0 {
		idStart += 3
		if idEnd := strings.Index(text[idStart:], " "); idEnd > 0 {
			report.MessageID = text[idStart : idStart+idEnd]
		} else {
			report.MessageID = text[idStart:]
		}
	}

	if statStart := strings.Index(text, "stat:"); statStart >= 0 {
		statStart += 5
		if statEnd := strings.Index(text[statStart:], " "); statEnd > 0 {
			report.Status = text[statStart : statStart+statEnd]
		} else {
			report.Status = text[statStart:]
		}
	}

	if submitDateStart := strings.Index(text, "submit date:"); submitDateStart >= 0 {
		submitDateStart += 12
		if submitDateStart+10 <= len(text) {
			dateStr := text[submitDateStart : submitDateStart+10]
			if t, err := time.Parse("0601021504", dateStr); err == nil {
				report.SubmitTime = t
			}
		}
	}

	if doneDateStart := strings.Index(text, "done date:"); doneDateStart >= 0 {
		doneDateStart += 10
		if doneDateStart+10 <= len(text) {
			dateStr := text[doneDateStart : doneDateStart+10]
			if t, err := time.Parse("0601021504", dateStr); err == nil {
				report.DoneTime = t
				report.Timestamp = t
			}
		}
	}

	// If message ID not found in text, try optional parameters
	if report.MessageID == "" {
		for _, param := range deliverSM.OptionalParams {
			if param.Tag == TagReceiptedMessageID {
				report.MessageID = string(param.Value)
				break
			}
		}
	}

	// If still no message ID, use the source address
	if report.MessageID == "" {
		report.MessageID = deliverSM.SourceAddr
	}

	return report, nil
}

// periodicTasks runs periodic maintenance tasks
func (c *Client) periodicTasks(ctx context.Context) {
	defer c.wg.Done()

	enquireLinkTicker := time.NewTicker(c.config.EnquireLinkInterval)
	defer enquireLinkTicker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-enquireLinkTicker.C:
			c.sendEnquireLink(ctx)
		}
	}
}

// sendEnquireLink sends an enquire_link PDU
func (c *Client) sendEnquireLink(ctx context.Context) {
	c.mu.RLock()
	if !c.bound {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()

	enquirePDU := c.builder.BuildEnquireLink()

	if err := c.sendPDU(enquirePDU); err != nil {
		if c.logger != nil {
			c.logger.Error("Failed to send enquire_link", "error", err)
		}
	} else {
		if c.logger != nil {
			c.logger.Debug("Enquire link sent")
		}
	}
}

// handleDisconnection handles connection loss and triggers reconnection
func (c *Client) handleDisconnection(ctx context.Context, err error) {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return // Already handled
	}

	c.connected = false
	c.bound = false
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	if c.logger != nil {
		c.logger.Warn("Connection lost", "error", err)
	}

	// Call callback
	if c.onDisconnect != nil {
		c.onDisconnect(err)
	}

	// Start reconnection process if configured
	if c.config.MaxReconnectAttempts > 0 {
		go c.reconnect(ctx)
	}
}

// reconnect attempts to reconnect to the server
func (c *Client) reconnect(ctx context.Context) {
	for c.reconnectCount < c.config.MaxReconnectAttempts {
		c.reconnectCount++

		if c.logger != nil {
			c.logger.Info("Attempting to reconnect",
				"attempt", c.reconnectCount,
				"max_attempts", c.config.MaxReconnectAttempts)
		}

		// Wait before reconnecting
		time.Sleep(c.config.ReconnectInterval)

		// Try to reconnect
		if err := c.Connect(ctx); err != nil {
			if c.logger != nil {
				c.logger.Error("Reconnection failed", "attempt", c.reconnectCount, "error", err)
			}
			continue
		}

		// Try to bind
		bindDone := make(chan struct{}, 1)
		c.OnBind(func() {
			bindDone <- struct{}{}
		})

		if err := c.Bind(ctx); err != nil {
			if c.logger != nil {
				c.logger.Error("Rebind failed to send", "attempt", c.reconnectCount, "error", err)
			}
			c.Disconnect(ctx)
			continue
		}

		// Wait for bind response
		select {
		case <-bindDone:
			// Bind successful
		case <-time.After(30 * time.Second):
			if c.logger != nil {
				c.logger.Error("Rebind timeout", "attempt", c.reconnectCount)
			}
			c.Disconnect(ctx)
			continue
		}

		// Reset reconnect count on successful reconnection
		c.reconnectCount = 0

		if c.logger != nil {
			c.logger.Info("Reconnected successfully")
		}

		return
	}

	if c.logger != nil {
		c.logger.Error("Max reconnection attempts reached, giving up")
	}
}

// getNextSequence returns the next sequence number
func (c *Client) getNextSequence() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.session == nil {
		return 1
	}

	seq := c.session.SequenceNum
	c.session.SequenceNum++
	if c.session.SequenceNum == 0 {
		c.session.SequenceNum = 1
	}

	return seq
}

// Status methods
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *Client) IsBound() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bound
}

func (c *Client) GetSession() *Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

// Additional builder methods for client
func (b *PDUBuilder) BuildBindReceiver(systemID, password, systemType string) *PDU {
	body := &BindRequest{
		SystemID:         systemID,
		Password:         password,
		SystemType:       systemType,
		InterfaceVersion: SMPPVersion,
		AddrTON:          TONUnknown,
		AddrNPI:          NPIUnknown,
		AddressRange:     "",
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandBindReceiver,
			CommandStatus: StatusOK,
			SequenceNum:   b.getNextSequence(),
		},
		Body: body,
	}
}

func (b *PDUBuilder) BuildBindTransmitter(systemID, password, systemType string) *PDU {
	body := &BindRequest{
		SystemID:         systemID,
		Password:         password,
		SystemType:       systemType,
		InterfaceVersion: SMPPVersion,
		AddrTON:          TONUnknown,
		AddrNPI:          NPIUnknown,
		AddressRange:     "",
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandBindTransmitter,
			CommandStatus: StatusOK,
			SequenceNum:   b.getNextSequence(),
		},
		Body: body,
	}
}
