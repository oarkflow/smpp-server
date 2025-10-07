package smpp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"
)

// Server represents an SMPP server
type Server struct {
	config         *ServerConfig
	listener       net.Listener
	connections    ConnectionManager
	userAuth       UserAuth
	smsStorage     SMSStorage
	reportStorage  ReportStorage
	messageHandler MessageHandler
	eventPublisher EventPublisher
	logger         Logger
	metrics        MetricsCollector
	encoder        *PDUEncoder
	decoder        *PDUDecoder
	builder        *PDUBuilder

	// Server state
	mu      sync.RWMutex
	running bool
	done    chan struct{}
	wg      sync.WaitGroup
}

// NewServer creates a new SMPP server
func NewServer(config *ServerConfig, deps ServerDependencies) *Server {
	return &Server{
		config:         config,
		connections:    deps.ConnectionManager,
		userAuth:       deps.UserAuth,
		smsStorage:     deps.SMSStorage,
		reportStorage:  deps.ReportStorage,
		messageHandler: deps.MessageHandler,
		eventPublisher: deps.EventPublisher,
		logger:         deps.Logger,
		metrics:        deps.MetricsCollector,
		encoder:        NewPDUEncoder(),
		decoder:        NewPDUDecoder(),
		builder:        NewPDUBuilder(),
		done:           make(chan struct{}),
	}
}

// ServerDependencies holds all dependencies for the server
type ServerDependencies struct {
	ConnectionManager ConnectionManager
	UserAuth          UserAuth
	SMSStorage        SMSStorage
	ReportStorage     ReportStorage
	MessageHandler    MessageHandler
	EventPublisher    EventPublisher
	Logger            Logger
	MetricsCollector  MetricsCollector
}

// Start starts the SMPP server
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server is already running")
	}
	s.running = true
	s.mu.Unlock()

	// Create listener
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	var listener net.Listener
	var err error

	if s.config.TLSEnabled {
		// Load TLS certificate
		cert, err := tls.LoadX509KeyPair(s.config.TLSCertFile, s.config.TLSKeyFile)
		if err != nil {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return fmt.Errorf("failed to load TLS certificate: %w", err)
		}

		// Create TLS config
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			ServerName:   s.config.Host,
		}

		// Create TLS listener
		listener, err = tls.Listen("tcp", addr, tlsConfig)
		if err != nil {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return fmt.Errorf("failed to listen on %s with TLS: %w", addr, err)
		}

		if s.logger != nil {
			s.logger.Info("SMPP server started with TLS", "address", addr, "cert_file", s.config.TLSCertFile)
		}
	} else {
		listener, err = net.Listen("tcp", addr)
		if err != nil {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return fmt.Errorf("failed to listen on %s: %w", addr, err)
		}

		if s.logger != nil {
			s.logger.Info("SMPP server started", "address", addr)
		}
	}

	s.listener = listener

	// Start accepting connections
	s.wg.Add(1)
	go s.acceptConnections(ctx)

	// Start periodic tasks
	s.wg.Add(1)
	go s.periodicTasks(ctx)

	return nil
}

// Stop stops the SMPP server
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return fmt.Errorf("server is not running")
	}
	s.running = false
	s.mu.Unlock()

	// Close listener
	if s.listener != nil {
		s.listener.Close()
	}

	// Signal shutdown
	close(s.done)

	// Wait for goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Graceful shutdown
	case <-time.After(5 * time.Second):
		// Force shutdown after timeout
		if s.logger != nil {
			s.logger.Warn("Server shutdown timed out, forcing exit")
		}
	}

	if s.logger != nil {
		s.logger.Info("SMPP server stopped")
	}

	return nil
}

// acceptConnections accepts new connections
func (s *Server) acceptConnections(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-s.done:
			return
		default:
			// Set accept deadline to allow periodic checking of done channel
			if tcpListener, ok := s.listener.(*net.TCPListener); ok {
				tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
			}

			conn, err := s.listener.Accept()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Timeout, check done channel
				}

				select {
				case <-s.done:
					return
				default:
					if s.logger != nil {
						s.logger.Error("Failed to accept connection", "error", err)
					}
					continue
				}
			}

			// Check connection limit
			if s.connections.GetConnectionCount(ctx) >= s.config.MaxConnections {
				if s.logger != nil {
					s.logger.Warn("Max connections reached, rejecting new connection",
						"remote_addr", conn.RemoteAddr().String())
				}
				conn.Close()
				continue
			}

			// Handle connection in a new goroutine
			s.wg.Add(1)
			go s.handleConnection(ctx, conn)
		}
	}
}

// handleConnection handles a new connection
func (s *Server) handleConnection(ctx context.Context, netConn net.Conn) {
	defer s.wg.Done()
	defer netConn.Close()

	remoteAddr := netConn.RemoteAddr().String()

	if s.logger != nil {
		s.logger.Debug("New connection accepted", "remote_addr", remoteAddr)
	}

	// Create connection wrapper
	conn := NewServerConnection(netConn, s.config, s.logger)

	// Set timeouts
	netConn.SetReadDeadline(time.Now().Add(s.config.ReadTimeout))
	netConn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))

	// Handle connection lifecycle
	if err := s.connectionLifecycle(ctx, conn); err != nil {
		if s.logger != nil {
			s.logger.Error("Connection error", "remote_addr", remoteAddr, "error", err)
		}
	}

	// Clean up connection
	if conn.GetSessionID() != "" {
		s.connections.RemoveConnection(ctx, conn.GetSessionID())
	}

	if s.logger != nil {
		s.logger.Debug("Connection closed", "remote_addr", remoteAddr)
	}

	// Publish disconnection event
	if s.eventPublisher != nil && conn.GetSession() != nil {
		event := &ConnectionEvent{
			Type:       EventTypeDisconnected,
			Timestamp:  time.Now(),
			SessionID:  conn.GetSessionID(),
			Session:    conn.GetSession(),
			RemoteAddr: remoteAddr,
			Data:       make(map[string]interface{}),
		}
		s.eventPublisher.PublishConnectionEvent(ctx, event)
	}
}

// connectionLifecycle manages the lifecycle of a connection
func (s *Server) connectionLifecycle(ctx context.Context, conn Connection) error {
	defer func() {
		if r := recover(); r != nil {
			if s.logger != nil {
				s.logger.Error("Panic in connectionLifecycle", "panic", r, "session_id", conn.GetSessionID())
			}
		}
	}()

	session := &Session{
		ID:           generateSessionID(),
		State:        SessionStateOpen,
		LastActivity: time.Now(),
		SequenceNum:  1,
	}

	// Update connection with session
	if sc, ok := conn.(*ServerConnection); ok {
		sc.SetSession(session)
	}

	// Publish connection event
	if s.eventPublisher != nil {
		event := &ConnectionEvent{
			Type:       EventTypeConnected,
			Timestamp:  time.Now(),
			SessionID:  session.ID,
			Session:    session,
			RemoteAddr: conn.GetRemoteAddr(),
			Data: map[string]interface{}{
				"connection": conn, // Include connection object for manager
			},
		}
		s.eventPublisher.PublishConnectionEvent(ctx, event)
	}

	// Handle PDUs
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return fmt.Errorf("server shutting down")
		default:
			// Read PDU
			netConn := conn.(*ServerConnection).netConn

			if s.logger != nil {
				s.logger.Info("Waiting for PDU from client", "session_id", session.ID)
			}

			pdu, err := s.decoder.DecodeFromReader(netConn)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// Check if connection is still active
					if !conn.IsActive() {
						return fmt.Errorf("connection inactive")
					}
					continue
				}
				if s.logger != nil {
					s.logger.Error("Failed to decode PDU", "error", err)
				}
				return fmt.Errorf("failed to decode PDU: %w", err)
			}

			if s.logger != nil {
				s.logger.Info("PDU decoded successfully",
					"command_id", fmt.Sprintf("0x%08X", pdu.Header.CommandID),
					"sequence", pdu.Header.SequenceNum,
					"session_id", session.ID)
			}

			conn.UpdateLastActivity()
			session.LastActivity = time.Now()

			// Handle the PDU
			response, err := s.handlePDU(ctx, session, pdu)
			if err != nil {
				if s.logger != nil {
					s.logger.Error("Error handling PDU",
						"session_id", session.ID,
						"command_id", fmt.Sprintf("0x%08X", pdu.Header.CommandID),
						"error", err)
				}

				// Send error response
				errorResp := s.builder.BuildGenericNack(pdu.Header.SequenceNum, StatusSysErr)
				if sendErr := conn.Send(ctx, errorResp); sendErr != nil {
					if s.logger != nil {
						s.logger.Error("Failed to send error response", "error", sendErr, "session_id", session.ID)
					}
					return fmt.Errorf("failed to send error response: %w", sendErr)
				}
				continue
			}

			// Send response if provided
			if response != nil {
				if s.logger != nil {
					s.logger.Info("About to send response", "command_id", fmt.Sprintf("0x%08X", response.Header.CommandID), "session_id", session.ID)
				}
				if err := conn.Send(ctx, response); err != nil {
					if s.logger != nil {
						s.logger.Error("Failed to send response", "error", err, "session_id", session.ID)
					}
					return fmt.Errorf("failed to send response: %w", err)
				}
				if s.logger != nil {
					s.logger.Info("Response sent successfully", "session_id", session.ID)
				}
			}

			// Check if session should be closed
			if session.State == SessionStateUnbound || session.State == SessionStateClosed {
				if s.logger != nil {
					s.logger.Info("Session should be closed", "state", session.State, "session_id", session.ID)
				}
				return nil
			}

			if s.logger != nil {
				s.logger.Info("Continuing loop for next PDU", "session_id", session.ID)
			}
		}
	}
}

// handlePDU handles a received PDU
func (s *Server) handlePDU(ctx context.Context, session *Session, pdu *PDU) (*PDU, error) {
	originalSeqNum := pdu.Header.SequenceNum
	session.SequenceNum = pdu.Header.SequenceNum + 1

	if s.logger != nil {
		s.logger.Info("Server received PDU",
			"command_id", fmt.Sprintf("0x%08X", pdu.Header.CommandID),
			"sequence", pdu.Header.SequenceNum,
			"session_id", session.ID)
	}

	// Handle PDU with panic recovery
	defer func() {
		if r := recover(); r != nil {
			if s.logger != nil {
				s.logger.Error("Panic in handlePDU",
					"command_id", fmt.Sprintf("0x%08X", pdu.Header.CommandID),
					"sequence", pdu.Header.SequenceNum,
					"session_id", session.ID,
					"panic", r)
			}
		}
	}()

	switch pdu.Header.CommandID {
	case CommandBindReceiver:
		return s.handleBindReceiver(ctx, session, pdu.Body.(*BindRequest), originalSeqNum)
	case CommandBindTransmitter:
		return s.handleBindTransmitter(ctx, session, pdu.Body.(*BindRequest), originalSeqNum)
	case CommandBindTransceiver:
		return s.handleBindTransceiver(ctx, session, pdu.Body.(*BindRequest), originalSeqNum)
	case CommandSubmitSM:
		if s.logger != nil {
			s.logger.Info("Routing to handleSubmitSM", "sequence", originalSeqNum)
		}
		return s.handleSubmitSM(ctx, session, pdu.Body.(*SubmitSM), originalSeqNum)
	case CommandQuerySM:
		return s.handleQuerySM(ctx, session, pdu.Body.(*QuerySM), originalSeqNum)
	case CommandReplaceSM:
		return s.handleReplaceSM(ctx, session, pdu.Body.(*ReplaceSM), originalSeqNum)
	case CommandCancelSM:
		return s.handleCancelSM(ctx, session, pdu.Body.(*CancelSM), originalSeqNum)
	case CommandSubmitMulti:
		return s.handleSubmitMulti(ctx, session, pdu.Body.(*SubmitMulti), originalSeqNum)
	case CommandDataSM:
		return s.handleDataSM(ctx, session, pdu.Body.(*DataSM), originalSeqNum)
	case CommandDeliverSMResp:
		return s.handleDeliverSMResp(ctx, session, pdu.Body.(*DeliverSMResp))
	case CommandEnquireLink:
		return s.handleEnquireLink(ctx, session, pdu.Body.(*EnquireLink), originalSeqNum)
	case CommandUnbind:
		return s.handleUnbind(ctx, session, pdu.Body.(*Unbind), originalSeqNum)
	default:
		return nil, fmt.Errorf("unsupported command ID: 0x%08X", pdu.Header.CommandID)
	}
}

// handleBindTransceiver handles bind_transceiver PDU
func (s *Server) handleBindTransceiver(ctx context.Context, session *Session, req *BindRequest, sequenceNum uint32) (*PDU, error) {
	// Authenticate user
	user, err := s.userAuth.Authenticate(ctx, req.SystemID, req.Password)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Authentication failed",
				"system_id", req.SystemID,
				"session_id", session.ID,
				"error", err)
		}

		if s.metrics != nil {
			s.metrics.IncCounter("auth_failures_total", map[string]string{
				"system_id": req.SystemID,
			})
		}

		return s.builder.BuildBindTransceiverResp("", sequenceNum, StatusInvPaswd), nil
	}

	// Update session
	session.SystemID = req.SystemID
	session.Password = req.Password
	session.SystemType = req.SystemType
	session.BindType = CommandBindTransceiver
	session.State = SessionStateBoundTRX

	if s.logger != nil {
		s.logger.Info("Bind transceiver successful",
			"system_id", req.SystemID,
			"session_id", session.ID)
	}

	if s.metrics != nil {
		s.metrics.IncCounter("bind_success_total", map[string]string{
			"system_id": req.SystemID,
			"bind_type": "transceiver",
		})
	}

	// Publish bind event
	if s.eventPublisher != nil {
		event := &ConnectionEvent{
			Type:      EventTypeBound,
			Timestamp: time.Now(),
			SessionID: session.ID,
			Session:   session,
			Data: map[string]interface{}{
				"bind_type": "transceiver",
				"user":      user,
			},
		}
		s.eventPublisher.PublishConnectionEvent(ctx, event)
	}

	return s.builder.BuildBindTransceiverResp(req.SystemID, sequenceNum, StatusOK), nil
}

// handleBindReceiver handles bind_receiver PDU
func (s *Server) handleBindReceiver(ctx context.Context, session *Session, req *BindRequest, sequenceNum uint32) (*PDU, error) {
	// Similar to handleBindTransceiver but with receiver state
	_, err := s.userAuth.Authenticate(ctx, req.SystemID, req.Password)
	if err != nil {
		return s.builder.BuildBindReceiverResp("", sequenceNum, StatusInvPaswd), nil
	}

	session.SystemID = req.SystemID
	session.Password = req.Password
	session.SystemType = req.SystemType
	session.BindType = CommandBindReceiver
	session.State = SessionStateBoundRX

	// Similar logging and events...

	return s.builder.BuildBindReceiverResp(req.SystemID, sequenceNum, StatusOK), nil
}

// handleBindTransmitter handles bind_transmitter PDU
func (s *Server) handleBindTransmitter(ctx context.Context, session *Session, req *BindRequest, sequenceNum uint32) (*PDU, error) {
	// Similar to handleBindTransceiver but with transmitter state
	_, err := s.userAuth.Authenticate(ctx, req.SystemID, req.Password)
	if err != nil {
		return s.builder.BuildBindTransmitterResp("", sequenceNum, StatusInvPaswd), nil
	}

	session.SystemID = req.SystemID
	session.Password = req.Password
	session.SystemType = req.SystemType
	session.BindType = CommandBindTransmitter
	session.State = SessionStateBoundTX

	// Similar logging and events...

	return s.builder.BuildBindTransmitterResp(req.SystemID, sequenceNum, StatusOK), nil
}

// handleSubmitSM handles submit_sm PDU
func (s *Server) handleSubmitSM(ctx context.Context, session *Session, req *SubmitSM, sequenceNum uint32) (*PDU, error) {
	if s.logger != nil {
		s.logger.Info("Received submit_sm",
			"session_id", session.ID,
			"sequence", sequenceNum,
			"source_addr", req.SourceAddr,
			"dest_addr", req.DestAddr,
			"message", string(req.ShortMessage))
	}

	// Check if session is bound for transmission
	if session.State != SessionStateBoundTX && session.State != SessionStateBoundTRX {
		return s.builder.BuildSubmitSMResp("", sequenceNum, StatusInvBnd), nil
	}

	// Use message handler to process the submit_sm
	response, err := s.messageHandler.HandleSubmitSM(ctx, session, req)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("Error handling submit_sm",
				"session_id", session.ID,
				"error", err)
		}
		return s.builder.BuildSubmitSMResp("", sequenceNum, StatusSysErr), nil
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandSubmitSMResp,
			CommandStatus: StatusOK,
			SequenceNum:   sequenceNum,
		},
		Body: response,
	}, nil
}

// handleQuerySM handles query_sm PDU
func (s *Server) handleQuerySM(ctx context.Context, session *Session, req *QuerySM, sequenceNum uint32) (*PDU, error) {
	if s.logger != nil {
		s.logger.Info("Received query_sm",
			"session_id", session.ID,
			"sequence", sequenceNum,
			"message_id", req.MessageID.GetString())
	}

	// Check if session is bound
	if session.State != SessionStateBoundRX && session.State != SessionStateBoundTRX {
		return s.builder.BuildQuerySMResp(req.MessageID, "", 0, 0, sequenceNum, StatusInvBnd), nil
	}

	// Use message handler to process the query_sm
	response, err := s.messageHandler.HandleQuerySM(ctx, session, req)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("Error handling query_sm",
				"session_id", session.ID,
				"message_id", req.MessageID.GetString(),
				"error", err)
		}
		return s.builder.BuildQuerySMResp(req.MessageID, "", 0, 0, sequenceNum, StatusSysErr), nil
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandQuerySMResp,
			CommandStatus: StatusOK,
			SequenceNum:   sequenceNum,
		},
		Body: response,
	}, nil
}

// handleReplaceSM handles replace_sm PDU
func (s *Server) handleReplaceSM(ctx context.Context, session *Session, req *ReplaceSM, sequenceNum uint32) (*PDU, error) {
	if s.logger != nil {
		s.logger.Info("Received replace_sm",
			"session_id", session.ID,
			"sequence", sequenceNum,
			"message_id", req.MessageID.GetString())
	}

	// Check if session is bound for transmission
	if session.State != SessionStateBoundTX && session.State != SessionStateBoundTRX {
		return s.builder.BuildReplaceSMResp(sequenceNum, StatusInvBnd), nil
	}

	// Use message handler to process the replace_sm
	response, err := s.messageHandler.HandleReplaceSM(ctx, session, req)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("Error handling replace_sm",
				"session_id", session.ID,
				"message_id", req.MessageID.GetString(),
				"error", err)
		}
		return s.builder.BuildReplaceSMResp(sequenceNum, StatusSysErr), nil
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandReplaceSMResp,
			CommandStatus: StatusOK,
			SequenceNum:   sequenceNum,
		},
		Body: response,
	}, nil
}

// handleCancelSM handles cancel_sm PDU
func (s *Server) handleCancelSM(ctx context.Context, session *Session, req *CancelSM, sequenceNum uint32) (*PDU, error) {
	if s.logger != nil {
		s.logger.Info("Received cancel_sm",
			"session_id", session.ID,
			"sequence", sequenceNum,
			"message_id", req.MessageID.GetString())
	}

	// Check if session is bound for transmission
	if session.State != SessionStateBoundTX && session.State != SessionStateBoundTRX {
		return s.builder.BuildCancelSMResp(sequenceNum, StatusInvBnd), nil
	}

	// Use message handler to process the cancel_sm
	response, err := s.messageHandler.HandleCancelSM(ctx, session, req)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("Error handling cancel_sm",
				"session_id", session.ID,
				"message_id", req.MessageID.GetString(),
				"error", err)
		}
		return s.builder.BuildCancelSMResp(sequenceNum, StatusSysErr), nil
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandCancelSMResp,
			CommandStatus: StatusOK,
			SequenceNum:   sequenceNum,
		},
		Body: response,
	}, nil
}

// handleSubmitMulti handles submit_multi PDU
func (s *Server) handleSubmitMulti(ctx context.Context, session *Session, req *SubmitMulti, sequenceNum uint32) (*PDU, error) {
	if s.logger != nil {
		s.logger.Info("Received submit_multi",
			"session_id", session.ID,
			"sequence", sequenceNum,
			"dest_count", req.NumberOfDests)
	}

	// Check if session is bound for transmission
	if session.State != SessionStateBoundTX && session.State != SessionStateBoundTRX {
		return s.builder.BuildSubmitMultiResp("", nil, sequenceNum, StatusInvBnd), nil
	}

	// Use message handler to process the submit_multi
	response, err := s.messageHandler.HandleSubmitMulti(ctx, session, req)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("Error handling submit_multi",
				"session_id", session.ID,
				"error", err)
		}
		return s.builder.BuildSubmitMultiResp("", nil, sequenceNum, StatusSysErr), nil
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandSubmitMultiResp,
			CommandStatus: StatusOK,
			SequenceNum:   sequenceNum,
		},
		Body: response,
	}, nil
}

// handleDataSM handles data_sm PDU
func (s *Server) handleDataSM(ctx context.Context, session *Session, req *DataSM, sequenceNum uint32) (*PDU, error) {
	if s.logger != nil {
		s.logger.Info("Received data_sm",
			"session_id", session.ID,
			"sequence", sequenceNum,
			"source", req.SourceAddr.Addr,
			"dest", req.DestAddr.Addr)
	}

	// Check if session is bound for transmission
	if session.State != SessionStateBoundTX && session.State != SessionStateBoundTRX {
		return s.builder.BuildDataSMResp("", sequenceNum, StatusInvBnd), nil
	}

	// Use message handler to process the data_sm
	response, err := s.messageHandler.HandleDataSM(ctx, session, req)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("Error handling data_sm",
				"session_id", session.ID,
				"error", err)
		}
		return s.builder.BuildDataSMResp("", sequenceNum, StatusSysErr), nil
	}

	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandDataSMResp,
			CommandStatus: StatusOK,
			SequenceNum:   sequenceNum,
		},
		Body: response,
	}, nil
}

// handleDeliverSMResp handles deliver_sm_resp PDU
func (s *Server) handleDeliverSMResp(ctx context.Context, session *Session, req *DeliverSMResp) (*PDU, error) {
	if s.logger != nil {
		s.logger.Debug("Received deliver_sm_resp",
			"session_id", session.ID,
			"message_id", req.MessageID)
	}

	// No response needed for deliver_sm_resp
	return nil, nil
}

// handleEnquireLink handles enquire_link PDU
func (s *Server) handleEnquireLink(ctx context.Context, session *Session, req *EnquireLink, sequenceNum uint32) (*PDU, error) {
	if s.logger != nil {
		s.logger.Debug("Received enquire_link", "session_id", session.ID)
	}

	return s.builder.BuildEnquireLinkResp(sequenceNum), nil
}

// handleUnbind handles unbind PDU
func (s *Server) handleUnbind(ctx context.Context, session *Session, req *Unbind, sequenceNum uint32) (*PDU, error) {
	if s.logger != nil {
		s.logger.Info("Received unbind", "session_id", session.ID)
	}

	session.State = SessionStateUnbound

	// Publish unbind event
	if s.eventPublisher != nil {
		event := &ConnectionEvent{
			Type:      EventTypeUnbound,
			Timestamp: time.Now(),
			SessionID: session.ID,
			Session:   session,
			Data:      make(map[string]interface{}),
		}
		s.eventPublisher.PublishConnectionEvent(ctx, event)
	}

	return s.builder.BuildUnbindResp(sequenceNum), nil
}

// periodicTasks runs periodic maintenance tasks
func (s *Server) periodicTasks(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.cleanupInactiveConnections(ctx)
		}
	}
}

// cleanupInactiveConnections removes inactive connections
func (s *Server) cleanupInactiveConnections(ctx context.Context) {
	connections, err := s.connections.GetAllConnections(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to get connections for cleanup", "error", err)
		}
		return
	}

	now := time.Now()
	for _, conn := range connections {
		if now.Sub(conn.GetLastActivity()) > s.config.IdleTimeout {
			if s.logger != nil {
				s.logger.Info("Removing inactive connection",
					"session_id", conn.GetSessionID(),
					"last_activity", conn.GetLastActivity())
			}

			conn.Close()
			s.connections.RemoveConnection(ctx, conn.GetSessionID())
		}
	}
}

// IsRunning returns true if the server is running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetConfig returns the server configuration
func (s *Server) GetConfig() *ServerConfig {
	return s.config
}

// GetStats returns server statistics
func (s *Server) GetStats(ctx context.Context) *ServerStats {
	connectionCount := s.connections.GetConnectionCount(ctx)

	// Get SMS and report counts if storage is available
	var smsCount, reportCount int64
	if s.smsStorage != nil {
		smsCount, _ = s.smsStorage.GetSMSCount(ctx)
	}
	if s.reportStorage != nil {
		reportCount, _ = s.reportStorage.GetReportCount(ctx)
	}

	return &ServerStats{
		ConnectionCount: connectionCount,
		SMSCount:        smsCount,
		ReportCount:     reportCount,
		Uptime:          time.Since(time.Now()), // This should track actual uptime
	}
}

// ServerStats represents server statistics
type ServerStats struct {
	ConnectionCount int
	SMSCount        int64
	ReportCount     int64
	Uptime          time.Duration
}

// Helper functions

// generateSessionID generates a unique session ID
func generateSessionID() string {
	return fmt.Sprintf("sess_%d_%d", time.Now().UnixNano(), time.Now().Unix()%1000)
}

// Additional PDU builder methods for bind responses
func (b *PDUBuilder) BuildBindReceiverResp(systemID string, sequenceNum uint32, status uint32) *PDU {
	body := &BindResponse{SystemID: systemID}
	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandBindReceiverResp,
			CommandStatus: status,
			SequenceNum:   sequenceNum,
		},
		Body: body,
	}
}

func (b *PDUBuilder) BuildBindTransmitterResp(systemID string, sequenceNum uint32, status uint32) *PDU {
	body := &BindResponse{SystemID: systemID}
	return &PDU{
		Header: PDUHeader{
			CommandID:     CommandBindTransmitterResp,
			CommandStatus: status,
			SequenceNum:   sequenceNum,
		},
		Body: body,
	}
}
