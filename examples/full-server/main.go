package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/oarkflow/smpp-server/internal/auth"
	"github.com/oarkflow/smpp-server/internal/config"
	"github.com/oarkflow/smpp-server/internal/logger"
	"github.com/oarkflow/smpp-server/internal/storage"
	"github.com/oarkflow/smpp-server/pkg/events"
	"github.com/oarkflow/smpp-server/pkg/smpp"
)

func main() {
	log.Println("Starting Full-Featured SMPP Server...")

	// Load configuration
	configPath := "configs/server.json"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	configManager := config.NewConfigManager(configPath)
	cfg, err := configManager.LoadConfig()
	if err != nil {
		log.Printf("Failed to load config, using defaults: %v", err)
		cfg = &smpp.Config{
			Server: smpp.ServerConfig{
				Host:               "localhost",
				Port:               2775,
				MaxConnections:     10,
				ReadTimeout:        30 * time.Second,
				WriteTimeout:       10 * time.Second,
				IdleTimeout:        300 * time.Second,
				EnquireLinkTimeout: 60 * time.Second,
				BindTimeout:        30 * time.Second,
				LogLevel:           "info",
			},
			Logging: smpp.LoggingConfig{
				Level:  "info",
				Format: "text",
				Output: "stdout",
			},
			Storage: smpp.StorageConfig{
				Type:    "memory",
				DataDir: "./data",
			},
		}
	}

	serverCfg := configManager.GetServerConfig()

	// Create logger
	appLogger := logger.NewDefaultLogger(cfg.Logging.Level)
	appLogger.Info("Initializing SMPP server components")

	// Create event bus
	eventBus := events.NewAsyncEventBus(appLogger, 1000)

	// Create storage
	var smsStorage smpp.SMSStorage
	var reportStorage smpp.ReportStorage

	switch cfg.Storage.Type {
	case "file":
		var err1, err2 error
		smsStorage, err1 = storage.NewFileSMSStorage(cfg.Storage.DataDir, appLogger)
		reportStorage, err2 = storage.NewFileReportStorage(cfg.Storage.DataDir, appLogger)
		if err1 != nil || err2 != nil {
			appLogger.Fatal("Failed to create file storage", "smsError", err1, "reportError", err2)
		}
		appLogger.Info("File-based storage initialized", "dataDir", cfg.Storage.DataDir)
	default:
		smsStorage = storage.NewInMemorySMSStorage(appLogger)
		reportStorage = storage.NewInMemoryReportStorage(appLogger)
		appLogger.Info("In-memory storage initialized")
	}

	// Create authentication provider
	userAuth := auth.NewDefaultUserAuth(appLogger)
	appLogger.Info("Authentication system initialized")

	// Create message routing components
	router := smpp.NewDefaultMessageRouter()
	queue := smpp.NewInMemoryMessageQueue(1000)
	splitter := smpp.NewDefaultMessageSplitter()

	// Add some default routing rules
	router.AddRoute("1", "usa-gateway")    // US numbers
	router.AddRoute("44", "uk-gateway")    // UK numbers
	router.AddRoute("91", "india-gateway") // India numbers

	// Create message handler
	messageHandlerDeps := smpp.MessageHandlerDependencies{
		SMSStorage:       smsStorage,
		ReportStorage:    reportStorage,
		EventPublisher:   eventBus,
		Logger:           appLogger,
		MetricsCollector: nil, // Can add metrics later
		Router:           router,
		Queue:            queue,
		Splitter:         splitter,
	}
	messageHandler := smpp.NewMessageHandler(messageHandlerDeps)
	appLogger.Info("Message handler initialized")

	// Create server dependencies
	deps := smpp.ServerDependencies{
		ConnectionManager: smpp.NewInMemoryConnectionManager(appLogger), // Add connection manager
		UserAuth:          userAuth,
		SMSStorage:        smsStorage,    // Add SMS storage
		ReportStorage:     reportStorage, // Add report storage
		MessageHandler:    messageHandler,
		EventPublisher:    eventBus,
		Logger:            appLogger,
		MetricsCollector:  nil, // Can add metrics later
	}

	// Create server
	server := smpp.NewServer(serverCfg, deps)

	// Create delivery report sender
	deliveryReportSender := smpp.NewDeliveryReportSender(deps.ConnectionManager, appLogger)

	// Set up event handlers
	setupEventHandlers(eventBus, deliveryReportSender, deps.ConnectionManager, appLogger)

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		appLogger.Fatal("Failed to start server", "error", err)
	}

	appLogger.Info("SMPP Server started successfully",
		"host", serverCfg.Host,
		"port", serverCfg.Port,
		"maxConnections", serverCfg.MaxConnections)

	log.Println("Available test users:")
	log.Println("  - test/test (basic permissions)")
	log.Println("  - client1/password1 (full permissions)")
	log.Println("  - esme/esme123 (limited permissions)")

	// For testing: run for 20 seconds then exit
	appLogger.Info("Server running for testing - will exit in 20 seconds")
	time.Sleep(20 * time.Second)
	appLogger.Info("Test period ended, stopping server...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Stop(shutdownCtx); err != nil {
		appLogger.Error("Error during server shutdown", "error", err)
	} else {
		appLogger.Info("Server stopped gracefully")
	}
}

// setupEventHandlers configures event handlers for logging and monitoring
func setupEventHandlers(eventBus smpp.EventPublisher, deliveryReportSender *smpp.DeliveryReportSender, connectionManager smpp.ConnectionManager, logger smpp.Logger) {
	ctx := context.Background()

	// Connection manager event handler
	connMgrHandler := smpp.NewConnectionManagerEventHandler(connectionManager, logger)
	if err := eventBus.Subscribe(ctx, smpp.EventTypeConnected, connMgrHandler); err != nil {
		logger.Error("Failed to subscribe connection manager to connected events", "error", err)
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeDisconnected, connMgrHandler); err != nil {
		logger.Error("Failed to subscribe connection manager to disconnected events", "error", err)
	}

	// Connection event handler
	connectionHandler := &LoggingEventHandler{
		EventType: "connection",
		Logger:    logger,
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeConnected, connectionHandler); err != nil {
		logger.Error("Failed to subscribe to connection events", "error", err)
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeDisconnected, connectionHandler); err != nil {
		logger.Error("Failed to subscribe to connection closed events", "error", err)
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeBound, connectionHandler); err != nil {
		logger.Error("Failed to subscribe to bound events", "error", err)
	}
	if err := eventBus.Subscribe(ctx, smpp.EventTypeUnbound, connectionHandler); err != nil {
		logger.Error("Failed to subscribe to unbound events", "error", err)
	}

	// Message event handler
	messageHandler := &LoggingEventHandler{
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

	// Delivery report sender
	if err := eventBus.Subscribe(ctx, smpp.EventTypeDeliveryReport, deliveryReportSender); err != nil {
		logger.Error("Failed to subscribe delivery report sender to delivery events", "error", err)
	}

	logger.Info("Event handlers configured successfully")
}

// LoggingEventHandler logs all events
type LoggingEventHandler struct {
	EventType string
	Logger    smpp.Logger
}

func (h *LoggingEventHandler) HandleEvent(ctx context.Context, event smpp.Event) error {
	h.Logger.Info("Event received",
		"type", string(event.GetEventType()),
		"timestamp", event.GetTimestamp(),
		"handler", h.EventType,
		"data", event.GetData())
	return nil
}

func (h *LoggingEventHandler) GetHandlerID() string {
	return "logging_" + h.EventType
}
