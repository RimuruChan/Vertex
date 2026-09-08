// Package ratelimit provides bounded, in-process abuse controls for HTTP
// entry points. It is deliberately transport-agnostic so identity and contest
// do not depend on one another.
package ratelimit

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

type entry struct {
	count   int
	resetAt time.Time
}

// Limiter is a fixed-window counter with a hard key-cap. Expired keys are
// removed lazily; a new key is rejected while the live key space is full.
// Failing closed keeps random-key churn from evicting a blocked account.
type Limiter struct {
	mu      sync.Mutex
	maxKeys int
	entries map[string]entry
	now     func() time.Time
}

func New(maxKeys int) *Limiter {
	if maxKeys < 1 {
		maxKeys = 1
	}
	return &Limiter{maxKeys: maxKeys, entries: make(map[string]entry, maxKeys), now: time.Now}
}

// Allow consumes one attempt for key under the supplied policy.
func (l *Limiter) Allow(key string, limit int, window time.Duration) bool {
	if l == nil || limit < 1 || window <= 0 {
		return true
	}
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if current, ok := l.entries[key]; ok {
		if !now.Before(current.resetAt) {
			current = entry{resetAt: now.Add(window)}
		}
		if current.count >= limit {
			l.entries[key] = current
			return false
		}
		current.count++
		l.entries[key] = current
		return true
	}

	if len(l.entries) >= l.maxKeys {
		// Sweeping only at capacity keeps the normal new-key path O(1). An
		// attacker rotating identities must not turn every request into a scan
		// over the complete live key set.
		l.removeExpired(now)
		if len(l.entries) >= l.maxKeys {
			return false
		}
	}
	l.entries[key] = entry{count: 1, resetAt: now.Add(window)}
	return true
}

func (l *Limiter) removeExpired(now time.Time) {
	for key, current := range l.entries {
		if !now.Before(current.resetAt) {
			delete(l.entries, key)
		}
	}
}

// Policy binds one endpoint's count and window to the shared bounded limiter.
// A zero Policy disables limiting, which keeps isolated handler tests simple.
type Policy struct {
	Limiter *Limiter
	Limit   int
	Window  time.Duration
}

func (p Policy) Allow(key string) bool {
	return p.Limiter == nil || p.Limiter.Allow(key, p.Limit, p.Window)
}

// Key fixes the stored key length even when an unauthenticated field contains
// a very long random value. Scope is included in the digest so endpoint
// policies remain isolated while sharing one capacity budget.
func Key(scope, identity string) string {
	digest := sha256.Sum256([]byte(scope + "\x00" + identity))
	return hex.EncodeToString(digest[:])
}

// ClientIdentity uses only the TCP peer recorded in RemoteAddr. It never
// trusts X-Forwarded-For: deployments that need the original address behind a
// proxy must enforce client limiting at that trusted ingress.
func ClientIdentity(request *http.Request) string {
	if request == nil {
		return "unknown"
	}
	raw := strings.TrimSpace(request.RemoteAddr)
	if address, err := netip.ParseAddrPort(raw); err == nil {
		return address.Addr().Unmap().String()
	}
	if address, err := netip.ParseAddr(raw); err == nil {
		return address.Unmap().String()
	}
	return "unknown"
}
