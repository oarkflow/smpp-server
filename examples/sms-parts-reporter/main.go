package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oarkflow/smpp-server/internal/config"
	"github.com/oarkflow/smpp-server/internal/logger"
	"github.com/oarkflow/smpp-server/pkg/events"
	"github.com/oarkflow/smpp-server/pkg/smpp"
)

// SMSReport tracks the status of SMS messages and their parts
type SMSReport struct {
	MessageID      string
	BaseMessageID  string
	TotalParts     int
	SubmittedParts int
	DeliveredParts int
	FailedParts    int
	PartStatuses   map[string]string // part_id -> status
	SubmitTime     time.Time
	FirstDelivery  *time.Time
	FinalDelivery  *time.Time
	Status         string // PENDING, PARTIAL, COMPLETE, FAILED
	mu             sync.RWMutex
}

// SMSReporter manages SMS reports for tracking message parts
type SMSReporter struct {
	reports map[string]*SMSReport
	mu      sync.RWMutex
	logger  smpp.Logger
}

// NewSMSReporter creates a new SMS reporter
func NewSMSReporter(logger smpp.Logger) *SMSReporter {
	return &SMSReporter{
		reports: make(map[string]*SMSReport),
		logger:  logger,
	}
}

// TrackMessage starts tracking a message
func (sr *SMSReporter) TrackMessage(messageID string, totalParts int) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	report := &SMSReport{
		MessageID:     messageID,
		BaseMessageID: sr.extractBaseID(messageID),
		TotalParts:    totalParts,
		PartStatuses:  make(map[string]string),
		SubmitTime:    time.Now(),
		Status:        "PENDING",
	}

	sr.reports[messageID] = report
	sr.logger.Info("Started tracking SMS message",
		"message_id", messageID,
		"base_id", report.BaseMessageID,
		"total_parts", totalParts)
}

// UpdatePartStatus updates the status of a message part
func (sr *SMSReporter) UpdatePartStatus(messageID, status string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	report, exists := sr.reports[messageID]
	if !exists {
		// Try to find by base ID
		baseID := sr.extractBaseID(messageID)
		for _, r := range sr.reports {
			if r.BaseMessageID == baseID {
				report = r
				break
			}
		}
		if report == nil {
			sr.logger.Warn("No report found for message", "message_id", messageID)
			return
		}
	}

	report.mu.Lock()
	report.PartStatuses[messageID] = status

	// Update counters
	report.SubmittedParts = 0
	report.DeliveredParts = 0
	report.FailedParts = 0

	for _, s := range report.PartStatuses {
		switch strings.ToUpper(s) {
		case "DELIVRD":
			report.DeliveredParts++
		case "ACCEPTD", "ENROUTE":
			report.SubmittedParts++
		case "EXPIRED", "DELETED", "UNDELIV", "REJECTD":
			report.FailedParts++
		}
	}

	// Update timestamps
	now := time.Now()
	if report.FirstDelivery == nil && (status == "DELIVRD" || status == "ACCEPTD") {
		report.FirstDelivery = &now
	}
	if status == "DELIVRD" {
		report.FinalDelivery = &now
	}

	// Update overall status
	sr.updateOverallStatus(report)
	report.mu.Unlock()

	sr.logger.Info("Updated SMS part status",
		"message_id", messageID,
		"part_status", status,
		"delivered_parts", report.DeliveredParts,
		"total_parts", report.TotalParts)
}

// updateOverallStatus updates the overall message status
func (sr *SMSReporter) updateOverallStatus(report *SMSReport) {
	if report.FailedParts > 0 && report.DeliveredParts == 0 {
		report.Status = "FAILED"
	} else if report.DeliveredParts == report.TotalParts {
		report.Status = "COMPLETE"
	} else if report.DeliveredParts > 0 {
		report.Status = "PARTIAL"
	} else {
		report.Status = "PENDING"
	}
}

// GetReport gets the current report for a message
func (sr *SMSReporter) GetReport(messageID string) *SMSReport {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	if report, exists := sr.reports[messageID]; exists {
		return report
	}

	// Try to find by base ID
	baseID := sr.extractBaseID(messageID)
	for _, report := range sr.reports {
		if report.BaseMessageID == baseID {
			return report
		}
	}

	return nil
}

// PrintReport prints a detailed report for a message
func (sr *SMSReporter) PrintReport(messageID string) {
	report := sr.GetReport(messageID)
	if report == nil {
		sr.logger.Warn("No report found", "message_id", messageID)
		return
	}

	report.mu.RLock()
	defer report.mu.RUnlock()

	fmt.Printf("\n=== SMS MESSAGE REPORT ===\n")
	fmt.Printf("Message ID: %s\n", report.MessageID)
	fmt.Printf("Base ID: %s\n", report.BaseMessageID)
	fmt.Printf("Total Parts: %d\n", report.TotalParts)
	fmt.Printf("Submitted Parts: %d\n", report.SubmittedParts)
	fmt.Printf("Delivered Parts: %d\n", report.DeliveredParts)
	fmt.Printf("Failed Parts: %d\n", report.FailedParts)
	fmt.Printf("Overall Status: %s\n", report.Status)
	fmt.Printf("Submit Time: %s\n", report.SubmitTime.Format("2006-01-02 15:04:05"))

	if report.FirstDelivery != nil {
		fmt.Printf("First Delivery: %s\n", report.FirstDelivery.Format("2006-01-02 15:04:05"))
	}
	if report.FinalDelivery != nil {
		fmt.Printf("Final Delivery: %s\n", report.FinalDelivery.Format("2006-01-02 15:04:05"))
	}

	fmt.Printf("\nPart Details:\n")
	for partID, status := range report.PartStatuses {
		fmt.Printf("  Part %s: %s\n", partID, status)
	}
	fmt.Printf("========================\n\n")
}

// extractBaseID extracts the base message ID from a part ID
func (sr *SMSReporter) extractBaseID(messageID string) string {
	if idx := strings.LastIndex(messageID, "_"); idx > 0 {
		return messageID[:idx]
	}
	return messageID
}

func main() {
	log.Println("Starting SMS Parts Reporter Example...")

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
	appLogger.Info("Initializing SMS Parts Reporter...")

	// Create SMS reporter
	smsReporter := NewSMSReporter(appLogger)

	// Create event bus for client events
	eventBus := events.NewAsyncEventBus(appLogger, 1000)

	// Create client dependencies
	deps := smpp.ClientDependencies{
		EventPublisher: eventBus,
		Logger:         appLogger,
	}

	// Create client
	client := smpp.NewClient(clientCfg, deps)

	// Set up client callbacks with SMS reporting
	setupReportingCallbacks(client, smsReporter, appLogger)

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

	// Wait for bind to complete
	time.Sleep(3 * time.Second)

	// Send test messages
	sendTestMessages(client, smsReporter, appLogger)

	// Wait for delivery reports
	appLogger.Info("Waiting for delivery reports...")
	time.Sleep(15 * time.Second)

	// Print final reports
	appLogger.Info("Generating final SMS reports...")
	smsReporter.PrintReport("MSG_test_001") // Short message
	smsReporter.PrintReport("MSG_test_002") // Long message

	appLogger.Info("SMS Parts Reporter example complete")
}

// sendTestMessages sends test messages for reporting
func sendTestMessages(client *smpp.Client, reporter *SMSReporter, logger smpp.Logger) {
	sourceAddr := "12345"
	destAddr := "67890"

	// Send short message
	shortMessage := "Short SMS test message"
	logger.Info("Sending short message", "text", shortMessage)

	// Track the message (assuming it will be single part)
	reporter.TrackMessage("MSG_test_001", 1)

	ctx1, cancel1 := context.WithTimeout(context.Background(), 30*time.Second)
	err := client.SendSMS(ctx1, sourceAddr, destAddr, shortMessage)
	cancel1()

	if err != nil {
		logger.Error("Failed to send short message", "error", err)
	}

	time.Sleep(2 * time.Second)

	// Send long message
	longMessage := "This is a very long SMS message that will be split into multiple parts. " +
		"Each part will have its own delivery report. This demonstrates how SMPP handles " +
		"concatenated SMS messages with proper part tracking and status reporting. " +
		"The message is designed to exceed 160 characters significantly."

	logger.Info("Sending long message", "length", len(longMessage))

	// Estimate parts (rough calculation)
	estimatedParts := (len(longMessage) + 153) / 154
	reporter.TrackMessage("MSG_test_002", estimatedParts)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	err = client.SendSMS(ctx2, sourceAddr, destAddr, longMessage)
	cancel2()

	if err != nil {
		logger.Error("Failed to send long message", "error", err)
	}
}

// setupReportingCallbacks sets up callbacks with SMS reporting
func setupReportingCallbacks(client *smpp.Client, reporter *SMSReporter, logger smpp.Logger) {
	client.OnSubmitResponse(func(messageID string, err error) {
		if err != nil {
			logger.Error("Submit failed", "messageID", messageID, "error", err)
			reporter.UpdatePartStatus(messageID, "FAILED")
		} else {
			logger.Info("Message part submitted successfully", "messageID", messageID)
			reporter.UpdatePartStatus(messageID, "SUBMITTED")
		}
	})

	client.OnDeliveryReport(func(report *smpp.DeliveryReport) {
		logger.Info("Delivery report received",
			"messageID", report.MessageID,
			"status", report.Status)

		reporter.UpdatePartStatus(report.MessageID, report.Status)

		// Print current report status
		if r := reporter.GetReport(report.MessageID); r != nil {
			logger.Info("Current message status",
				"message_id", r.MessageID,
				"status", r.Status,
				"delivered_parts", r.DeliveredParts,
				"total_parts", r.TotalParts)
		}
	})
}
