package memory

import (
	"testing"
	"time"
)

func TestQuotaIsolationAndWindowExpiry(t *testing.T) {
	now := time.Unix(1800000000, 0)
	limiter := NewSlidingWindowLimiter(time.Minute, 2)
	limiter.now = func() time.Time { return now }
	if !limiter.Allow("one") || !limiter.Allow("one") || limiter.Allow("one") {
		t.Fatal("per-user quota was not enforced")
	}
	if !limiter.Allow("two") {
		t.Fatal("another user's quota was consumed")
	}
	now = now.Add(time.Minute)
	if !limiter.Allow("one") {
		t.Fatal("expired hits still consume quota")
	}
}
