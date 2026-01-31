package ratelimit

import (
	"sync"
	"time"
)

// tokenBucket represents a token bucket rate limiter.
type tokenBucket struct {
	rate       float64    // tokens per second
	capacity   float64    // max tokens
	tokens     float64    // current tokens
	lastRefill time.Time  // last refill time
	mu         sync.Mutex // lock
}

// newTokenBucket creates a new token bucket.
func newTokenBucket(rate, capacity float64) *tokenBucket {
	return &tokenBucket{
		rate:       rate,
		capacity:   capacity,
		tokens:     capacity, // start full
		lastRefill: time.Now(),
	}
}

// allow checks if one token can be consumed.
func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	// Refill tokens
	tokensToAdd := elapsed * tb.rate
	tb.tokens = tb.tokens + tokensToAdd
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
	tb.lastRefill = now

	// Consume token
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}

	return false
}

// Manager manages multiple token buckets (e.g. per IP).
type Manager struct {
	buckets    map[string]*tokenBucket
	mu         sync.RWMutex
	rate       float64
	capacity   float64
	expiration time.Duration // remove bucket if unused for this duration
}

// NewManager creates a new rate limiter manager.
// rate: tokens per second.
// capacity: max burst.
// expiration: how long to keep an idle bucket in memory.
func NewManager(rate, capacity float64, expiration time.Duration) *Manager {
	m := &Manager{
		buckets:    make(map[string]*tokenBucket),
		rate:       rate,
		capacity:   capacity,
		expiration: expiration,
	}

	// Start cleanup loop
	go m.cleanupLoop()
	return m
}

// Allow checks if the key is allowed.
func (m *Manager) Allow(key string) bool {
	m.mu.RLock()
	bucket, exists := m.buckets[key]
	m.mu.RUnlock()

	if !exists {
		m.mu.Lock()
		// Double check
		bucket, exists = m.buckets[key]
		if !exists {
			bucket = newTokenBucket(m.rate, m.capacity)
			m.buckets[key] = bucket
		}
		m.mu.Unlock()
	}

	return bucket.allow()
}

func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(m.expiration)
	for range ticker.C {
		m.cleanup()
	}
}

func (m *Manager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, bucket := range m.buckets {
		bucket.mu.Lock()
		// If unused for expiration time, delete it.
		// Approximation: lastRefill is updated on every allow() check.
		if now.Sub(bucket.lastRefill) > m.expiration {
			delete(m.buckets, key)
		}
		bucket.mu.Unlock()
	}
}
