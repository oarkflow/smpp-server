package logger

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/oarkflow/smpp-server/pkg/smpp"
)

// Level represents logging level
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// DefaultLogger implements the smpp.Logger interface
type DefaultLogger struct {
	level  Level
	fields map[string]interface{}
	logger *log.Logger
}

// NewDefaultLogger creates a new logger
func NewDefaultLogger(level string) smpp.Logger {
	var logLevel Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = LevelDebug
	case "info":
		logLevel = LevelInfo
	case "warn", "warning":
		logLevel = LevelWarn
	case "error":
		logLevel = LevelError
	case "fatal":
		logLevel = LevelFatal
	default:
		logLevel = LevelInfo
	}

	return &DefaultLogger{
		level:  logLevel,
		fields: make(map[string]interface{}),
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
}

// Debug logs a debug message
func (l *DefaultLogger) Debug(msg string, fields ...interface{}) {
	if l.level <= LevelDebug {
		l.logWithFields("DEBUG", msg, fields...)
	}
}

// Info logs an info message
func (l *DefaultLogger) Info(msg string, fields ...interface{}) {
	if l.level <= LevelInfo {
		l.logWithFields("INFO", msg, fields...)
	}
}

// Warn logs a warning message
func (l *DefaultLogger) Warn(msg string, fields ...interface{}) {
	if l.level <= LevelWarn {
		l.logWithFields("WARN", msg, fields...)
	}
}

// Error logs an error message
func (l *DefaultLogger) Error(msg string, fields ...interface{}) {
	if l.level <= LevelError {
		l.logWithFields("ERROR", msg, fields...)
	}
}

// Fatal logs a fatal message and exits
func (l *DefaultLogger) Fatal(msg string, fields ...interface{}) {
	l.logWithFields("FATAL", msg, fields...)
	os.Exit(1)
}

// WithFields returns a logger with additional fields
func (l *DefaultLogger) WithFields(fields map[string]interface{}) smpp.Logger {
	newFields := make(map[string]interface{})
	// Copy existing fields
	for k, v := range l.fields {
		newFields[k] = v
	}
	// Add new fields
	for k, v := range fields {
		newFields[k] = v
	}

	return &DefaultLogger{
		level:  l.level,
		fields: newFields,
		logger: l.logger,
	}
}

// logWithFields logs a message with fields
func (l *DefaultLogger) logWithFields(level string, msg string, fields ...interface{}) {
	// Build the log message
	var parts []string
	parts = append(parts, fmt.Sprintf("[%s]", level))
	parts = append(parts, msg)

	// Add structured fields
	if len(l.fields) > 0 {
		for k, v := range l.fields {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}

	// Add variadic fields (key-value pairs)
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key := fmt.Sprintf("%v", fields[i])
			value := fmt.Sprintf("%v", fields[i+1])
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		} else {
			// Odd number of fields, just add the last one
			parts = append(parts, fmt.Sprintf("%v", fields[i]))
		}
	}

	l.logger.Println(strings.Join(parts, " "))
}
