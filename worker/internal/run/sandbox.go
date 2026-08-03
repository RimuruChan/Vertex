package run

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

// Limits describes one sandboxed process resource budget.
type Limits struct {
	TimeMs      int
	WallTimeMs  int
	MemoryKB    int
	Processes   int
	OutputBytes int64
	StackKB     int
}

// Config describes one vertex-sandbox execution.
type Config struct {
	// StdinPath is a workspace-relative input file name.
	StdinPath string
	// CPU and wall-clock limits, in seconds. They are converted to integral
	// milliseconds for the native runner.
	TimeLimitSec float64
	WallLimitSec float64
	MemLimitKB   int
	Processes    int
	OutputBytes  int64
	StackKB      int
	// Env is an explicit allowlist. The native runner always starts from an
	// empty environment and rejects dynamic-loader variables.
	Env []string
}

const (
	DefaultTimeOvershootMs = 1000
	DefaultWorkspaceBytes  = int64(64 * 1024 * 1024)
	DefaultWorkspaceInodes = int64(4096)
)

// Policy contains worker-wide sandbox enforcement settings that are not part
// of an individual problem's resource limits.
type Policy struct {
	TimeOvershootMs int
	WorkspaceBytes  int64
	WorkspaceInodes int64
	CPUSet          string
}

func DefaultPolicy() Policy {
	return Policy{
		TimeOvershootMs: DefaultTimeOvershootMs,
		WorkspaceBytes:  DefaultWorkspaceBytes,
		WorkspaceInodes: DefaultWorkspaceInodes,
	}
}

func (p Policy) validate() error {
	if p.TimeOvershootMs < 0 {
		return fmt.Errorf("time overshoot must be non-negative")
	}
	if p.WorkspaceBytes <= 0 {
		return fmt.Errorf("workspace byte limit must be positive")
	}
	if p.WorkspaceInodes <= 0 {
		return fmt.Errorf("workspace inode limit must be positive")
	}
	if p.CPUSet != strings.TrimSpace(p.CPUSet) {
		return fmt.Errorf("CPU set must not have leading or trailing whitespace")
	}
	return nil
}

type nativeLimits struct {
	timeMs       int64
	timeHardMs   int64
	wallMs       int64
	wallHardMs   int64
	workspaceB   int64
	workspaceIDs int64
}

func calculateNativeLimits(cfg *Config, policy Policy) (nativeLimits, error) {
	if err := policy.validate(); err != nil {
		return nativeLimits{}, err
	}
	timeMs, err := secondsToMilliseconds(cfg.TimeLimitSec)
	if err != nil {
		return nativeLimits{}, fmt.Errorf("CPU limit: %w", err)
	}
	wallMs, err := secondsToMilliseconds(cfg.WallLimitSec)
	if err != nil {
		return nativeLimits{}, fmt.Errorf("wall limit: %w", err)
	}
	timeHardMs, err := addMilliseconds(timeMs, policy.TimeOvershootMs)
	if err != nil {
		return nativeLimits{}, fmt.Errorf("hard CPU limit: %w", err)
	}
	wallHardMs, err := addMilliseconds(wallMs, policy.TimeOvershootMs)
	if err != nil {
		return nativeLimits{}, fmt.Errorf("hard wall limit: %w", err)
	}
	return nativeLimits{
		timeMs:       timeMs,
		timeHardMs:   timeHardMs,
		wallMs:       wallMs,
		wallHardMs:   wallHardMs,
		workspaceB:   policy.WorkspaceBytes,
		workspaceIDs: policy.WorkspaceInodes,
	}, nil
}

func secondsToMilliseconds(seconds float64) (int64, error) {
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, fmt.Errorf("must be a positive finite number")
	}
	milliseconds := seconds * 1000
	if milliseconds >= float64(math.MaxInt64) {
		return 0, fmt.Errorf("is too large")
	}
	result := int64(milliseconds)
	if result < 1 {
		result = 1
	}
	return result, nil
}

func addMilliseconds(soft int64, overshoot int) (int64, error) {
	if overshoot < 0 || int64(overshoot) > math.MaxInt64-soft {
		return 0, fmt.Errorf("is too large")
	}
	return soft + int64(overshoot), nil
}

type RunResult struct {
	Meta       *verdict.SandboxMeta
	StdoutPath string
	StderrPath string
	RunTimeMs  int
}

// Sandbox wraps the native C++ vertex-sandbox runner. Each instance owns one
// workspace and one cgroup slot; callers must not share it concurrently.
type Sandbox struct {
	BoxID   int
	BaseDir string
	Policy  Policy
}

func NewSandbox(boxID int, baseDir string) *Sandbox {
	if baseDir == "" {
		baseDir = "/var/local/lib/vertex-sandbox"
	}
	return &Sandbox{BoxID: boxID, BaseDir: baseDir, Policy: DefaultPolicy()}
}

func (s *Sandbox) BoxPath(name string) string {
	return filepath.Join(s.BoxDir(), "box", name)
}

func (s *Sandbox) BoxDir() string {
	return filepath.Join(s.BaseDir, itoa(s.BoxID))
}

func (s *Sandbox) controlPath(name string) string {
	return filepath.Join(s.BoxDir(), "control", name)
}

func (s *Sandbox) Init(ctx context.Context) error {
	if err := s.Policy.validate(); err != nil {
		return fmt.Errorf("sandbox policy: %w", err)
	}
	out, err := s.command(ctx, "init").CombinedOutput()
	if err != nil {
		return fmt.Errorf("vertex-sandbox init: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Sandbox) Cleanup(ctx context.Context) error {
	out, err := s.command(ctx, "cleanup").CombinedOutput()
	if err != nil {
		return fmt.Errorf("vertex-sandbox cleanup: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Reset is deliberately detached from a submission context: cancellation of
// the judged process must not prevent cgroup cleanup and workspace renewal.
func (s *Sandbox) Reset() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Cleanup(ctx); err != nil {
		return err
	}
	return s.Init(ctx)
}

// CopyIn copies trusted worker inputs into the sandbox workspace. Removing the
// destination first ensures that a stale user-created symlink is never
// followed by the privileged worker.
func (s *Sandbox) CopyIn(ctx context.Context, data map[string]string) error {
	for boxName, hostPath := range data {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := validateBoxName(boxName); err != nil {
			return err
		}
		src, err := os.Open(hostPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", hostPath, err)
		}
		dstPath := s.BoxPath(boxName)
		if err := os.Remove(dstPath); err != nil && !os.IsNotExist(err) {
			src.Close()
			return fmt.Errorf("remove stale %s: %w", dstPath, err)
		}
		dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			src.Close()
			return fmt.Errorf("create %s: %w", dstPath, err)
		}
		_, copyErr := io.Copy(dst, src)
		srcErr := src.Close()
		dstErr := dst.Close()
		if copyErr != nil {
			return fmt.Errorf("copy %s -> %s: %w", hostPath, dstPath, copyErr)
		}
		if srcErr != nil {
			return srcErr
		}
		if dstErr != nil {
			return dstErr
		}
	}
	return nil
}

func (s *Sandbox) Run(ctx context.Context, cfg *Config, cmdArgs ...string) (*RunResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("sandbox config is required")
	}
	if len(cmdArgs) == 0 {
		return nil, fmt.Errorf("sandbox command is required")
	}
	limits, err := calculateNativeLimits(cfg, s.Policy)
	if err != nil {
		return nil, fmt.Errorf("sandbox limits: %w", err)
	}
	stdinName := strings.TrimPrefix(cfg.StdinPath, "/box/")
	if stdinName != "" {
		if err := validateBoxName(stdinName); err != nil {
			return nil, err
		}
	}

	args := []string{
		"run",
		"--box-id", itoa(s.BoxID),
		"--base", s.BaseDir,
		"--time-ms", fmt.Sprintf("%d", limits.timeMs),
		"--time-hard-ms", fmt.Sprintf("%d", limits.timeHardMs),
		"--wall-ms", fmt.Sprintf("%d", limits.wallMs),
		"--wall-hard-ms", fmt.Sprintf("%d", limits.wallHardMs),
		"--memory-kb", itoa(cfg.MemLimitKB),
		"--processes", itoa(cfg.Processes),
		"--output-bytes", fmt.Sprintf("%d", cfg.OutputBytes),
		"--workspace-bytes", fmt.Sprintf("%d", limits.workspaceB),
		"--workspace-inodes", fmt.Sprintf("%d", limits.workspaceIDs),
	}
	if s.Policy.CPUSet != "" {
		args = append(args, "--cpu-set", s.Policy.CPUSet)
	}
	if cfg.StackKB > 0 {
		args = append(args, "--stack-kb", itoa(cfg.StackKB))
	}
	if stdinName != "" {
		args = append(args, "--stdin", stdinName)
	}
	for _, entry := range cfg.Env {
		args = append(args, "--env", entry)
	}
	args = append(args, "--")
	args = append(args, cmdArgs...)

	cmd := exec.CommandContext(ctx, "vertex-sandbox", args...)
	cmd.Env = runnerEnvironment()
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	start := time.Now()
	err = cmd.Run()
	elapsed := time.Since(start).Milliseconds()

	metaPath := s.MetaFilePath()
	if err != nil {
		if _, statErr := os.Stat(metaPath); statErr == nil {
			if meta, parseErr := verdict.ParseSandboxMeta(metaPath); parseErr == nil {
				return &RunResult{
					Meta: meta, StdoutPath: s.StdoutPath(), StderrPath: s.StderrPath(),
					RunTimeMs: int(elapsed),
				}, nil
			}
		}
		return nil, fmt.Errorf("vertex-sandbox run: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	meta, parseErr := verdict.ParseSandboxMeta(metaPath)
	if parseErr != nil {
		meta = &verdict.SandboxMeta{Status: "XX", Message: "sandbox meta file missing"}
	}
	return &RunResult{
		Meta: meta, StdoutPath: s.StdoutPath(), StderrPath: s.StderrPath(),
		RunTimeMs: int(elapsed),
	}, nil
}

func (s *Sandbox) MetaFilePath() string { return s.controlPath("meta") }
func (s *Sandbox) StdoutPath() string   { return s.controlPath("stdout") }
func (s *Sandbox) StderrPath() string   { return s.controlPath("stderr") }

func (s *Sandbox) command(ctx context.Context, action string) *exec.Cmd {
	args := []string{action, "--box-id", itoa(s.BoxID), "--base", s.BaseDir}
	if s.Policy.CPUSet != "" {
		args = append(args, "--cpu-set", s.Policy.CPUSet)
	}
	cmd := exec.CommandContext(ctx, "vertex-sandbox", args...)
	cmd.Env = runnerEnvironment()
	return cmd
}

func runnerEnvironment() []string {
	env := []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C"}
	if root := os.Getenv("VERTEX_CGROUP_ROOT"); root != "" {
		env = append(env, "VERTEX_CGROUP_ROOT="+root)
	}
	return env
}

func validateBoxName(name string) error {
	if name == "" {
		return fmt.Errorf("sandbox file name is required")
	}
	// Keep privileged copy-in targets to one path component. Besides making the
	// contract platform-independent, this prevents a submission from planting a
	// symlinked parent directory that the worker could later follow.
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid sandbox file name %q", name)
	}
	return nil
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
