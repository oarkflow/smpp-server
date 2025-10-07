package errorrecovery

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

// RetryConfig defines the configuration for retry logic
type RetryConfig struct {
	MaxRetries      int           // Maximum number of retry attempts
	InitialDelay    time.Duration // Initial delay before first retry
	MaxDelay        time.Duration // Maximum delay between retries
	BackoffFactor   float64       // Exponential backoff factor
	JitterFactor    float64       // Random jitter factor (0.0 to 1.0)
	RetryableErrors []error       // List of errors that should be retried
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0.1,
		RetryableErrors: []error{
			errors.New("connection refused"),
			errors.New("connection reset"),
			errors.New("timeout"),
			errors.New("temporary failure"),
		},
	}
}

// IsRetryableError checks if an error should trigger a retry
func (c *RetryConfig) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	for _, retryableErr := range c.RetryableErrors {
		if errors.Is(err, retryableErr) || fmt.Sprintf("%v", retryableErr) == errStr {
			return true
		}
	}

	return false
}

// RetryableFunc represents a function that can be retried
type RetryableFunc func() error

// RetryResult contains the result of a retry operation
type RetryResult struct {
	Attempts int
	Duration time.Duration
	Error    error
}

// Retry executes a function with retry logic
func Retry(ctx context.Context, config RetryConfig, fn RetryableFunc) RetryResult {
	start := time.Now()
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return RetryResult{
				Attempts: attempt,
				Duration: time.Since(start),
				Error:    ctx.Err(),
			}
		default:
		}

		// Execute the function
		err := fn()
		if err == nil {
			return RetryResult{
				Attempts: attempt + 1,
				Duration: time.Since(start),
				Error:    nil,
			}
		}

		lastErr = err

		// If this is the last attempt or error is not retryable, don't retry
		if attempt == config.MaxRetries || !config.IsRetryableError(err) {
			break
		}

		// Calculate delay for next attempt
		delay := calculateDelay(config, attempt)

		// Wait before retrying
		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return RetryResult{
				Attempts: attempt + 1,
				Duration: time.Since(start),
				Error:    ctx.Err(),
			}
		}
	}

	return RetryResult{
		Attempts: config.MaxRetries + 1,
		Duration: time.Since(start),
		Error:    lastErr,
	}
}

// calculateDelay calculates the delay for the next retry attempt
func calculateDelay(config RetryConfig, attempt int) time.Duration {
	// Exponential backoff: initial_delay * (backoff_factor ^ attempt)
	delay := float64(config.InitialDelay) * math.Pow(config.BackoffFactor, float64(attempt))

	// Cap at max delay
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	// Add jitter to prevent thundering herd
	if config.JitterFactor > 0 {
		jitterRange := delay * config.JitterFactor
		jitter := (rand.Float64() - 0.5) * 2 * jitterRange
		delay += jitter
	}

	return time.Duration(delay)
}

// CircuitBreakerConfig defines the configuration for circuit breaker
type CircuitBreakerConfig struct {
	FailureThreshold int           // Number of failures before opening circuit
	ResetTimeout     time.Duration // Time to wait before trying to close circuit
	SuccessThreshold int           // Number of successes needed to close circuit
}

// DefaultCircuitBreakerConfig returns a default circuit breaker configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		ResetTimeout:     60 * time.Second,
		SuccessThreshold: 2,
	}
}

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int

const (
	StateClosed CircuitBreakerState = iota
	StateOpen
	StateHalfOpen
)

// CircuitBreaker implements circuit breaker pattern
type CircuitBreaker struct {
	config      CircuitBreakerConfig
	state       CircuitBreakerState
	failures    int
	successes   int
	lastFailure time.Time
	lastSuccess time.Time
	mu          sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn RetryableFunc) error {
	cb.mu.RLock()
	state := cb.state
	cb.mu.RUnlock()

	switch state {
	case StateOpen:
		if time.Since(cb.lastFailure) < cb.config.ResetTimeout {
			return errors.New("circuit breaker is open")
		}
		// Try to go to half-open state
		cb.mu.Lock()
		if cb.state == StateOpen {
			cb.state = StateHalfOpen
			cb.successes = 0
		}
		cb.mu.Unlock()
		fallthrough

	case StateHalfOpen:
		err := fn()
		cb.mu.Lock()
		if err != nil {
			cb.failures++
			cb.lastFailure = time.Now()
			cb.state = StateOpen
			cb.mu.Unlock()
			return err
		}

		cb.successes++
		cb.lastSuccess = time.Now()
		if cb.successes >= cb.config.SuccessThreshold {
			cb.state = StateClosed
			cb.failures = 0
		}
		cb.mu.Unlock()
		return nil

	case StateClosed:
		err := fn()
		if err != nil {
			cb.mu.Lock()
			cb.failures++
			cb.lastFailure = time.Now()
			if cb.failures >= cb.config.FailureThreshold {
				cb.state = StateOpen
			}
			cb.mu.Unlock()
		} else {
			cb.mu.Lock()
			cb.failures = 0
			cb.mu.Unlock()
		}
		return err

	default:
		return errors.New("unknown circuit breaker state")
	}
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}
