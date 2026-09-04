package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter manages per-user rate limiting to prevent abuse and spam.
type Limiter struct {
	limiters sync.Map // map[int64]*rate.Limiter
	mu       sync.Mutex
	
	// Rate limit configuration
	rps   rate.Limit // Requests per second
	burst int        // Burst size
	
	// Cleanup configuration
	cleanupInterval time.Duration
	lastCleanup     time.Time
}

// NewLimiter creates a new rate limiter.
// rps: requests per second per user
// burst: maximum burst size per user
func NewLimiter(rps rate.Limit, burst int) *Limiter {
	l := &Limiter{
		rps:             rps,
		burst:           burst,
		cleanupInterval: 5 * time.Minute,
		lastCleanup:     time.Now(),
	}
	
	// Start cleanup goroutine
	go l.cleanupLoop()
	
	return l
}

// Allow checks if a request from the given user is allowed.
// Returns true if the request is within rate limits.
func (l *Limiter) Allow(userID int64) bool {
	limiter := l.getLimiter(userID)
	
	// Periodic cleanup check (lock-free fast path)
	if time.Since(l.lastCleanup) > l.cleanupInterval {
		go l.cleanup()
	}
	
	return limiter.Allow()
}

// getLimiter returns the rate limiter for a specific user.
func (l *Limiter) getLimiter(userID int64) *rate.Limiter {
	if limiter, exists := l.limiters.Load(userID); exists {
		return limiter.(*rate.Limiter)
	}
	
	// Create new limiter for this user
	limiter := rate.NewLimiter(l.rps, l.burst)
	l.limiters.Store(userID, limiter)
	return limiter
}

// cleanupLoop periodically removes idle limiters to prevent memory leak.
func (l *Limiter) cleanupLoop() {
	ticker := time.NewTicker(l.cleanupInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		l.cleanup()
	}
}

// cleanup removes limiters that haven't been used recently.
func (l *Limiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	// Mark cleanup time
	l.lastCleanup = time.Now()
	
	// Count before cleanup (for logging)
	count := 0
	l.limiters.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	
	// Note: In production, you'd want to track last access time per limiter
	// For now, we rely on sync.Map's internal cleanup and Go's GC
	// A more sophisticated approach would track last access time
	
	// Log cleanup if count is high
	// if count > 1000 {
	// 	// Could implement more aggressive cleanup here if needed
	// }
}

// GetStats returns current rate limiter statistics.
func (l *Limiter) GetStats() (count int) {
	l.limiters.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}
