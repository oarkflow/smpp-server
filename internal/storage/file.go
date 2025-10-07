package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oarkflow/smpp-server/pkg/smpp"
)

// FileSMSStorage implements SMSStorage interface using file-based storage
type FileSMSStorage struct {
	mu       sync.RWMutex
	dataDir  string
	messages map[string]*smpp.Message
	logger   smpp.Logger
}

// NewFileSMSStorage creates a new file-based SMS storage
func NewFileSMSStorage(dataDir string, logger smpp.Logger) (*FileSMSStorage, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	storage := &FileSMSStorage{
		dataDir:  dataDir,
		messages: make(map[string]*smpp.Message),
		logger:   logger,
	}

	// Load existing messages
	if err := storage.loadMessages(); err != nil {
		return nil, fmt.Errorf("failed to load messages: %w", err)
	}

	return storage, nil
}

// StoreSMS stores an SMS message and returns the message ID
func (fs *FileSMSStorage) StoreSMS(ctx context.Context, message *smpp.Message) (string, error) {
	if message == nil {
		return "", fmt.Errorf("message cannot be nil")
	}

	if message.ID == "" {
		return "", fmt.Errorf("message ID cannot be empty")
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Create a copy to avoid external modifications
	messageCopy := *message
	messageCopy.SubmitTime = time.Now()

	fs.messages[message.ID] = &messageCopy

	// Persist to file
	if err := fs.saveMessage(&messageCopy); err != nil {
		return "", fmt.Errorf("failed to save message to file: %w", err)
	}

	if fs.logger != nil {
		fs.logger.Debug("SMS stored to file",
			"message_id", message.ID,
			"source", message.SourceAddr.Addr,
			"dest", message.DestAddr.Addr)
	}

	return message.ID, nil
}

// GetSMS retrieves an SMS message by ID
func (fs *FileSMSStorage) GetSMS(ctx context.Context, messageID string) (*smpp.Message, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message ID cannot be empty")
	}

	fs.mu.RLock()
	defer fs.mu.RUnlock()

	message, exists := fs.messages[messageID]
	if !exists {
		return nil, fmt.Errorf("message with ID %s not found", messageID)
	}

	// Return a copy to avoid external modifications
	messageCopy := *message
	return &messageCopy, nil
}

// UpdateSMSStatus updates the status of an SMS message
func (fs *FileSMSStorage) UpdateSMSStatus(ctx context.Context, messageID string, status smpp.MessageStatus) error {
	if messageID == "" {
		return fmt.Errorf("message ID cannot be empty")
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	message, exists := fs.messages[messageID]
	if !exists {
		return fmt.Errorf("message with ID %s not found", messageID)
	}

	message.Status = status

	// Update done time for final statuses
	if status == smpp.MessageStatusDelivered || status == smpp.MessageStatusExpired ||
		status == smpp.MessageStatusDeleted || status == smpp.MessageStatusUndeliverable ||
		status == smpp.MessageStatusRejected {
		now := time.Now()
		message.DoneTime = &now
		message.DoneDate = now.Format("0601021504")
	}

	// Persist changes to file
	if err := fs.saveMessage(message); err != nil {
		return fmt.Errorf("failed to save updated message to file: %w", err)
	}

	if fs.logger != nil {
		fs.logger.Debug("SMS status updated in file",
			"message_id", messageID,
			"status", status.String())
	}

	return nil
}

// SearchSMS searches for SMS messages based on criteria
func (fs *FileSMSStorage) SearchSMS(ctx context.Context, criteria *smpp.SMSSearchCriteria) ([]*smpp.Message, error) {
	if criteria == nil {
		return nil, fmt.Errorf("search criteria cannot be nil")
	}

	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var results []*smpp.Message

	for _, message := range fs.messages {
		if fs.matchesCriteria(message, criteria) {
			messageCopy := *message
			results = append(results, &messageCopy)
		}
	}

	// Apply sorting
	if criteria.SortBy != "" {
		switch criteria.SortBy {
		case "submit_time":
			if criteria.SortOrder == "desc" {
				for i := 0; i < len(results)-1; i++ {
					for j := i + 1; j < len(results); j++ {
						if results[i].SubmitTime.Before(results[j].SubmitTime) {
							results[i], results[j] = results[j], results[i]
						}
					}
				}
			} else {
				for i := 0; i < len(results)-1; i++ {
					for j := i + 1; j < len(results); j++ {
						if results[i].SubmitTime.After(results[j].SubmitTime) {
							results[i], results[j] = results[j], results[i]
						}
					}
				}
			}
		case "status":
			if criteria.SortOrder == "desc" {
				for i := 0; i < len(results)-1; i++ {
					for j := i + 1; j < len(results); j++ {
						if results[i].Status < results[j].Status {
							results[i], results[j] = results[j], results[i]
						}
					}
				}
			} else {
				for i := 0; i < len(results)-1; i++ {
					for j := i + 1; j < len(results); j++ {
						if results[i].Status > results[j].Status {
							results[i], results[j] = results[j], results[i]
						}
					}
				}
			}
		}
	}

	// Apply offset and limit
	start := 0
	if criteria.Offset > 0 {
		start = criteria.Offset
		if start >= len(results) {
			return []*smpp.Message{}, nil
		}
	}

	end := len(results)
	if criteria.Limit > 0 {
		requestedEnd := start + criteria.Limit
		if requestedEnd < end {
			end = requestedEnd
		}
	}

	return results[start:end], nil
}

// DeleteSMS deletes an SMS message
func (fs *FileSMSStorage) DeleteSMS(ctx context.Context, messageID string) error {
	if messageID == "" {
		return fmt.Errorf("message ID cannot be empty")
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, exists := fs.messages[messageID]; !exists {
		return fmt.Errorf("message with ID %s not found", messageID)
	}

	// Remove from memory
	delete(fs.messages, messageID)

	// Remove file
	filename := fs.getMessageFilename(messageID)
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove message file: %w", err)
	}

	if fs.logger != nil {
		fs.logger.Debug("SMS deleted from file", "message_id", messageID)
	}

	return nil
}

// GetSMSByDateRange retrieves SMS messages within a date range
func (fs *FileSMSStorage) GetSMSByDateRange(ctx context.Context, from, to time.Time) ([]*smpp.Message, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var results []*smpp.Message

	for _, message := range fs.messages {
		if (message.SubmitTime.Equal(from) || message.SubmitTime.After(from)) &&
			(message.SubmitTime.Equal(to) || message.SubmitTime.Before(to)) {
			messageCopy := *message
			results = append(results, &messageCopy)
		}
	}

	return results, nil
}

// GetSMSCount returns the total count of SMS messages
func (fs *FileSMSStorage) GetSMSCount(ctx context.Context) (int64, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	return int64(len(fs.messages)), nil
}

// loadMessages loads all messages from files
func (fs *FileSMSStorage) loadMessages() error {
	messagesDir := filepath.Join(fs.dataDir, "messages")
	if err := os.MkdirAll(messagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create messages directory: %w", err)
	}

	entries, err := os.ReadDir(messagesDir)
	if err != nil {
		return fmt.Errorf("failed to read messages directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filename := filepath.Join(messagesDir, entry.Name())
		message, err := fs.loadMessage(filename)
		if err != nil {
			if fs.logger != nil {
				fs.logger.Warn("Failed to load message file",
					"filename", filename,
					"error", err)
			}
			continue
		}

		fs.messages[message.ID] = message
	}

	if fs.logger != nil {
		fs.logger.Info("Loaded messages from files", "count", len(fs.messages))
	}

	return nil
}

// loadMessage loads a single message from file
func (fs *FileSMSStorage) loadMessage(filename string) (*smpp.Message, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var message smpp.Message
	if err := json.Unmarshal(data, &message); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return &message, nil
}

// saveMessage saves a message to file
func (fs *FileSMSStorage) saveMessage(message *smpp.Message) error {
	filename := fs.getMessageFilename(message.ID)

	// Ensure directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(message, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// getMessageFilename returns the filename for a message
func (fs *FileSMSStorage) getMessageFilename(messageID string) string {
	return filepath.Join(fs.dataDir, "messages", messageID+".json")
}

// matchesCriteria checks if a message matches the search criteria
func (fs *FileSMSStorage) matchesCriteria(message *smpp.Message, criteria *smpp.SMSSearchCriteria) bool {
	if criteria.SourceAddr != "" && message.SourceAddr.Addr != criteria.SourceAddr {
		return false
	}

	if criteria.DestAddr != "" && message.DestAddr.Addr != criteria.DestAddr {
		return false
	}

	if criteria.Status != nil && message.Status != *criteria.Status {
		return false
	}

	if criteria.FromDate != nil && message.SubmitTime.Before(*criteria.FromDate) {
		return false
	}

	if criteria.ToDate != nil && message.SubmitTime.After(*criteria.ToDate) {
		return false
	}

	return true
}

// FileReportStorage implements ReportStorage interface using file-based storage
type FileReportStorage struct {
	mu      sync.RWMutex
	dataDir string
	reports map[string]*smpp.DeliveryReport
	logger  smpp.Logger
}

// NewFileReportStorage creates a new file-based report storage
func NewFileReportStorage(dataDir string, logger smpp.Logger) (*FileReportStorage, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	storage := &FileReportStorage{
		dataDir: dataDir,
		reports: make(map[string]*smpp.DeliveryReport),
		logger:  logger,
	}

	// Load existing reports
	if err := storage.loadReports(); err != nil {
		return nil, fmt.Errorf("failed to load reports: %w", err)
	}

	return storage, nil
}

// StoreReport stores a delivery report
func (fr *FileReportStorage) StoreReport(ctx context.Context, report *smpp.DeliveryReport) error {
	if report == nil {
		return fmt.Errorf("report cannot be nil")
	}

	if report.MessageID == "" {
		return fmt.Errorf("report message ID cannot be empty")
	}

	fr.mu.Lock()
	defer fr.mu.Unlock()

	// Create a copy to avoid external modifications
	reportCopy := *report
	fr.reports[report.MessageID] = &reportCopy

	// Persist to file
	if err := fr.saveReport(&reportCopy); err != nil {
		return fmt.Errorf("failed to save report to file: %w", err)
	}

	if fr.logger != nil {
		fr.logger.Debug("Delivery report stored to file",
			"message_id", report.MessageID,
			"status", report.Status)
	}

	return nil
}

// GetReport retrieves a delivery report by message ID
func (fr *FileReportStorage) GetReport(ctx context.Context, messageID string) (*smpp.DeliveryReport, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message ID cannot be empty")
	}

	fr.mu.RLock()
	defer fr.mu.RUnlock()

	report, exists := fr.reports[messageID]
	if !exists {
		return nil, fmt.Errorf("report for message ID %s not found", messageID)
	}

	// Return a copy to avoid external modifications
	reportCopy := *report
	return &reportCopy, nil
}

// GetReportsByDateRange retrieves delivery reports within a date range
func (fr *FileReportStorage) GetReportsByDateRange(ctx context.Context, from, to time.Time) ([]*smpp.DeliveryReport, error) {
	fr.mu.RLock()
	defer fr.mu.RUnlock()

	var results []*smpp.DeliveryReport

	for _, report := range fr.reports {
		// Filter by timestamp if available
		if !report.Timestamp.IsZero() {
			if (report.Timestamp.Equal(from) || report.Timestamp.After(from)) &&
				(report.Timestamp.Equal(to) || report.Timestamp.Before(to)) {
				reportCopy := *report
				results = append(results, &reportCopy)
			}
		} else {
			// Fallback: include all reports if no timestamp
			reportCopy := *report
			results = append(results, &reportCopy)
		}
	}

	return results, nil
}

// SearchReports searches for delivery reports based on criteria
func (fr *FileReportStorage) SearchReports(ctx context.Context, criteria *smpp.ReportSearchCriteria) ([]*smpp.DeliveryReport, error) {
	if criteria == nil {
		return nil, fmt.Errorf("search criteria cannot be nil")
	}

	fr.mu.RLock()
	defer fr.mu.RUnlock()

	var results []*smpp.DeliveryReport

	for _, report := range fr.reports {
		if fr.matchesReportCriteria(report, criteria) {
			reportCopy := *report
			results = append(results, &reportCopy)
		}
	}

	return results, nil
}

// DeleteReport deletes a delivery report
func (fr *FileReportStorage) DeleteReport(ctx context.Context, messageID string) error {
	if messageID == "" {
		return fmt.Errorf("message ID cannot be empty")
	}

	fr.mu.Lock()
	defer fr.mu.Unlock()

	if _, exists := fr.reports[messageID]; !exists {
		return fmt.Errorf("report for message ID %s not found", messageID)
	}

	// Remove from memory
	delete(fr.reports, messageID)

	// Remove file
	filename := fr.getReportFilename(messageID)
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove report file: %w", err)
	}

	if fr.logger != nil {
		fr.logger.Debug("Delivery report deleted from file", "message_id", messageID)
	}

	return nil
}

// GetReportCount returns the total count of delivery reports
func (fr *FileReportStorage) GetReportCount(ctx context.Context) (int64, error) {
	fr.mu.RLock()
	defer fr.mu.RUnlock()

	return int64(len(fr.reports)), nil
}

// loadReports loads all reports from files
func (fr *FileReportStorage) loadReports() error {
	reportsDir := filepath.Join(fr.dataDir, "reports")
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		return fmt.Errorf("failed to create reports directory: %w", err)
	}

	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		return fmt.Errorf("failed to read reports directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filename := filepath.Join(reportsDir, entry.Name())
		report, err := fr.loadReport(filename)
		if err != nil {
			if fr.logger != nil {
				fr.logger.Warn("Failed to load report file",
					"filename", filename,
					"error", err)
			}
			continue
		}

		fr.reports[report.MessageID] = report
	}

	if fr.logger != nil {
		fr.logger.Info("Loaded reports from files", "count", len(fr.reports))
	}

	return nil
}

// loadReport loads a single report from file
func (fr *FileReportStorage) loadReport(filename string) (*smpp.DeliveryReport, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var report smpp.DeliveryReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("failed to unmarshal report: %w", err)
	}

	return &report, nil
}

// saveReport saves a report to file
func (fr *FileReportStorage) saveReport(report *smpp.DeliveryReport) error {
	filename := fr.getReportFilename(report.MessageID)

	// Ensure directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// getReportFilename returns the filename for a report
func (fr *FileReportStorage) getReportFilename(messageID string) string {
	return filepath.Join(fr.dataDir, "reports", messageID+".json")
}

// matchesReportCriteria checks if a report matches the search criteria
func (fr *FileReportStorage) matchesReportCriteria(report *smpp.DeliveryReport, criteria *smpp.ReportSearchCriteria) bool {
	if criteria.MessageID != "" && report.MessageID != criteria.MessageID {
		return false
	}

	if criteria.Status != "" && report.Status != criteria.Status {
		return false
	}

	return true
}
