package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/oarkflow/smpp-server/pkg/smpp"
)

// ConfigManager manages application configuration
type ConfigManager struct {
	configPath string
	config     *smpp.Config
}

// configJSON represents the JSON structure for configuration
type configJSON struct {
	Server  serverConfigJSON   `json:"server"`
	Client  clientConfigJSON   `json:"client"`
	Logging smpp.LoggingConfig `json:"logging"`
	Storage smpp.StorageConfig `json:"storage"`
	Metrics smpp.MetricsConfig `json:"metrics"`
}

type serverConfigJSON struct {
	Host               string `json:"host"`
	Port               int    `json:"port"`
	MaxConnections     int    `json:"max_connections"`
	ReadTimeout        string `json:"read_timeout"`
	WriteTimeout       string `json:"write_timeout"`
	IdleTimeout        string `json:"idle_timeout"`
	EnquireLinkTimeout string `json:"enquire_link_timeout"`
	BindTimeout        string `json:"bind_timeout"`
	TLSEnabled         bool   `json:"tls_enabled"`
	TLSCertFile        string `json:"tls_cert_file"`
	TLSKeyFile         string `json:"tls_key_file"`
	LogLevel           string `json:"log_level"`
	LogFile            string `json:"log_file"`
	MetricsEnabled     bool   `json:"metrics_enabled"`
	MetricsPort        int    `json:"metrics_port"`
}

type clientConfigJSON struct {
	Host                 string `json:"host"`
	Port                 int    `json:"port"`
	SystemID             string `json:"system_id"`
	Password             string `json:"password"`
	SystemType           string `json:"system_type"`
	BindType             string `json:"bind_type"`
	ConnectTimeout       string `json:"connect_timeout"`
	ReadTimeout          string `json:"read_timeout"`
	WriteTimeout         string `json:"write_timeout"`
	EnquireLinkInterval  string `json:"enquire_link_interval"`
	ReconnectInterval    string `json:"reconnect_interval"`
	MaxReconnectAttempts int    `json:"max_reconnect_attempts"`
	TLSEnabled           bool   `json:"tls_enabled"`
	TLSSkipVerify        bool   `json:"tls_skip_verify"`
	LogLevel             string `json:"log_level"`
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(configPath string) *ConfigManager {
	return &ConfigManager{
		configPath: configPath,
	}
}

// LoadConfig loads configuration from file
func (cm *ConfigManager) LoadConfig() (*smpp.Config, error) {
	// Set default configuration
	config := cm.getDefaultConfig()

	// If config file exists, load it
	if cm.configPath != "" && cm.fileExists(cm.configPath) {
		data, err := os.ReadFile(cm.configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		var jsonConfig configJSON
		if err := json.Unmarshal(data, &jsonConfig); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}

		// Convert JSON config to internal config
		if err := cm.convertJSONConfig(&jsonConfig, config); err != nil {
			return nil, fmt.Errorf("failed to convert config: %w", err)
		}
	}

	cm.config = config

	// Validate configuration
	if err := cm.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// convertJSONConfig converts JSON config structure to internal config
func (cm *ConfigManager) convertJSONConfig(jsonConfig *configJSON, config *smpp.Config) error {
	// Convert server config
	if err := cm.convertServerConfig(&jsonConfig.Server, &config.Server); err != nil {
		return fmt.Errorf("failed to convert server config: %w", err)
	}

	// Convert client config
	if err := cm.convertClientConfig(&jsonConfig.Client, &config.Client); err != nil {
		return fmt.Errorf("failed to convert client config: %w", err)
	}

	// Copy other configs directly
	config.Logging = jsonConfig.Logging
	config.Storage = jsonConfig.Storage
	config.Metrics = jsonConfig.Metrics

	return nil
}

// convertServerConfig converts server config with duration parsing
func (cm *ConfigManager) convertServerConfig(jsonConfig *serverConfigJSON, config *smpp.ServerConfig) error {
	config.Host = jsonConfig.Host
	config.Port = jsonConfig.Port
	config.MaxConnections = jsonConfig.MaxConnections
	config.TLSEnabled = jsonConfig.TLSEnabled
	config.TLSCertFile = jsonConfig.TLSCertFile
	config.TLSKeyFile = jsonConfig.TLSKeyFile
	config.LogLevel = jsonConfig.LogLevel
	config.LogFile = jsonConfig.LogFile
	config.MetricsEnabled = jsonConfig.MetricsEnabled
	config.MetricsPort = jsonConfig.MetricsPort

	// Parse durations
	var err error
	if config.ReadTimeout, err = time.ParseDuration(jsonConfig.ReadTimeout); err != nil {
		return fmt.Errorf("invalid read_timeout: %w", err)
	}
	if config.WriteTimeout, err = time.ParseDuration(jsonConfig.WriteTimeout); err != nil {
		return fmt.Errorf("invalid write_timeout: %w", err)
	}
	if config.IdleTimeout, err = time.ParseDuration(jsonConfig.IdleTimeout); err != nil {
		return fmt.Errorf("invalid idle_timeout: %w", err)
	}
	if config.EnquireLinkTimeout, err = time.ParseDuration(jsonConfig.EnquireLinkTimeout); err != nil {
		return fmt.Errorf("invalid enquire_link_timeout: %w", err)
	}
	if config.BindTimeout, err = time.ParseDuration(jsonConfig.BindTimeout); err != nil {
		return fmt.Errorf("invalid bind_timeout: %w", err)
	}

	return nil
}

// convertClientConfig converts client config with duration parsing
func (cm *ConfigManager) convertClientConfig(jsonConfig *clientConfigJSON, config *smpp.ClientConfig) error {
	config.Host = jsonConfig.Host
	config.Port = jsonConfig.Port
	config.SystemID = jsonConfig.SystemID
	config.Password = jsonConfig.Password
	config.SystemType = jsonConfig.SystemType
	config.BindType = jsonConfig.BindType
	config.MaxReconnectAttempts = jsonConfig.MaxReconnectAttempts
	config.TLSEnabled = jsonConfig.TLSEnabled
	config.TLSSkipVerify = jsonConfig.TLSSkipVerify
	config.LogLevel = jsonConfig.LogLevel

	// Parse durations
	var err error
	if config.ConnectTimeout, err = time.ParseDuration(jsonConfig.ConnectTimeout); err != nil {
		return fmt.Errorf("invalid connect_timeout: %w", err)
	}
	if config.ReadTimeout, err = time.ParseDuration(jsonConfig.ReadTimeout); err != nil {
		return fmt.Errorf("invalid read_timeout: %w", err)
	}
	if config.WriteTimeout, err = time.ParseDuration(jsonConfig.WriteTimeout); err != nil {
		return fmt.Errorf("invalid write_timeout: %w", err)
	}
	if config.EnquireLinkInterval, err = time.ParseDuration(jsonConfig.EnquireLinkInterval); err != nil {
		return fmt.Errorf("invalid enquire_link_interval: %w", err)
	}
	if config.ReconnectInterval, err = time.ParseDuration(jsonConfig.ReconnectInterval); err != nil {
		return fmt.Errorf("invalid reconnect_interval: %w", err)
	}

	return nil
}

// SaveConfig saves configuration to file
func (cm *ConfigManager) SaveConfig() error {
	if cm.config == nil {
		return fmt.Errorf("no configuration to save")
	}

	if cm.configPath == "" {
		return fmt.Errorf("no config path specified")
	}

	// Ensure directory exists
	dir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal configuration
	data, err := json.MarshalIndent(cm.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	// Write to file
	if err := os.WriteFile(cm.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetServerConfig returns server configuration
func (cm *ConfigManager) GetServerConfig() *smpp.ServerConfig {
	if cm.config == nil {
		return nil
	}

	return &smpp.ServerConfig{
		Host:               cm.config.Server.Host,
		Port:               cm.config.Server.Port,
		MaxConnections:     cm.config.Server.MaxConnections,
		ReadTimeout:        cm.config.Server.ReadTimeout,
		WriteTimeout:       cm.config.Server.WriteTimeout,
		IdleTimeout:        cm.config.Server.IdleTimeout,
		EnquireLinkTimeout: cm.config.Server.EnquireLinkTimeout,
		BindTimeout:        cm.config.Server.BindTimeout,
		TLSEnabled:         cm.config.Server.TLSEnabled,
		TLSCertFile:        cm.config.Server.TLSCertFile,
		TLSKeyFile:         cm.config.Server.TLSKeyFile,
		LogLevel:           cm.config.Logging.Level,
		LogFile:            cm.config.Logging.File,
		MetricsEnabled:     cm.config.Metrics.Enabled,
		MetricsPort:        cm.config.Metrics.Port,
	}
}

// GetClientConfig returns client configuration
func (cm *ConfigManager) GetClientConfig() *smpp.ClientConfig {
	if cm.config == nil {
		return nil
	}

	return &smpp.ClientConfig{
		Host:                 cm.config.Client.Host,
		Port:                 cm.config.Client.Port,
		SystemID:             cm.config.Client.SystemID,
		Password:             cm.config.Client.Password,
		SystemType:           cm.config.Client.SystemType,
		BindType:             cm.config.Client.BindType,
		ConnectTimeout:       cm.config.Client.ConnectTimeout,
		ReadTimeout:          cm.config.Client.ReadTimeout,
		WriteTimeout:         cm.config.Client.WriteTimeout,
		EnquireLinkInterval:  cm.config.Client.EnquireLinkInterval,
		ReconnectInterval:    cm.config.Client.ReconnectInterval,
		MaxReconnectAttempts: cm.config.Client.MaxReconnectAttempts,
		TLSEnabled:           cm.config.Client.TLSEnabled,
		TLSSkipVerify:        cm.config.Client.TLSSkipVerify,
		LogLevel:             cm.config.Logging.Level,
	}
}

// UpdateConfig updates configuration
func (cm *ConfigManager) UpdateConfig(config interface{}) error {
	switch c := config.(type) {
	case *smpp.Config:
		cm.config = c
	case *smpp.ServerConfig:
		if cm.config == nil {
			cm.config = cm.getDefaultConfig()
		}
		cm.config.Server = *c
	case *smpp.ClientConfig:
		if cm.config == nil {
			cm.config = cm.getDefaultConfig()
		}
		cm.config.Client = *c
	case *smpp.LoggingConfig:
		if cm.config == nil {
			cm.config = cm.getDefaultConfig()
		}
		cm.config.Logging = *c
	case *smpp.StorageConfig:
		if cm.config == nil {
			cm.config = cm.getDefaultConfig()
		}
		cm.config.Storage = *c
	case *smpp.MetricsConfig:
		if cm.config == nil {
			cm.config = cm.getDefaultConfig()
		}
		cm.config.Metrics = *c
	default:
		return fmt.Errorf("unsupported config type: %T", config)
	}

	return cm.Validate()
}

// Reload reloads configuration from source
func (cm *ConfigManager) Reload() error {
	_, err := cm.LoadConfig()
	return err
}

// Validate validates configuration
func (cm *ConfigManager) Validate() error {
	if cm.config == nil {
		return fmt.Errorf("configuration is nil")
	}

	// Validate server configuration
	if err := cm.validateServerConfig(); err != nil {
		return fmt.Errorf("invalid server config: %w", err)
	}

	// Validate client configuration
	if err := cm.validateClientConfig(); err != nil {
		return fmt.Errorf("invalid client config: %w", err)
	}

	// Validate logging configuration
	if err := cm.validateLoggingConfig(); err != nil {
		return fmt.Errorf("invalid logging config: %w", err)
	}

	// Validate storage configuration
	if err := cm.validateStorageConfig(); err != nil {
		return fmt.Errorf("invalid storage config: %w", err)
	}

	// Validate metrics configuration
	if err := cm.validateMetricsConfig(); err != nil {
		return fmt.Errorf("invalid metrics config: %w", err)
	}

	return nil
}

// validateServerConfig validates server configuration
func (cm *ConfigManager) validateServerConfig() error {
	server := &cm.config.Server

	if server.Host == "" {
		return fmt.Errorf("server host cannot be empty")
	}

	if server.Port <= 0 || server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", server.Port)
	}

	if server.MaxConnections <= 0 {
		return fmt.Errorf("max connections must be positive: %d", server.MaxConnections)
	}

	if server.ReadTimeout <= 0 {
		return fmt.Errorf("read timeout must be positive: %v", server.ReadTimeout)
	}

	if server.WriteTimeout <= 0 {
		return fmt.Errorf("write timeout must be positive: %v", server.WriteTimeout)
	}

	if server.TLSEnabled {
		if server.TLSCertFile == "" {
			return fmt.Errorf("TLS cert file required when TLS is enabled")
		}
		if server.TLSKeyFile == "" {
			return fmt.Errorf("TLS key file required when TLS is enabled")
		}
		if !cm.fileExists(server.TLSCertFile) {
			return fmt.Errorf("TLS cert file not found: %s", server.TLSCertFile)
		}
		if !cm.fileExists(server.TLSKeyFile) {
			return fmt.Errorf("TLS key file not found: %s", server.TLSKeyFile)
		}
	}

	return nil
}

// validateClientConfig validates client configuration
func (cm *ConfigManager) validateClientConfig() error {
	client := &cm.config.Client

	if client.Host == "" {
		return fmt.Errorf("client host cannot be empty")
	}

	if client.Port <= 0 || client.Port > 65535 {
		return fmt.Errorf("invalid client port: %d", client.Port)
	}

	if client.SystemID == "" {
		return fmt.Errorf("system ID cannot be empty")
	}

	if client.Password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	validBindTypes := map[string]bool{
		"transmitter": true,
		"receiver":    true,
		"transceiver": true,
	}

	if !validBindTypes[client.BindType] {
		return fmt.Errorf("invalid bind type: %s", client.BindType)
	}

	if client.ConnectTimeout <= 0 {
		return fmt.Errorf("connect timeout must be positive: %v", client.ConnectTimeout)
	}

	if client.MaxReconnectAttempts < 0 {
		return fmt.Errorf("max reconnect attempts cannot be negative: %d", client.MaxReconnectAttempts)
	}

	return nil
}

// validateLoggingConfig validates logging configuration
func (cm *ConfigManager) validateLoggingConfig() error {
	logging := &cm.config.Logging

	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
		"fatal": true,
	}

	if !validLevels[logging.Level] {
		return fmt.Errorf("invalid log level: %s", logging.Level)
	}

	validFormats := map[string]bool{
		"json": true,
		"text": true,
	}

	if !validFormats[logging.Format] {
		return fmt.Errorf("invalid log format: %s", logging.Format)
	}

	validOutputs := map[string]bool{
		"stdout": true,
		"stderr": true,
		"file":   true,
	}

	if !validOutputs[logging.Output] {
		return fmt.Errorf("invalid log output: %s", logging.Output)
	}

	if logging.Output == "file" && logging.File == "" {
		return fmt.Errorf("log file path required when output is file")
	}

	return nil
}

// validateStorageConfig validates storage configuration
func (cm *ConfigManager) validateStorageConfig() error {
	storage := &cm.config.Storage

	validTypes := map[string]bool{
		"memory":   true,
		"file":     true,
		"database": true,
	}

	if !validTypes[storage.Type] {
		return fmt.Errorf("invalid storage type: %s", storage.Type)
	}

	if storage.Type == "file" && storage.DataDir == "" {
		return fmt.Errorf("data directory required for file storage")
	}

	if storage.Type == "database" {
		db := &storage.Database
		if db.Driver == "" {
			return fmt.Errorf("database driver required")
		}
		if db.Host == "" {
			return fmt.Errorf("database host required")
		}
		if db.Database == "" {
			return fmt.Errorf("database name required")
		}
	}

	return nil
}

// validateMetricsConfig validates metrics configuration
func (cm *ConfigManager) validateMetricsConfig() error {
	metrics := &cm.config.Metrics

	if metrics.Enabled {
		if metrics.Port <= 0 || metrics.Port > 65535 {
			return fmt.Errorf("invalid metrics port: %d", metrics.Port)
		}
		if metrics.Path == "" {
			return fmt.Errorf("metrics path cannot be empty")
		}
	}

	return nil
}

// getDefaultConfig returns default configuration
func (cm *ConfigManager) getDefaultConfig() *smpp.Config {
	return &smpp.Config{
		Server: smpp.ServerConfig{
			Host:               "localhost",
			Port:               2775,
			MaxConnections:     100,
			ReadTimeout:        30 * time.Second,
			WriteTimeout:       10 * time.Second,
			IdleTimeout:        300 * time.Second,
			EnquireLinkTimeout: 60 * time.Second,
			BindTimeout:        30 * time.Second,
			TLSEnabled:         false,
			TLSCertFile:        "",
			TLSKeyFile:         "",
		},
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
			TLSEnabled:           false,
			TLSSkipVerify:        false,
		},
		Logging: smpp.LoggingConfig{
			Level:      "info",
			Format:     "text",
			Output:     "stdout",
			File:       "",
			MaxSize:    100,
			MaxBackups: 3,
			MaxAge:     28,
			Compress:   true,
		},
		Storage: smpp.StorageConfig{
			Type:    "memory",
			DataDir: "./data",
			Database: smpp.DatabaseConfig{
				Driver:   "postgres",
				Host:     "localhost",
				Port:     5432,
				Username: "smpp",
				Password: "smpp",
				Database: "smpp",
				SSLMode:  "disable",
			},
		},
		Metrics: smpp.MetricsConfig{
			Enabled:   false,
			Port:      9090,
			Path:      "/metrics",
			Namespace: "smpp",
			Subsystem: "server",
		},
	}
}

// fileExists checks if a file exists
func (cm *ConfigManager) fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

// GetConfig returns the current configuration
func (cm *ConfigManager) GetConfig() *smpp.Config {
	return cm.config
}

// CreateDefaultConfigFile creates a default configuration file
func CreateDefaultConfigFile(path string) error {
	cm := NewConfigManager("")
	config := cm.getDefaultConfig()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal configuration
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
