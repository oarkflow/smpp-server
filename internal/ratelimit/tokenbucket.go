package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	capacity   int64     // Maximum number of tokens
	tokens     int64     // Current number of tokens
	refillRate float64   // Tokens added per second
	lastRefill time.Time // Last time tokens were refilled
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket rate limiter
func NewTokenBucket(capacity int64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity, // Start with full capacity
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed and consumes a token if available
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}

// AllowN checks if n requests are allowed and consumes n tokens if available
func (tb *TokenBucket) AllowN(n int64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= n {
		tb.tokens -= n
		return true
	}

	return false
}

// refill adds tokens based on elapsed time
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	// Calculate tokens to add
	tokensToAdd := int64(float64(elapsed.Nanoseconds()) * tb.refillRate / float64(time.Second.Nanoseconds()))

	if tokensToAdd > 0 {
		tb.tokens += tokensToAdd
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		tb.lastRefill = now
	}
}

// Tokens returns the current number of tokens
func (tb *TokenBucket) Tokens() int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()
	return tb.tokens
}

// Capacity returns the bucket capacity
func (tb *TokenBucket) Capacity() int64 {
	return tb.capacity
}

// RateLimiter manages rate limiting for multiple users
type RateLimiter struct {
	buckets map[string]*TokenBucket
	mu      sync.RWMutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*TokenBucket),
	}
}

// Allow checks if a user is allowed to make a request
func (rl *RateLimiter) Allow(userID string, rateLimit int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.buckets[userID]
	if !exists {
		// Create new bucket: rateLimit tokens per minute
		refillRate := float64(rateLimit) / 60.0 // tokens per second
		bucket = NewTokenBucket(int64(rateLimit), refillRate)
		rl.buckets[userID] = bucket
	}

	return bucket.Allow()
}

// AllowN checks if a user is allowed to make n requests
func (rl *RateLimiter) AllowN(userID string, n int64, rateLimit int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.buckets[userID]
	if !exists {
		// Create new bucket: rateLimit tokens per minute
		refillRate := float64(rateLimit) / 60.0 // tokens per second
		bucket = NewTokenBucket(int64(rateLimit), refillRate)
		rl.buckets[userID] = bucket
	}

	return bucket.AllowN(n)
}

// GetRemainingTokens returns the remaining tokens for a user
func (rl *RateLimiter) GetRemainingTokens(userID string) int64 {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if bucket, exists := rl.buckets[userID]; exists {
		return bucket.Tokens()
	}

	return 0
}

// Cleanup removes inactive buckets (optional, for memory management)
func (rl *RateLimiter) Cleanup(maxAge time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for userID, bucket := range rl.buckets {
		// If bucket hasn't been used recently and is full, remove it
		if bucket.Tokens() == bucket.Capacity() &&
			now.Sub(bucket.lastRefill) > maxAge {
			delete(rl.buckets, userID)
		}
	}
}

// SlidingWindowLimiter implements a sliding window rate limiter
type SlidingWindowLimiter struct {
	windows map[string][]time.Time
	window  time.Duration
	limit   int
	mu      sync.Mutex
}

// NewSlidingWindowLimiter creates a new sliding window rate limiter
func NewSlidingWindowLimiter(window time.Duration, limit int) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		windows: make(map[string][]time.Time),
		window:  window,
		limit:   limit,
	}
}

// Allow checks if a request is allowed within the sliding window
func (swl *SlidingWindowLimiter) Allow(key string) bool {
	swl.mu.Lock()
	defer swl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-swl.window)

	// Get or create window for this key
	times, exists := swl.windows[key]
	if !exists {
		times = make([]time.Time, 0)
	}

	// Remove old entries outside the window
	validTimes := make([]time.Time, 0)
	for _, t := range times {
		if t.After(windowStart) {
			validTimes = append(validTimes, t)
		}
	}

	// Check if under limit
	if len(validTimes) < swl.limit {
		validTimes = append(validTimes, now)
		swl.windows[key] = validTimes
		return true
	}

	swl.windows[key] = validTimes
	return false
}

// GetRemainingRequests returns the remaining requests allowed in the current window
func (swl *SlidingWindowLimiter) GetRemainingRequests(key string) int {
	swl.mu.Lock()
	defer swl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-swl.window)

	times, exists := swl.windows[key]
	if !exists {
		return swl.limit
	}

	// Count valid entries
	validCount := 0
	for _, t := range times {
		if t.After(windowStart) {
			validCount++
		}
	}

	return swl.limit - validCount
}
