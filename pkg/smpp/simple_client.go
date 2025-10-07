package smpp

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// simpleLogger implements the Logger interface with basic logging
type simpleLogger struct {
	level string
}

func (l *simpleLogger) Debug(msg string, fields ...interface{}) {
	if l.level == "debug" {
		var parts []string
		parts = append(parts, "[DEBUG]", msg)
		for i := 0; i < len(fields); i += 2 {
			if i+1 < len(fields) {
				key := fmt.Sprintf("%v", fields[i])
				value := fmt.Sprintf("%v", fields[i+1])
				parts = append(parts, fmt.Sprintf("%s=%s", key, value))
			}
		}
		log.Println(strings.Join(parts, " "))
	}
}

func (l *simpleLogger) Info(msg string, fields ...interface{}) {
	if l.level == "debug" || l.level == "info" {
		var parts []string
		parts = append(parts, "[INFO]", msg)
		for i := 0; i < len(fields); i += 2 {
			if i+1 < len(fields) {
				key := fmt.Sprintf("%v", fields[i])
				value := fmt.Sprintf("%v", fields[i+1])
				parts = append(parts, fmt.Sprintf("%s=%s", key, value))
			}
		}
		log.Println(strings.Join(parts, " "))
	}
}

func (l *simpleLogger) Warn(msg string, fields ...interface{}) {
	if l.level != "error" {
		var parts []string
		parts = append(parts, "[WARN]", msg)
		for i := 0; i < len(fields); i += 2 {
			if i+1 < len(fields) {
				key := fmt.Sprintf("%v", fields[i])
				value := fmt.Sprintf("%v", fields[i+1])
				parts = append(parts, fmt.Sprintf("%s=%s", key, value))
			}
		}
		log.Println(strings.Join(parts, " "))
	}
}

func (l *simpleLogger) Error(msg string, fields ...interface{}) {
	var parts []string
	parts = append(parts, "[ERROR]", msg)
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key := fmt.Sprintf("%v", fields[i])
			value := fmt.Sprintf("%v", fields[i+1])
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		}
	}
	log.Println(strings.Join(parts, " "))
}

func (l *simpleLogger) Fatal(msg string, fields ...interface{}) {
	var parts []string
	parts = append(parts, "[FATAL]", msg)
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key := fmt.Sprintf("%v", fields[i])
			value := fmt.Sprintf("%v", fields[i+1])
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		}
	}
	log.Fatalln(strings.Join(parts, " "))
}

func (l *simpleLogger) WithFields(fields map[string]interface{}) Logger {
	return l // Simple implementation
}

// SimpleClient provides a simplified SMPP client API
type SimpleClient struct {
	client *Client
	logger Logger
}

// SimpleClientOptions contains configuration options for the simple client
type SimpleClientOptions struct {
	Host                 string
	Port                 int
	SystemID             string
	Password             string
	SystemType           string
	BindType             string
	ConnectTimeout       time.Duration
	ReadTimeout          time.Duration
	WriteTimeout         time.Duration
	EnquireLinkInterval  time.Duration
	ReconnectInterval    time.Duration
	MaxReconnectAttempts int
	LogLevel             string
}

// DefaultSimpleClientOptions returns default options for the simple client
func DefaultSimpleClientOptions() *SimpleClientOptions {
	return &SimpleClientOptions{
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
	}
}

// New creates a new simple SMPP client with the given options
func New(opts ...*SimpleClientOptions) (*SimpleClient, error) {
	options := DefaultSimpleClientOptions()
	if len(opts) > 0 && opts[0] != nil {
		opt := opts[0]
		if opt.Host != "" {
			options.Host = opt.Host
		}
		if opt.Port != 0 {
			options.Port = opt.Port
		}
		if opt.SystemID != "" {
			options.SystemID = opt.SystemID
		}
		if opt.Password != "" {
			options.Password = opt.Password
		}
		if opt.SystemType != "" {
			options.SystemType = opt.SystemType
		}
		if opt.BindType != "" {
			options.BindType = opt.BindType
		}
		if opt.ConnectTimeout != 0 {
			options.ConnectTimeout = opt.ConnectTimeout
		}
		if opt.ReadTimeout != 0 {
			options.ReadTimeout = opt.ReadTimeout
		}
		if opt.WriteTimeout != 0 {
			options.WriteTimeout = opt.WriteTimeout
		}
		if opt.EnquireLinkInterval != 0 {
			options.EnquireLinkInterval = opt.EnquireLinkInterval
		}
		if opt.ReconnectInterval != 0 {
			options.ReconnectInterval = opt.ReconnectInterval
		}
		if opt.MaxReconnectAttempts != 0 {
			options.MaxReconnectAttempts = opt.MaxReconnectAttempts
		}
		if opt.LogLevel != "" {
			options.LogLevel = opt.LogLevel
		}
	}

	// Create logger
	var appLogger Logger
	switch options.LogLevel {
	case "debug":
		appLogger = &simpleLogger{level: "debug"}
	case "info":
		appLogger = &simpleLogger{level: "info"}
	case "warn":
		appLogger = &simpleLogger{level: "warn"}
	case "error":
		appLogger = &simpleLogger{level: "error"}
	default:
		appLogger = &simpleLogger{level: "info"}
	}

	// Create client config
	config := &ClientConfig{
		Host:                 options.Host,
		Port:                 options.Port,
		SystemID:             options.SystemID,
		Password:             options.Password,
		SystemType:           options.SystemType,
		BindType:             options.BindType,
		ConnectTimeout:       options.ConnectTimeout,
		ReadTimeout:          options.ReadTimeout,
		WriteTimeout:         options.WriteTimeout,
		EnquireLinkInterval:  options.EnquireLinkInterval,
		ReconnectInterval:    options.ReconnectInterval,
		MaxReconnectAttempts: options.MaxReconnectAttempts,
		LogLevel:             options.LogLevel,
	}

	// Create client dependencies (no event publisher or metrics for simple client)
	deps := ClientDependencies{
		EventPublisher:   nil, // Simple client doesn't use events
		Logger:           appLogger,
		MetricsCollector: nil, // Simple client doesn't use metrics
	}

	// Create the underlying client
	client := NewClient(config, deps)

	return &SimpleClient{
		client: client,
		logger: appLogger,
	}, nil
}

// Connect connects to the SMPP server and binds automatically
func (sc *SimpleClient) Connect(ctx context.Context) error {
	// Connect to server
	if err := sc.client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Bind to server
	if err := sc.client.Bind(ctx); err != nil {
		return fmt.Errorf("failed to bind: %w", err)
	}

	// Wait a moment for binding to complete
	time.Sleep(1 * time.Second)

	return nil
}

// Disconnect disconnects from the SMPP server
func (sc *SimpleClient) Disconnect(ctx context.Context) error {
	return sc.client.Disconnect(ctx)
}

// OnReport sets the callback for delivery reports
func (sc *SimpleClient) OnReport(callback func(report *DeliveryReport)) {
	sc.client.OnDeliveryReport(callback)
}

// SendSMS sends an SMS message from sourceAddr to destAddr with the given message
func (sc *SimpleClient) SendSMS(from, message, to string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return sc.client.SendSMS(ctx, from, to, message)
}

// SendSMSWithOptions sends an SMS message with additional options
func (sc *SimpleClient) SendSMSWithOptions(options *SMSOptions) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return sc.client.SendSMS(ctx, options.From, options.To, options.Message)
}

// SMSOptions contains options for sending SMS
type SMSOptions struct {
	From    string
	To      string
	Message string
	// Additional options can be added here in the future
}

// IsConnected returns true if the client is connected
func (sc *SimpleClient) IsConnected() bool {
	return sc.client.IsConnected()
}

// IsBound returns true if the client is bound
func (sc *SimpleClient) IsBound() bool {
	return sc.client.IsBound()
}
