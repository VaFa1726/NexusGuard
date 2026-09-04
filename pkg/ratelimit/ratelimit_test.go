package ratelimit

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestLimiter_Allow(t *testing.T) {
	limiter := NewLimiter(rate.Limit(10), 20) // 10 req/sec, burst 20

	userID := int64(12345)

	// First 20 requests should pass (burst)
	for i := 0; i < 20; i++ {
		if !limiter.Allow(userID) {
			t.Errorf("Request %d should be allowed (within burst)", i)
		}
	}

	// 21st request should be rate limited
	if limiter.Allow(userID) {
		t.Error("Request 21 should be rate limited")
	}

	// Wait for rate limit to reset
	time.Sleep(200 * time.Millisecond)

	// Should allow 2 more requests (10 req/sec * 0.2s = 2)
	if !limiter.Allow(userID) {
		t.Error("Request after rate limit reset should be allowed")
	}
}

func TestLimiter_MultipleUsers(t *testing.T) {
	limiter := NewLimiter(rate.Limit(5), 10)

	user1 := int64(111)
	user2 := int64(222)

	// Both users should have independent limits
	for i := 0; i < 10; i++ {
		if !limiter.Allow(user1) {
			t.Errorf("User 1 request %d should be allowed", i)
		}
		if !limiter.Allow(user2) {
			t.Errorf("User 2 request %d should be allowed", i)
		}
	}

	// Both should be rate limited now
	if limiter.Allow(user1) {
		t.Error("User 1 should be rate limited")
	}
	if limiter.Allow(user2) {
		t.Error("User 2 should be rate limited")
	}
}

func TestLimiter_GetStats(t *testing.T) {
	limiter := NewLimiter(rate.Limit(10), 20)

	// No users yet
	if count := limiter.GetStats(); count != 0 {
		t.Errorf("Expected 0 limiters, got %d", count)
	}

	// Add 3 users
	limiter.Allow(123)
	limiter.Allow(456)
	limiter.Allow(789)

	if count := limiter.GetStats(); count != 3 {
		t.Errorf("Expected 3 limiters, got %d", count)
	}
}

func BenchmarkLimiter_Allow(b *testing.B) {
	limiter := NewLimiter(rate.Limit(1000), 2000)
	userID := int64(12345)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(userID)
	}
}

func BenchmarkLimiter_MultipleUsers(b *testing.B) {
	limiter := NewLimiter(rate.Limit(1000), 2000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		userID := int64(i % 1000) // 1000 different users
		limiter.Allow(userID)
	}
}
