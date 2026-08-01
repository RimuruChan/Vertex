package store

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/vertex-oj/web/internal/auth"
)

// BootstrapAdmin 在启动时根据环境变量创建默认管理员。
// 设 ADMIN_USERNAME / ADMIN_PASSWORD(以及可选 ADMIN_EMAIL)后首次启动会创建 admin 用户。
func BootstrapAdmin(ctx context.Context, db *DB) {
	username := os.Getenv("ADMIN_USERNAME")
	if username == "" {
		return
	}
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		slog.Warn("ADMIN_USERNAME set but ADMIN_PASSWORD empty; skipping admin bootstrap")
		return
	}
	email := os.Getenv("ADMIN_EMAIL")
	if email == "" {
		email = "admin@vertex.local"
	}

	users := NewUserStore(db)
	existing, err := users.ByUsername(ctx, username)
	if err == nil && existing != nil {
		// 已存在:若当前是普通用户且提供了密码,则提升为 admin 的幂等操作不做;
		// 仅记录,避免误覆盖。
		slog.Info("admin bootstrap: user already exists, skipping", "username", username)
		return
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		slog.Warn("admin bootstrap lookup failed", "error", err)
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		slog.Warn("admin bootstrap: hash failed", "error", err)
		return
	}

	// 直接插入 admin 用户
	_, err = db.Pool.Exec(ctx,
		`INSERT INTO users (username, email, password_hash, role)
		 VALUES ($1, $2, $3, 'admin')
		 ON CONFLICT (username) DO NOTHING`,
		username, email, hash,
	)
	if err != nil {
		slog.Warn("admin bootstrap insert failed", "error", err)
		return
	}
	slog.Info("admin user created", "username", username)
}
