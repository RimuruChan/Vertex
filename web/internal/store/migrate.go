package store

import (
	"errors"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations 应用 web/migrations 下的版本化 SQL。
// 迁移目录由 MIGRATIONS_DIR 指定,默认从当前工作目录的 web/migrations 读取。
func RunMigrations(databaseURL string) error {
	dir := os.Getenv("MIGRATIONS_DIR")
	if dir == "" {
		dir = "web/migrations"
	}
	sourceURL := "file://" + dir

	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil {
		// ErrNoChange 表示已是最新版本,正常情况
		if errors.Is(err, migrate.ErrNoChange) {
			return nil
		}
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
