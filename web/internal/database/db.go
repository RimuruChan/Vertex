package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// DB wraps the sqlx connection pool shared by domain stores.
type DB struct {
	Pool *sqlx.DB
}

// NewDB creates a PostgreSQL connection pool for the supplied URL.
func NewDB(ctx context.Context, url string) (*DB, error) {
	db, err := sqlx.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open DATABASE_URL: %w", err)
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)

	// Confirm the connection before returning it to the composition root.
	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctxPing); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	slog.Info("connected to postgres")
	return &DB{Pool: db}, nil
}

// Close 关闭连接池。
func (db *DB) Close() {
	db.Pool.Close()
}
