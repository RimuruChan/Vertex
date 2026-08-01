package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// migrationsDir 解析迁移目录:
//  1. MIGRATIONS_DIR 环境变量优先;
//  2. 当前目录下有 migrations/ 则用它(CI 与本地 `cd web` 运行);
//  3. 否则用 web/migrations(从仓库根运行)。
func migrationsDir() string {
	if env := os.Getenv("MIGRATIONS_DIR"); env != "" {
		return env
	}
	if _, err := os.Stat(filepath.Join("migrations", "000001_init.up.sql")); err == nil {
		return "migrations"
	}
	if _, err := os.Stat(filepath.Join("web", "migrations", "000001_init.up.sql")); err == nil {
		return filepath.Join("web", "migrations")
	}
	return "migrations"
}

// RunMigrations 应用版本化 SQL 迁移。
func RunMigrations(databaseURL string) error {
	sourceURL := "file://" + migrationsDir()

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
