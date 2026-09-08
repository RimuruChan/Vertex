package application

import (
	"context"
	"time"
)

// Dispatcher wakes one long-poll waiter per queued build. The database stays
// authoritative; the periodic fallback covers notifications lost to a
// reconnect, exactly as the judge queue does.
type Dispatcher struct{ wake chan struct{} }

func NewDispatcher(capacity int) *Dispatcher {
	if capacity < 1 {
		capacity = 1
	}
	return &Dispatcher{wake: make(chan struct{}, capacity)}
}

func (d *Dispatcher) Notify() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// RunFallback emits one bounded wake-up per interval for the whole instance.
func (d *Dispatcher) RunFallback(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.Notify()
		}
	}
}
