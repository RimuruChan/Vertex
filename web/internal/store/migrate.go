package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // 注册 pgx5 驱动(配合 postgres:// URL)
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

// migrationsURL 把标准 postgres:// URL 转成 golang-migrate 认识的 pgx5://。
// pgx5 驱动注册名为 "pgx5"(见 database/pgx/v5 源码),但实际连接会换回 postgres scheme。
func migrationsURL(databaseURL string) string {
	if len(databaseURL) >= 11 && databaseURL[:11] == "postgres://" {
		return "pgx5://" + databaseURL[11:]
	}
	if len(databaseURL) >= 15 && databaseURL[:15] == "postgresql://" {
		return "pgx5://" + databaseURL[15:]
	}
	return databaseURL
}

// RunMigrations 应用版本化 SQL 迁移。
func RunMigrations(databaseURL string) error {
	sourceURL := "file://" + migrationsDir()

	m, err := migrate.New(sourceURL, migrationsURL(databaseURL))
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
