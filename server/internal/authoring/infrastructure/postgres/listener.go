package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/RimuruChan/Vertex/server/internal/authoring/infrastructure/postgres/internal/dbgen"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/jackc/pgx/v5/stdlib"
)

const BuildsChannel = "vertex_problem_builds"

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
	if err := dbgen.New(conn).ListenBuildJobs(ctx); err != nil {
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
