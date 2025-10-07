package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/oarkflow/smpp-server/internal/config"
	"github.com/oarkflow/smpp-server/internal/logger"
	"github.com/oarkflow/smpp-server/pkg/events"
	"github.com/oarkflow/smpp-server/pkg/smpp"
)

func main() {
	log.Println("Starting Full-Featured SMPP Client...")

	// Load configuration
	configPath := "configs/client.json"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	configManager := config.NewConfigManager(configPath)
	cfg, err := configManager.LoadConfig()
	if err != nil {
		log.Printf("Failed to load config, using defaults: %v", err)
		cfg = &smpp.Config{
			Client: smpp.ClientConfig{
				Host:                 "localhost",
				Port:                 2775,
				SystemID:             "test",
				Password:             "test",
				SystemType:           "SMPP",
				BindType:             "transceiver",
				ConnectTimeout:       10 * time.Second,
				ReadTimeout:          30 * time.Second,
				WriteTimeout:         10 * time.Second,
				EnquireLinkInterval:  30 * time.Second,
				ReconnectInterval:    5 * time.Second,
				MaxReconnectAttempts: 5,
				LogLevel:             "info",
			},
			Logging: smpp.LoggingConfig{
				Level:  "info",
				Format: "text",
				Output: "stdout",
			},
		}
	}

	clientCfg := configManager.GetClientConfig()

	// Create logger
	appLogger := logger.NewDefaultLogger(cfg.Logging.Level)
	appLogger.Info("Initializing SMPP client components")

	// Create event bus for client events
	eventBus := events.NewAsyncEventBus(appLogger, 1000)

	// Create client dependencies
	deps := smpp.ClientDependencies{
		EventPublisher: eventBus,
		Logger:         appLogger,
	}

	// Create client
	client := smpp.NewClient(clientCfg, deps)

	// Set up client callbacks
	setupClientCallbacks(client, appLogger)

	// Set up event handlers
	setupClientEventHandlers(eventBus, appLogger)

	// Connect to server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	appLogger.Info("Connecting to SMPP server",
		"host", clientCfg.Host,
		"port", clientCfg.Port,
		"systemID", clientCfg.SystemID)

	if err := client.Connect(ctx); err != nil {
		appLogger.Fatal("Failed to connect to server", "error", err)
	}

	appLogger.Info("Connected to SMPP server successfully")

	// Bind to the server
	appLogger.Info("Binding to SMPP server", "bind_type", clientCfg.BindType)

	if err := client.Bind(ctx); err != nil {
		appLogger.Fatal("Failed to send bind request", "error", err)
	}

	appLogger.Info("Bind request sent, waiting for response...")

	// Wait for bind to complete (the callback will be called when bind succeeds)
	time.Sleep(2 * time.Second) // Give it a moment to bind

	// Start message sending demo
	go sendDemoMessages(client, appLogger)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	appLogger.Info("Received shutdown signal, disconnecting client...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := client.Disconnect(shutdownCtx); err != nil {
		appLogger.Error("Error during client disconnect", "error", err)
	} else {
		appLogger.Info("Client disconnected gracefully")
	}
}

// sendDemoMessages sends test messages periodically
func sendDemoMessages(client *smpp.Client, logger smpp.Logger) {
	// Wait a moment for binding to complete
	time.Sleep(3 * time.Second)

	// Send short message example
	sendShortMessageExample(client, logger)

	// Wait a bit
	time.Sleep(5 * time.Second)

	// Send long message example
	sendLongMessageExample(client, logger)

	logger.Info("Demo complete - sent both short and long message examples")
}

// sendShortMessageExample demonstrates sending a short message
func sendShortMessageExample(client *smpp.Client, logger smpp.Logger) {
	logger.Info("=== SHORT MESSAGE EXAMPLE ===")

	sourceAddr := "12345"
	destAddr := "67890"
	messageText := "Hello! This is a short SMS message."

	logger.Info("Sending short message",
		"source", sourceAddr,
		"dest", destAddr,
		"text", messageText,
		"length", len(messageText))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err := client.SendSMS(ctx, sourceAddr, destAddr, messageText)
	cancel()

	if err != nil {
		logger.Error("Failed to send short message", "error", err)
	} else {
		logger.Info("Short message send request submitted successfully")
	}
}

// sendLongMessageExample demonstrates sending a long message that gets split into parts
func sendLongMessageExample(client *smpp.Client, logger smpp.Logger) {
	logger.Info("=== LONG MESSAGE EXAMPLE ===")

	sourceAddr := "12345"
	destAddr := "67890"
	// Create a long message that will be split into multiple parts
	messageText := "This is a very long SMS message that exceeds the 160 character limit for standard SMS. " +
		"It will be automatically split into multiple parts by the SMPP server. " +
		"Each part contains a User Data Header (UDH) that allows the receiving device " +
		"to reassemble the message correctly. This demonstrates how SMPP handles " +
		"concatenated SMS messages. The total length of this message is over 400 characters, " +
		"so it will likely be split into 3 or more parts depending on the encoding used."

	logger.Info("Sending long message",
		"source", sourceAddr,
		"dest", destAddr,
		"text_length", len(messageText),
		"estimated_parts", (len(messageText)+153)/154) // Rough estimate

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err := client.SendSMS(ctx, sourceAddr, destAddr, messageText)
	cancel()

	if err != nil {
		logger.Error("Failed to send long message", "error", err)
	} else {
		logger.Info("Long message send request submitted successfully")
	}
}

// setupClientCallbacks sets up client-level event callbacks
func setupClientCallbacks(client *smpp.Client, logger smpp.Logger) {
	client.OnConnect(func() {
		logger.Info("Client connected")
	})

	client.OnDisconnect(func(err error) {
		if err != nil {
			logger.Error("Client disconnected with error", "error", err)
		} else {
			logger.Info("Client disconnected")
		}
	})

	client.OnBind(func() {
		logger.Info("Client bound successfully")
	})

	client.OnUnbind(func() {
		logger.Info("Client unbound")
	})

	client.OnSubmitResponse(func(messageID string, err error) {
		if err != nil {
			logger.Error("Submit failed", "messageID", messageID, "error", err)
		} else {
			logger.Info("Message submitted successfully", "messageID", messageID)

			// Check if this is a multi-part message
			if strings.Contains(messageID, "_") {
				// Extract part information from message ID
				parts := strings.Split(messageID, "_")
				if len(parts) >= 2 {
					baseID := strings.Join(parts[:len(parts)-1], "_")
					partNum := parts[len(parts)-1]
					logger.Info("SMS Part submitted",
						"base_message_id", baseID,
						"part_number", partNum,
						"part_message_id", messageID)
				}
			}
		}
	})

	client.OnMessage(func(message *smpp.Message) {
		logger.Info("Received SMS message",
			"source", message.SourceAddr.Addr,
			"dest", message.DestAddr.Addr,
			"text", message.Text)
	})

	client.OnDeliveryReport(func(report *smpp.DeliveryReport) {
		logger.Info("Received delivery report",
			"messageID", report.MessageID,
			"status", report.Status,
			"timestamp", report.Timestamp)

		// Check if this is a delivery report for a message part
		if strings.Contains(report.MessageID, "_") {
			parts := strings.Split(report.MessageID, "_")
			if len(parts) >= 2 {
				baseID := strings.Join(parts[:len(parts)-1], "_")
				partNum := parts[len(parts)-1]
				logger.Info("SMS Part delivery report",
					"base_message_id", baseID,
					"part_number", partNum,
					"part_status", report.Status,
					"part_message_id", report.MessageID)
			}
		}

		// Log final status information
		logDeliveryStatus(report, logger)
	})
}
func setupClientEventHandlers(eventBus smpp.EventPublisher, logger smpp.Logger) {
	ctx := context.Background()

	// Connection event handler
	connectionHandler := &ClientEventHandler{
		EventType: "connection",
		Logger:    logger,
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeConnected, connectionHandler); err != nil {
		logger.Error("Failed to subscribe to connection events", "error", err)
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeDisconnected, connectionHandler); err != nil {
		logger.Error("Failed to subscribe to disconnection events", "error", err)
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeBound, connectionHandler); err != nil {
		logger.Error("Failed to subscribe to bound events", "error", err)
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeUnbound, connectionHandler); err != nil {
		logger.Error("Failed to subscribe to unbound events", "error", err)
	}

	// Message event handler
	messageHandler := &ClientEventHandler{
		EventType: "message",
		Logger:    logger,
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeSMSSubmitted, messageHandler); err != nil {
		logger.Error("Failed to subscribe to SMS submitted events", "error", err)
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeSMSDelivered, messageHandler); err != nil {
		logger.Error("Failed to subscribe to SMS delivered events", "error", err)
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeSMSFailed, messageHandler); err != nil {
		logger.Error("Failed to subscribe to SMS failed events", "error", err)
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeDeliveryReport, messageHandler); err != nil {
		logger.Error("Failed to subscribe to delivery report events", "error", err)
	}

	logger.Info("Client event handlers configured successfully")
}

// ClientEventHandler handles client events
type ClientEventHandler struct {
	EventType string
	Logger    smpp.Logger
}

func (h *ClientEventHandler) HandleEvent(ctx context.Context, event smpp.Event) error {
	h.Logger.Info("Client event received",
		"type", string(event.GetEventType()),
		"timestamp", event.GetTimestamp(),
		"handler", h.EventType,
		"data", event.GetData())

	// Special handling for delivery reports
	if event.GetEventType() == smpp.EventTypeDeliveryReport {
		if data := event.GetData(); data != nil {
			if messageID, ok := data["messageID"].(string); ok {
				if status, ok := data["status"].(string); ok {
					h.Logger.Info("Delivery report details",
						"messageID", messageID,
						"status", status)

					// Check if this is a part of a multi-part message
					if strings.Contains(messageID, "_") {
						parts := strings.Split(messageID, "_")
						if len(parts) >= 2 {
							baseID := strings.Join(parts[:len(parts)-1], "_")
							partNum := parts[len(parts)-1]
							h.Logger.Info("📦 Multi-part SMS delivery report",
								"base_message_id", baseID,
								"part_number", partNum,
								"part_status", status)
						}
					}
				}
			}
		}
	}

	return nil
}

func (h *ClientEventHandler) GetHandlerID() string {
	return "client_" + h.EventType
}

// logDeliveryStatus logs detailed delivery status information
func logDeliveryStatus(report *smpp.DeliveryReport, logger smpp.Logger) {
	status := strings.ToUpper(report.Status)

	switch status {
	case "DELIVRD":
		logger.Info("✅ SMS delivered successfully",
			"message_id", report.MessageID,
			"delivered_at", report.Timestamp)
	case "ACCEPTD":
		logger.Info("📨 SMS accepted by network",
			"message_id", report.MessageID,
			"accepted_at", report.Timestamp)
	case "ENROUTE":
		logger.Info("🚀 SMS in transit",
			"message_id", report.MessageID,
			"enroute_at", report.Timestamp)
	case "EXPIRED":
		logger.Warn("⏰ SMS expired",
			"message_id", report.MessageID,
			"expired_at", report.Timestamp)
	case "DELETED":
		logger.Warn("🗑️ SMS deleted",
			"message_id", report.MessageID,
			"deleted_at", report.Timestamp)
	case "UNDELIV":
		logger.Error("❌ SMS undeliverable",
			"message_id", report.MessageID,
			"failed_at", report.Timestamp)
	case "REJECTD":
		logger.Error("🚫 SMS rejected",
			"message_id", report.MessageID,
			"rejected_at", report.Timestamp)
	default:
		logger.Info("📊 SMS status update",
			"message_id", report.MessageID,
			"status", status,
			"timestamp", report.Timestamp)
	}
}
