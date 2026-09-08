package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/RimuruChan/Vertex/server/internal/modules/judge/infrastructure/postgres/internal/dbgen"

	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	"github.com/jackc/pgx/v5/stdlib"
)

const judgeJobsChannel = "vertex_judge_jobs"

// ListenJobs reserves one database connection and forwards committed job
// notifications. Callers reconnect on errors; correctness still comes from the
// periodic claim fallback because PostgreSQL notifications are best-effort.
func ListenJobs(ctx context.Context, db *database.DB, notify func()) error {
	if db == nil {
		return errors.New("judge notification database is required")
	}
	if notify == nil {
		return errors.New("judge job notification callback is required")
	}
	conn, err := db.Pool.DB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve judge notification connection: %w", err)
	}
	defer conn.Close()
	if err := dbgen.New(conn).ListenJudgeJobs(ctx); err != nil {
		return fmt.Errorf("listen for judge jobs: %w", err)
	}
	return conn.Raw(func(driverConn any) error {
		pgxConn, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("judge notification connection uses unexpected driver %T", driverConn)
		}
		for {
			if _, err := pgxConn.Conn().WaitForNotification(ctx); err != nil {
				return err
			}
			notify()
		}
	})
}
