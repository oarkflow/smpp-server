package flowcontrol

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// WindowConfig defines the configuration for flow control windows
type WindowConfig struct {
	MaxOutstanding int           // Maximum number of outstanding requests
	WindowSize     time.Duration // Size of the sliding window
	MaxRetries     int           // Maximum number of retries
	RetryDelay     time.Duration // Delay between retries
}

// DefaultWindowConfig returns a default window configuration
func DefaultWindowConfig() WindowConfig {
	return WindowConfig{
		MaxOutstanding: 100,
		WindowSize:     time.Minute,
		MaxRetries:     3,
		RetryDelay:     time.Second,
	}
}

// SlidingWindow implements sliding window flow control
type SlidingWindow struct {
	config      WindowConfig
	outstanding int64       // Current outstanding requests
	window      []time.Time // Request timestamps in current window
	mu          sync.Mutex
}

// NewSlidingWindow creates a new sliding window flow controller
func NewSlidingWindow(config WindowConfig) *SlidingWindow {
	return &SlidingWindow{
		config: config,
		window: make([]time.Time, 0),
	}
}

// Acquire attempts to acquire a slot in the window
func (sw *SlidingWindow) Acquire(ctx context.Context) error {
	for retries := 0; retries < sw.config.MaxRetries; retries++ {
		if sw.tryAcquire() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sw.config.RetryDelay):
			// Continue retrying
		}
	}

	return ErrWindowFull
}

// tryAcquire attempts to acquire without blocking
func (sw *SlidingWindow) tryAcquire() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-sw.config.WindowSize)

	// Clean old entries
	validWindow := make([]time.Time, 0)
	for _, t := range sw.window {
		if t.After(windowStart) {
			validWindow = append(validWindow, t)
		}
	}
	sw.window = validWindow

	// Check if we can accept more requests
	if len(sw.window) < sw.config.MaxOutstanding {
		sw.window = append(sw.window, now)
		atomic.AddInt64(&sw.outstanding, 1)
		return true
	}

	return false
}

// Release releases a slot from the window
func (sw *SlidingWindow) Release() {
	atomic.AddInt64(&sw.outstanding, -1)
}

// Outstanding returns the current number of outstanding requests
func (sw *SlidingWindow) Outstanding() int64 {
	return atomic.LoadInt64(&sw.outstanding)
}

// WindowSize returns the current window size
func (sw *SlidingWindow) WindowSize() int {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return len(sw.window)
}

// FlowController manages flow control for multiple sessions
type FlowController struct {
	windows map[string]*SlidingWindow
	config  WindowConfig
	mu      sync.RWMutex
}

// NewFlowController creates a new flow controller
func NewFlowController(config WindowConfig) *FlowController {
	return &FlowController{
		windows: make(map[string]*SlidingWindow),
		config:  config,
	}
}

// Acquire acquires a flow control slot for a session
func (fc *FlowController) Acquire(ctx context.Context, sessionID string) error {
	fc.mu.Lock()
	window, exists := fc.windows[sessionID]
	if !exists {
		window = NewSlidingWindow(fc.config)
		fc.windows[sessionID] = window
	}
	fc.mu.Unlock()

	return window.Acquire(ctx)
}

// Release releases a flow control slot for a session
func (fc *FlowController) Release(sessionID string) {
	fc.mu.RLock()
	window, exists := fc.windows[sessionID]
	fc.mu.RUnlock()

	if exists {
		window.Release()
	}
}

// GetOutstanding returns outstanding requests for a session
func (fc *FlowController) GetOutstanding(sessionID string) int64 {
	fc.mu.RLock()
	window, exists := fc.windows[sessionID]
	fc.mu.RUnlock()

	if exists {
		return window.Outstanding()
	}

	return 0
}

// Cleanup removes inactive windows
func (fc *FlowController) Cleanup(maxAge time.Duration) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	now := time.Now()
	for sessionID, window := range fc.windows {
		// If window has no outstanding requests and hasn't been used recently
		if window.Outstanding() == 0 && len(window.window) > 0 {
			lastActivity := window.window[len(window.window)-1]
			if now.Sub(lastActivity) > maxAge {
				delete(fc.windows, sessionID)
			}
		}
	}
}

// Errors
var (
	ErrWindowFull = &FlowControlError{Message: "flow control window is full"}
)

// FlowControlError represents a flow control error
type FlowControlError struct {
	Message string
}

func (e *FlowControlError) Error() string {
	return e.Message
}
