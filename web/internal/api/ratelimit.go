package api

import (
	"sync"
	"time"
)

// rateLimiter 简单的滑动窗口限流器(进程内,按 key 计数)。
// 生产环境多实例时应换 Redis 计数(此处满足单实例 MVP)。
type rateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	maxHits int
	hits    map[string][]time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		window:  time.Minute,
		maxHits: 10,
		hits:    make(map[string][]time.Time),
	}
}

// Allow 判断 key 在窗口内是否还有配额。
func (r *rateLimiter) Allow(key string) bool {
	if key == "" {
		return true // 未认证不适用
	}
	now := time.Now()
	cutoff := now.Add(-r.window)

	r.mu.Lock()
	defer r.mu.Unlock()

	// 清理过期记录
	recent := r.hits[key][:0]
	for _, t := range r.hits[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= r.maxHits {
		r.hits[key] = recent
		return false
	}
	r.hits[key] = append(recent, now)
	return true
}
