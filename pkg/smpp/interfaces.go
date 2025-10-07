package smpp

import (
	"context"
	"encoding/json"
	"time"
)

// Duration wraps time.Duration to allow JSON unmarshaling
type Duration time.Duration

// UnmarshalJSON implements json.Unmarshaler
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	duration, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(duration)
	return nil
}

// MarshalJSON implements json.Marshaler
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// ToDuration converts to time.Duration
func (d Duration) ToDuration() time.Duration {
	return time.Duration(d)
}

// Configuration interfaces
type Config struct {
	Server  ServerConfig  `json:"server"`
	Client  ClientConfig  `json:"client"`
	Logging LoggingConfig `json:"logging"`
	Storage StorageConfig `json:"storage"`
	Metrics MetricsConfig `json:"metrics"`
}

type LoggingConfig struct {
	Level      string `json:"level"`
	Format     string `json:"format"`
	Output     string `json:"output"`
	File       string `json:"file"`
	MaxSize    int    `json:"max_size"`
	MaxBackups int    `json:"max_backups"`
	MaxAge     int    `json:"max_age"`
	Compress   bool   `json:"compress"`
}

type StorageConfig struct {
	Type     string         `json:"type"`
	DataDir  string         `json:"data_dir"`
	Database DatabaseConfig `json:"database"`
}

type DatabaseConfig struct {
	Driver   string `json:"driver"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
	SSLMode  string `json:"ssl_mode"`
}

type MetricsConfig struct {
	Enabled   bool   `json:"enabled"`
	Port      int    `json:"port"`
	Path      string `json:"path"`
	Namespace string `json:"namespace"`
	Subsystem string `json:"subsystem"`
}

type ConfigManager interface {
	LoadConfig() (*Config, error)
	SaveConfig() error
	GetServerConfig() *ServerConfig
	GetClientConfig() *ClientConfig
	UpdateConfig(config interface{}) error
	Reload() error
	Validate() error
}

// ServerConfig represents server configuration
type ServerConfig struct {
	Host               string        `json:"host"`
	Port               int           `json:"port"`
	MaxConnections     int           `json:"max_connections"`
	ReadTimeout        time.Duration `json:"read_timeout"`
	WriteTimeout       time.Duration `json:"write_timeout"`
	IdleTimeout        time.Duration `json:"idle_timeout"`
	EnquireLinkTimeout time.Duration `json:"enquire_link_timeout"`
	BindTimeout        time.Duration `json:"bind_timeout"`
	TLSEnabled         bool          `json:"tls_enabled"`
	TLSCertFile        string        `json:"tls_cert_file"`
	TLSKeyFile         string        `json:"tls_key_file"`
	LogLevel           string        `json:"log_level"`
	LogFile            string        `json:"log_file"`
	MetricsEnabled     bool          `json:"metrics_enabled"`
	MetricsPort        int           `json:"metrics_port"`
}

// ClientConfig represents client configuration
type ClientConfig struct {
	Host                 string        `json:"host"`
	Port                 int           `json:"port"`
	SystemID             string        `json:"system_id"`
	Password             string        `json:"password"`
	SystemType           string        `json:"system_type"`
	BindType             string        `json:"bind_type"`
	ConnectTimeout       time.Duration `json:"connect_timeout"`
	ReadTimeout          time.Duration `json:"read_timeout"`
	WriteTimeout         time.Duration `json:"write_timeout"`
	EnquireLinkInterval  time.Duration `json:"enquire_link_interval"`
	ReconnectInterval    time.Duration `json:"reconnect_interval"`
	MaxReconnectAttempts int           `json:"max_reconnect_attempts"`
	TLSEnabled           bool          `json:"tls_enabled"`
	TLSSkipVerify        bool          `json:"tls_skip_verify"`
	LogLevel             string        `json:"log_level"`
}

// Storage interfaces

// DeliveryReport represents a delivery receipt (forward declaration)
type DeliveryReport struct {
	MessageID      string
	SubmitDate     string
	DoneDate       string
	Status         string
	Error          string
	Text           string
	SubmittedParts int
	DeliveredParts int
	Timestamp      time.Time // When the report was generated
	SubmitTime     time.Time // Parsed submit date
	DoneTime       time.Time // Parsed done date
}

// SMSStorage interface defines operations for storing and retrieving SMS messages
type SMSStorage interface {
	// StoreSMS stores an SMS message and returns the message ID
	StoreSMS(ctx context.Context, message *Message) (string, error)

	// GetSMS retrieves an SMS message by ID
	GetSMS(ctx context.Context, messageID string) (*Message, error)

	// UpdateSMSStatus updates the status of an SMS message
	UpdateSMSStatus(ctx context.Context, messageID string, status MessageStatus) error

	// SearchSMS searches for SMS messages based on criteria
	SearchSMS(ctx context.Context, criteria *SMSSearchCriteria) ([]*Message, error)

	// DeleteSMS deletes an SMS message
	DeleteSMS(ctx context.Context, messageID string) error

	// GetSMSByDateRange retrieves SMS messages within a date range
	GetSMSByDateRange(ctx context.Context, from, to time.Time) ([]*Message, error)

	// GetSMSCount returns the total count of SMS messages
	GetSMSCount(ctx context.Context) (int64, error)
}

// ReportStorage interface defines operations for storing and retrieving delivery reports
type ReportStorage interface {
	// StoreReport stores a delivery report
	StoreReport(ctx context.Context, report *DeliveryReport) error

	// GetReport retrieves a delivery report by message ID
	GetReport(ctx context.Context, messageID string) (*DeliveryReport, error)

	// GetReportsByDateRange retrieves delivery reports within a date range
	GetReportsByDateRange(ctx context.Context, from, to time.Time) ([]*DeliveryReport, error)

	// SearchReports searches for delivery reports based on criteria
	SearchReports(ctx context.Context, criteria *ReportSearchCriteria) ([]*DeliveryReport, error)

	// DeleteReport deletes a delivery report
	DeleteReport(ctx context.Context, messageID string) error

	// GetReportCount returns the total count of delivery reports
	GetReportCount(ctx context.Context) (int64, error)
}

// UserAuth interface defines user authentication operations
type UserAuth interface {
	// Authenticate validates user credentials
	Authenticate(ctx context.Context, systemID, password string) (*User, error)

	// CreateUser creates a new user
	CreateUser(ctx context.Context, user *User) error

	// UpdateUser updates user information
	UpdateUser(ctx context.Context, user *User) error

	// DeleteUser deletes a user
	DeleteUser(ctx context.Context, systemID string) error

	// GetUser retrieves user information
	GetUser(ctx context.Context, systemID string) (*User, error)

	// ListUsers returns all users
	ListUsers(ctx context.Context) ([]*User, error)

	// IsAuthorized checks if a user is authorized for a specific operation
	IsAuthorized(ctx context.Context, systemID string, operation Operation) (bool, error)
}

// MessageHandler interface defines message processing operations
type MessageHandler interface {
	// HandleSubmitSM processes a submit_sm PDU
	HandleSubmitSM(ctx context.Context, session *Session, pdu *SubmitSM) (*SubmitSMResp, error)

	// HandleDeliverSM processes a deliver_sm PDU
	HandleDeliverSM(ctx context.Context, session *Session, pdu *DeliverSM) (*DeliverSMResp, error)

	// ProcessMessage processes an SMS message for delivery
	ProcessMessage(ctx context.Context, message *Message) error

	// GenerateDeliveryReport generates a delivery report for a message
	GenerateDeliveryReport(ctx context.Context, message *Message, status MessageStatus) (*DeliveryReport, error)

	// ValidateMessage validates an SMS message
	ValidateMessage(ctx context.Context, message *Message) error
}

// ConnectionManager interface defines connection management operations
type ConnectionManager interface {
	// AddConnection adds a new connection
	AddConnection(ctx context.Context, conn Connection) error

	// RemoveConnection removes a connection
	RemoveConnection(ctx context.Context, sessionID string) error

	// GetConnection retrieves a connection by session ID
	GetConnection(ctx context.Context, sessionID string) (Connection, error)

	// GetAllConnections returns all active connections
	GetAllConnections(ctx context.Context) ([]Connection, error)

	// BroadcastMessage broadcasts a message to all connections
	BroadcastMessage(ctx context.Context, message interface{}) error

	// GetConnectionCount returns the number of active connections
	GetConnectionCount(ctx context.Context) int
}

// Connection interface defines a connection to an SMPP client/server
type Connection interface {
	// GetSessionID returns the session ID
	GetSessionID() string

	// GetSession returns the session information
	GetSession() *Session

	// Send sends a PDU to the connection
	Send(ctx context.Context, pdu *PDU) error

	// Close closes the connection
	Close() error

	// IsActive returns true if the connection is active
	IsActive() bool

	// GetRemoteAddr returns the remote address
	GetRemoteAddr() string

	// GetLastActivity returns the last activity time
	GetLastActivity() time.Time

	// UpdateLastActivity updates the last activity time
	UpdateLastActivity()
}

// EventPublisher interface defines event publishing operations
type EventPublisher interface {
	// PublishSMSEvent publishes an SMS-related event
	PublishSMSEvent(ctx context.Context, event *SMSEvent) error

	// PublishConnectionEvent publishes a connection-related event
	PublishConnectionEvent(ctx context.Context, event *ConnectionEvent) error

	// PublishDeliveryEvent publishes a delivery report event
	PublishDeliveryEvent(ctx context.Context, event *DeliveryEvent) error

	// Subscribe subscribes to events of a specific type
	Subscribe(ctx context.Context, eventType EventType, handler EventHandler) error

	// Unsubscribe unsubscribes from events
	Unsubscribe(ctx context.Context, eventType EventType, handler EventHandler) error
}

// EventHandler interface defines event handling operations
type EventHandler interface {
	// HandleEvent handles an event
	HandleEvent(ctx context.Context, event Event) error

	// GetHandlerID returns a unique identifier for this handler
	GetHandlerID() string
}

// Logger interface defines logging operations
type Logger interface {
	// Debug logs a debug message
	Debug(msg string, fields ...interface{})

	// Info logs an info message
	Info(msg string, fields ...interface{})

	// Warn logs a warning message
	Warn(msg string, fields ...interface{})

	// Error logs an error message
	Error(msg string, fields ...interface{})

	// Fatal logs a fatal message and exits
	Fatal(msg string, fields ...interface{})

	// WithFields returns a logger with additional fields
	WithFields(fields map[string]interface{}) Logger
}

// MetricsCollector interface defines metrics collection operations
type MetricsCollector interface {
	// IncCounter increments a counter metric
	IncCounter(name string, labels map[string]string)

	// SetGauge sets a gauge metric
	SetGauge(name string, value float64, labels map[string]string)

	// ObserveHistogram observes a value for a histogram metric
	ObserveHistogram(name string, value float64, labels map[string]string)

	// RecordDuration records a duration metric
	RecordDuration(name string, duration time.Duration, labels map[string]string)
}

// Supporting data structures

// SMSSearchCriteria defines criteria for searching SMS messages
type SMSSearchCriteria struct {
	SourceAddr string
	DestAddr   string
	Status     *MessageStatus
	FromDate   *time.Time
	ToDate     *time.Time
	Limit      int
	Offset     int
	SortBy     string // "submit_time", "status", etc.
	SortOrder  string // "asc", "desc"
}

// ReportSearchCriteria defines criteria for searching delivery reports
type ReportSearchCriteria struct {
	MessageID string
	Status    string
	FromDate  *time.Time
	ToDate    *time.Time
	Limit     int
	Offset    int
	SortBy    string // "timestamp", "status", etc.
	SortOrder string // "asc", "desc"
}

// User represents a system user
type User struct {
	ID             string
	SystemID       string
	Password       string
	PasswordHash   string
	Salt           string
	Name           string
	Email          string
	SystemType     string
	Permissions    map[Operation]bool
	MaxConnections int
	RateLimit      int // Messages per minute
	Active         bool
	Enabled        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastLogin      time.Time
	LoginCount     int64
}

// Permission represents a user permission
type Permission struct {
	Operation Operation
	Resource  string
}

// Operation represents an operation type
type Operation string

const (
	OperationBind    Operation = "bind"
	OperationSubmit  Operation = "submit"
	OperationDeliver Operation = "deliver"
	OperationQuery   Operation = "query"
	OperationCancel  Operation = "cancel"
	OperationReplace Operation = "replace"
	OperationUnbind  Operation = "unbind"
)

// Event represents a system event
type Event interface {
	GetEventType() EventType
	GetTimestamp() time.Time
	GetData() map[string]interface{}
}

// EventType represents the type of event
type EventType string

const (
	EventTypeSMSSubmitted   EventType = "sms.submitted"
	EventTypeSMSDelivered   EventType = "sms.delivered"
	EventTypeSMSFailed      EventType = "sms.failed"
	EventTypeConnected      EventType = "connection.connected"
	EventTypeDisconnected   EventType = "connection.disconnected"
	EventTypeBound          EventType = "connection.bound"
	EventTypeUnbound        EventType = "connection.unbound"
	EventTypeDeliveryReport EventType = "delivery.report"
)

// SMSEvent represents an SMS-related event
type SMSEvent struct {
	Type      EventType
	Timestamp time.Time
	MessageID string
	Session   *Session
	Message   *Message
	Error     error
	Data      map[string]interface{}
}

func (e *SMSEvent) GetEventType() EventType {
	return e.Type
}

func (e *SMSEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e *SMSEvent) GetData() map[string]interface{} {
	return e.Data
}

// ConnectionEvent represents a connection-related event
type ConnectionEvent struct {
	Type       EventType
	Timestamp  time.Time
	SessionID  string
	Session    *Session
	RemoteAddr string
	Error      error
	Data       map[string]interface{}
}

func (e *ConnectionEvent) GetEventType() EventType {
	return e.Type
}

func (e *ConnectionEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e *ConnectionEvent) GetData() map[string]interface{} {
	return e.Data
}

// DeliveryEvent represents a delivery report event
type DeliveryEvent struct {
	Type      EventType
	Timestamp time.Time
	MessageID string
	Report    *DeliveryReport
	Session   *Session
	Data      map[string]interface{}
}

func (e *DeliveryEvent) GetEventType() EventType {
	return e.Type
}

func (e *DeliveryEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e *DeliveryEvent) GetData() map[string]interface{} {
	return e.Data
}
