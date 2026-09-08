package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/run"
)

// Config contains all process-level worker settings. Keeping parsing here lets
// the command package remain an explicit composition root.
type Config struct {
	Workers         int
	TestdataRoot    string
	ScratchRoot     string
	CacheRoot       string
	SandboxBase     string
	SandboxPolicy   run.Policy
	JudgeAPIURL     string
	JudgeAPIToken   string
	JudgeWorkerID   string
	LongPollTimeout time.Duration
	HTTPTimeout     time.Duration
	// BuildsEnabled turns the package build loop on. Operators can dedicate
	// nodes to judging or to authoring builds by flipping it.
	BuildsEnabled     bool
	TestlibPath       string
	BuildProgressTick time.Duration
}

func Load() (Config, error) {
	return Parse(os.LookupEnv)
}

func Parse(lookup func(string) (string, bool)) (Config, error) {
	get := func(key, fallback string) string {
		if value, ok := lookup(key); ok && value != "" {
			return value
		}
		return fallback
	}

	root := get("VERTEX_ROOT", "/vertex")
	cfg := Config{
		TestdataRoot:  get("TESTDATA_ROOT", filepath.Join(root, "testdata")),
		ScratchRoot:   get("SCRATCH_ROOT", filepath.Join(root, "scratch")),
		CacheRoot:     get("CACHE_ROOT", filepath.Join(root, "cache")),
		SandboxBase:   get("SANDBOX_BASE", filepath.Join(root, "sandbox")),
		JudgeAPIURL:   get("JUDGE_API_URL", "http://server:8080/internal/judge/v1"),
		JudgeAPIToken: get("JUDGE_API_TOKEN", ""),
		JudgeWorkerID: get("JUDGE_WORKER_ID", defaultWorkerID()),
		SandboxPolicy: run.DefaultPolicy(),
		TestlibPath:   get("TESTLIB_PATH", "/vertex/testlib.h"),
	}
	var err error
	if cfg.Workers, err = integer(lookup, "JUDGE_WORKERS", 2, 1, 4096); err != nil {
		return Config{}, err
	}
	if cfg.BuildsEnabled, err = boolean(lookup, "BUILD_WORKER_ENABLED", true); err != nil {
		return Config{}, err
	}
	if cfg.LongPollTimeout, err = duration(lookup, "JUDGE_LONG_POLL_TIMEOUT", 25*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.LongPollTimeout%time.Second != 0 {
		return Config{}, fmt.Errorf("JUDGE_LONG_POLL_TIMEOUT must be a whole number of seconds")
	}
	if cfg.HTTPTimeout, err = duration(lookup, "JUDGE_HTTP_TIMEOUT", 40*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.HTTPTimeout <= cfg.LongPollTimeout {
		return Config{}, fmt.Errorf("JUDGE_HTTP_TIMEOUT must be greater than JUDGE_LONG_POLL_TIMEOUT")
	}
	if err := validateURL(cfg.JudgeAPIURL); err != nil {
		return Config{}, err
	}
	if len(strings.TrimSpace(cfg.JudgeAPIToken)) < 32 {
		return Config{}, fmt.Errorf("JUDGE_API_TOKEN must contain at least 32 characters")
	}
	if cfg.JudgeWorkerID != strings.TrimSpace(cfg.JudgeWorkerID) || len(cfg.JudgeWorkerID) > 128 {
		return Config{}, fmt.Errorf("JUDGE_WORKER_ID must contain 1-128 characters without surrounding whitespace")
	}
	if cfg.BuildProgressTick, err = duration(lookup, "BUILD_PROGRESS_INTERVAL", 15*time.Second); err != nil {
		return Config{}, err
	}
	if err := parseSandboxPolicy(lookup, &cfg.SandboxPolicy); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func integer(lookup func(string) (string, bool), key string, fallback, minimum, maximum int) (int, error) {
	raw, ok := lookup(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, minimum, maximum)
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

func validateURL(raw string) error {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("JUDGE_API_URL must be an absolute HTTP(S) URL")
	}
	return nil
}

func parseSandboxPolicy(lookup func(string) (string, bool), policy *run.Policy) error {
	if raw, ok := lookup("SANDBOX_TIME_OVERSHOOT_MS"); ok && raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return fmt.Errorf("SANDBOX_TIME_OVERSHOOT_MS must be a non-negative integer")
		}
		policy.TimeOvershootMs = value
	}
	if raw, ok := lookup("SANDBOX_WORKSPACE_BYTES"); ok && raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			return fmt.Errorf("SANDBOX_WORKSPACE_BYTES must be a positive integer")
		}
		policy.WorkspaceBytes = value
	}
	if raw, ok := lookup("SANDBOX_WORKSPACE_INODES"); ok && raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			return fmt.Errorf("SANDBOX_WORKSPACE_INODES must be a positive integer")
		}
		policy.WorkspaceInodes = value
	}
	if raw, ok := lookup("SANDBOX_CPUSET"); ok && raw != "" {
		value := strings.TrimSpace(raw)
		if value == "" || value != raw {
			return fmt.Errorf("SANDBOX_CPUSET must not contain surrounding whitespace")
		}
		policy.CPUSet = value
	}
	return nil
}

func defaultWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "worker"
	}
	return fmt.Sprintf("%s/%d", hostname, os.Getpid())
}
