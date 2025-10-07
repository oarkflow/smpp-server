# Example Configuration Files

This directory contains example configuration files for the SMPP server and client.

## Configuration Files

### `server.json`
Example configuration for running an SMPP server.

### `client.json`
Example configuration for running an SMPP client.

### `production.json`
Example production configuration with TLS enabled and file-based storage.

### `development.json`
Example development configuration with debug logging and in-memory storage.

## Configuration Structure

The configuration is organized into sections:

- **Server**: Server-specific settings (host, port, timeouts, TLS)
- **Client**: Client-specific settings (connection details, authentication)
- **Logging**: Logging configuration (level, format, output)
- **Storage**: Storage configuration (type, directories, database)
- **Metrics**: Metrics configuration (enabled, port, path)

## Usage

1. Copy an example configuration file to your desired location
2. Modify the settings as needed
3. Pass the configuration file path to your application

```go
import "github.com/oarkflow/smpp-server/internal/config"

configManager := config.NewConfigManager("config.json")
cfg, err := configManager.LoadConfig()
if err != nil {
    log.Fatal("Failed to load config:", err)
}

// Use the configuration
serverCfg := configManager.GetServerConfig()
clientCfg := configManager.GetClientConfig()
```

## Environment Variables

Configuration values can be overridden using environment variables:

- `SMPP_SERVER_HOST` - Server host
- `SMPP_SERVER_PORT` - Server port
- `SMPP_CLIENT_SYSTEM_ID` - Client system ID
- `SMPP_CLIENT_PASSWORD` - Client password
- `SMPP_LOG_LEVEL` - Log level
- `SMPP_STORAGE_TYPE` - Storage type
- `SMPP_DATA_DIR` - Data directory
