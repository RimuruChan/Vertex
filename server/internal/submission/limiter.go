package submission

import (
	"sync"
	"time"
)

// SlidingWindowLimiter bounds submission bursts per user within one Web process.
// PostgreSQL remains the source of truth for judge jobs; this limiter is only a
// local abuse-control policy and correctness does not depend on it.
type SlidingWindowLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	maxHits int
	hits    map[string][]time.Time
	now     func() time.Time
}

func NewSlidingWindowLimiter(window time.Duration, maxHits int) *SlidingWindowLimiter {
	if window <= 0 {
		window = time.Minute
	}
	if maxHits <= 0 {
		maxHits = 10
	}
	return &SlidingWindowLimiter{
		window: window, maxHits: maxHits, hits: make(map[string][]time.Time), now: time.Now,
	}
}

func (r *SlidingWindowLimiter) Allow(key string) bool {
	if key == "" {
		return true
	}
	now := r.now()
	cutoff := now.Add(-r.window)

	r.mu.Lock()
	defer r.mu.Unlock()
	recent := r.hits[key][:0]
	for _, hit := range r.hits[key] {
		if hit.After(cutoff) {
			recent = append(recent, hit)
		}
	}
	if len(recent) >= r.maxHits {
		r.hits[key] = recent
		return false
	}
	r.hits[key] = append(recent, now)
	return true
}
