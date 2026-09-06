package authoring

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/jackc/pgx/v5/stdlib"
)

// BuildsChannel carries committed build notifications between server replicas.
const BuildsChannel = "vertex_problem_builds"

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

// ListenBuilds reserves one connection and forwards committed build
// notifications until the context ends or the connection drops.
func ListenBuilds(ctx context.Context, db *database.DB, notify func()) error {
	if db == nil {
		return errors.New("build notification database is required")
	}
	if notify == nil {
		return errors.New("build notification callback is required")
	}
	conn, err := db.Pool.Connx(ctx)
	if err != nil {
		return fmt.Errorf("reserve build notification connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "LISTEN "+BuildsChannel); err != nil {
		return fmt.Errorf("listen for problem builds: %w", err)
	}
	return conn.Raw(func(driverConn any) error {
		pgxConn, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("build notification connection uses unexpected driver %T", driverConn)
		}
		for {
			if _, err := pgxConn.Conn().WaitForNotification(ctx); err != nil {
				return err
			}
			notify()
		}
	})
}
