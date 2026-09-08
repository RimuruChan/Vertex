package application

import (
	"context"
	"time"
)

// Dispatcher wakes one long-poll waiter for each newly queued job. Database
// state remains authoritative; the periodic fallback covers lost signals.
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

// RunFallback emits one bounded wake-up per interval for the whole Web
// instance. Waiters never run independent database polling loops.
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
