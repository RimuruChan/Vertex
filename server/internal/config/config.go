package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains process-level settings. Parsing is centralized so invalid
// security and timeout combinations fail before the HTTP server starts.
type Config struct {
	Environment          string
	DatabaseURL          string
	Port                 string
	JWTSecret            string
	AccessTokenTTL       time.Duration
	RefreshTokenTTL      time.Duration
	AuthCookieSecure     bool
	AuthCookieDomain     string
	CORSAllowedOrigins   []string
	JudgeAPIToken        string
	JudgeLongPollTimeout time.Duration
	JudgeLeaseTTL        time.Duration
	TestdataRoot         string
	MigrationsDir        string
	SwaggerEnabled       bool
	AdminUsername        string
	AdminPassword        string
	AdminEmail           string
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return Parse(os.LookupEnv)
}

// Parse builds a Config through an injectable lookup function for tests.
func Parse(lookup func(string) (string, bool)) (Config, error) {
	get := func(key, fallback string) string {
		if value, ok := lookup(key); ok && value != "" {
			return value
		}
		return fallback
	}

	cfg := Config{
		Environment:        get("APP_ENV", "development"),
		DatabaseURL:        get("DATABASE_URL", ""),
		Port:               get("PORT", "8080"),
		JWTSecret:          get("JWT_SECRET", ""),
		AuthCookieDomain:   get("AUTH_COOKIE_DOMAIN", ""),
		CORSAllowedOrigins: splitCSV(get("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		JudgeAPIToken:      get("JUDGE_API_TOKEN", ""),
		TestdataRoot:       get("TESTDATA_ROOT", "./testdata"),
		MigrationsDir:      get("MIGRATIONS_DIR", ""),
		AdminUsername:      get("ADMIN_USERNAME", ""),
		AdminPassword:      get("ADMIN_PASSWORD", ""),
		AdminEmail:         get("ADMIN_EMAIL", "admin@vertex.local"),
	}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return Config{}, fmt.Errorf("DATABASE_URL must be explicitly configured")
	}
	if len(strings.TrimSpace(cfg.JWTSecret)) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must contain at least 32 characters")
	}
	if len(strings.TrimSpace(cfg.JudgeAPIToken)) < 32 {
		return Config{}, fmt.Errorf("JUDGE_API_TOKEN must contain at least 32 characters")
	}

	var err error
	if cfg.AccessTokenTTL, err = duration(lookup, "AUTH_ACCESS_TTL", 15*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.RefreshTokenTTL, err = duration(lookup, "AUTH_REFRESH_TTL", 30*24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.JudgeLongPollTimeout, err = duration(lookup, "JUDGE_LONG_POLL_TIMEOUT", 25*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.JudgeLeaseTTL, err = duration(lookup, "JUDGE_LEASE_TTL", 45*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.AuthCookieSecure, err = boolean(lookup, "AUTH_COOKIE_SECURE", cfg.Environment == "production"); err != nil {
		return Config{}, err
	}
	if cfg.SwaggerEnabled, err = boolean(lookup, "SWAGGER_ENABLED", cfg.Environment != "production"); err != nil {
		return Config{}, err
	}

	port, portErr := strconv.Atoi(cfg.Port)
	if portErr != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("PORT must be an integer between 1 and 65535")
	}
	if len(cfg.CORSAllowedOrigins) == 0 {
		return Config{}, fmt.Errorf("CORS_ALLOWED_ORIGINS must contain at least one origin")
	}
	if cfg.JudgeLeaseTTL <= cfg.JudgeLongPollTimeout {
		return Config{}, fmt.Errorf("JUDGE_LEASE_TTL must be greater than JUDGE_LONG_POLL_TIMEOUT")
	}
	if (cfg.AdminUsername == "") != (cfg.AdminPassword == "") {
		return Config{}, fmt.Errorf("ADMIN_USERNAME and ADMIN_PASSWORD must be configured together")
	}
	if cfg.Environment == "production" {
		if !cfg.AuthCookieSecure {
			return Config{}, fmt.Errorf("AUTH_COOKIE_SECURE must be true in production")
		}
	}

	return cfg, nil
}

func duration(lookup func(string) (string, bool), key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := lookup(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return value, nil
}

func boolean(lookup func(string) (string, bool), key string, fallback bool) (bool, error) {
	raw, ok := lookup(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return value, nil
}

func splitCSV(raw string) []string {
	values := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}
