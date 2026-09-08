package run

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Limits describes the complete resource budget of one isolated process tree.
// Hard time limits are optional; when omitted, the worker policy overshoot is
// added to the corresponding soft limit.
type Limits struct {
	CPUTime         time.Duration
	CPUHardTime     time.Duration
	WallTime        time.Duration
	WallHardTime    time.Duration
	MemoryKB        int
	Processes       int
	OutputBytes     int64
	StackKB         int
	WorkspaceBytes  int64
	WorkspaceInodes int64
}

// Execution is a verdict-neutral request to run one command and all of its
// descendants inside a sandbox. Task roles such as solution, interactor or
// generator belong to the trusted orchestration layer, not this contract.
type Execution struct {
	// Optional caller-owned destinations. Internal sandbox paths never escape.
	StdoutPath string
	StderrPath string
	Stream     bool
	Command    []string
	// StdinFile is a single workspace-relative input file name.
	StdinFile string
	// Environment is an explicit allowlist. The native runner always starts from an
	// empty environment and rejects dynamic-loader variables.
	Environment []string
	Limits      Limits
}

const (
	DefaultTimeOvershootMs = 1000
	DefaultWorkspaceBytes  = int64(64 * 1024 * 1024)
	DefaultWorkspaceInodes = int64(4096)
)

// Policy contains worker-wide defaults and ceilings. An execution may request
// a smaller workspace budget but cannot exceed these node limits.
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
	if p.WorkspaceBytes <= 0 || p.WorkspaceBytes == math.MaxInt64 {
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

func calculateNativeLimits(limits Limits, policy Policy) (nativeLimits, error) {
	if err := policy.validate(); err != nil {
		return nativeLimits{}, err
	}
	if limits.MemoryKB <= 0 {
		return nativeLimits{}, fmt.Errorf("memory limit must be positive")
	}
	if limits.Processes <= 0 {
		return nativeLimits{}, fmt.Errorf("process limit must be positive")
	}
	if limits.OutputBytes <= 0 {
		return nativeLimits{}, fmt.Errorf("output limit must be positive")
	}
	if limits.StackKB < 0 {
		return nativeLimits{}, fmt.Errorf("stack limit must be non-negative")
	}
	if limits.CPUHardTime < 0 || limits.WallHardTime < 0 {
		return nativeLimits{}, fmt.Errorf("hard time limits must be non-negative")
	}
	timeMs, err := durationToMilliseconds(limits.CPUTime)
	if err != nil {
		return nativeLimits{}, fmt.Errorf("CPU limit: %w", err)
	}
	wallMs, err := durationToMilliseconds(limits.WallTime)
	if err != nil {
		return nativeLimits{}, fmt.Errorf("wall limit: %w", err)
	}
	timeHardMs := int64(0)
	if limits.CPUHardTime > 0 {
		timeHardMs, err = durationToMilliseconds(limits.CPUHardTime)
	} else {
		timeHardMs, err = addMilliseconds(timeMs, policy.TimeOvershootMs)
	}
	if err != nil {
		return nativeLimits{}, fmt.Errorf("hard CPU limit: %w", err)
	}
	wallHardMs := int64(0)
	if limits.WallHardTime > 0 {
		wallHardMs, err = durationToMilliseconds(limits.WallHardTime)
	} else {
		wallHardMs, err = addMilliseconds(wallMs, policy.TimeOvershootMs)
	}
	if err != nil {
		return nativeLimits{}, fmt.Errorf("hard wall limit: %w", err)
	}
	if timeHardMs < timeMs || wallHardMs < wallMs {
		return nativeLimits{}, fmt.Errorf("hard time limits must not be below soft limits")
	}
	workspaceBytes := limits.WorkspaceBytes
	if workspaceBytes == 0 {
		workspaceBytes = policy.WorkspaceBytes
	}
	if workspaceBytes < 0 || workspaceBytes > policy.WorkspaceBytes {
		return nativeLimits{}, fmt.Errorf("workspace byte limit must be positive and at most %d", policy.WorkspaceBytes)
	}
	workspaceInodes := limits.WorkspaceInodes
	if workspaceInodes == 0 {
		workspaceInodes = policy.WorkspaceInodes
	}
	if workspaceInodes < 0 || workspaceInodes > policy.WorkspaceInodes {
		return nativeLimits{}, fmt.Errorf("workspace inode limit must be positive and at most %d", policy.WorkspaceInodes)
	}
	return nativeLimits{
		timeMs:       timeMs,
		timeHardMs:   timeHardMs,
		wallMs:       wallMs,
		wallHardMs:   wallHardMs,
		workspaceB:   workspaceBytes,
		workspaceIDs: workspaceInodes,
	}, nil
}

func durationToMilliseconds(duration time.Duration) (int64, error) {
	if duration <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	milliseconds := duration / time.Millisecond
	if duration%time.Millisecond != 0 {
		milliseconds++
	}
	return int64(milliseconds), nil
}

func addMilliseconds(soft int64, overshoot int) (int64, error) {
	if overshoot < 0 || int64(overshoot) > math.MaxInt64-soft {
		return 0, fmt.Errorf("is too large")
	}
	return soft + int64(overshoot), nil
}

type Result struct {
	Stdout     string
	Stderr     string
	Meta       *Meta
	StdoutPath string
	StderrPath string
	RunTimeMs  int
}

// box is the private native protocol layout. Application code only sees
// Environment and Process handles.
type box struct {
	BoxID   int
	BaseDir string
	Policy  Policy
}

func newBox(boxID int, baseDir string) *box {
	if baseDir == "" {
		baseDir = "/vertex/sandbox"
	}
	return &box{BoxID: boxID, BaseDir: baseDir, Policy: DefaultPolicy()}
}

func (s *box) BoxPath(name string) string {
	return filepath.Join(s.BoxDir(), "box", name)
}

func (s *box) BoxDir() string {
	return filepath.Join(s.BaseDir, itoa(s.BoxID))
}

func (s *box) controlPath(name string) string {
	return filepath.Join(s.BoxDir(), "control", name)
}

// CopyIn copies trusted worker inputs into the sandbox workspace. Removing the
// destination first ensures that a stale user-created symlink is never
// followed by the privileged worker.
func (s *box) CopyIn(ctx context.Context, data map[string]string) error {
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
		written, copyErr := io.Copy(dst, io.LimitReader(src, s.Policy.WorkspaceBytes+1))
		if copyErr == nil && written > s.Policy.WorkspaceBytes {
			copyErr = fmt.Errorf("input exceeds workspace limit")
		}
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

// CopyOut copies one regular file from a completed sandbox execution into a
// trusted destination. The one-component source name and identity check keep
// symlinks or path replacement from turning privileged artifact collection
// into an arbitrary file read. The destination must not already exist.
func (s *box) CopyOut(ctx context.Context, boxName, destination string, maxBytes int64) error {
	if err := validateBoxName(boxName); err != nil {
		return err
	}
	if destination == "" {
		return fmt.Errorf("artifact destination is required")
	}
	if maxBytes <= 0 || maxBytes == math.MaxInt64 {
		return fmt.Errorf("artifact byte limit must be between 1 and %d", int64(math.MaxInt64-1))
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return copyArtifact(ctx, s.BoxPath(boxName), destination, maxBytes)
}

func copyArtifact(ctx context.Context, source, destination string, maxBytes int64) error {
	if destination == "" || maxBytes <= 0 || maxBytes == math.MaxInt64 {
		return fmt.Errorf("invalid artifact destination or limit")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	boxName := filepath.Base(source)
	linkInfo, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect sandbox artifact %s: %w", boxName, err)
	}
	if !linkInfo.Mode().IsRegular() {
		return fmt.Errorf("sandbox artifact %s is not a regular file", boxName)
	}
	src, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open sandbox artifact %s: %w", boxName, err)
	}
	defer src.Close()
	openInfo, err := src.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened sandbox artifact %s: %w", boxName, err)
	}
	if !openInfo.Mode().IsRegular() || !os.SameFile(linkInfo, openInfo) {
		return fmt.Errorf("sandbox artifact %s changed while opening", boxName)
	}
	if openInfo.Size() > maxBytes {
		return fmt.Errorf("sandbox artifact %s exceeds %d bytes", boxName, maxBytes)
	}
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("artifact destination already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect artifact destination: %w", err)
	}

	directory := filepath.Dir(destination)
	tmp, err := os.CreateTemp(directory, ".vertex-artifact-*")
	if err != nil {
		return fmt.Errorf("create artifact temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	written, copyErr := io.Copy(tmp, io.LimitReader(src, maxBytes+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		return fmt.Errorf("copy sandbox artifact %s: %w", boxName, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close sandbox artifact destination: %w", closeErr)
	}
	if written > maxBytes {
		return fmt.Errorf("sandbox artifact %s exceeds %d bytes", boxName, maxBytes)
	}
	// Linking within the destination directory publishes atomically and fails
	// when another collector created the destination after the Lstat check.
	if err := os.Link(tmpPath, destination); err != nil {
		return fmt.Errorf("publish sandbox artifact: %w", err)
	}
	return nil
}

func (s *box) executionArgs(execution Execution, stdinFD, stdoutFD int) ([]string, error) {
	if len(execution.Command) == 0 {
		return nil, fmt.Errorf("sandbox command is required")
	}
	limits, err := calculateNativeLimits(execution.Limits, s.Policy)
	if err != nil {
		return nil, fmt.Errorf("sandbox limits: %w", err)
	}
	stdinName := strings.TrimPrefix(execution.StdinFile, "/box/")
	if stdinName != "" {
		if err := validateBoxName(stdinName); err != nil {
			return nil, err
		}
	}
	if stdinName != "" && stdinFD >= 0 {
		return nil, fmt.Errorf("streaming stdin and stdin file are mutually exclusive")
	}

	args := []string{
		"run",
		"--box-id", itoa(s.BoxID),
		"--base", s.BaseDir,
		"--time-ms", fmt.Sprintf("%d", limits.timeMs),
		"--time-hard-ms", fmt.Sprintf("%d", limits.timeHardMs),
		"--wall-ms", fmt.Sprintf("%d", limits.wallMs),
		"--wall-hard-ms", fmt.Sprintf("%d", limits.wallHardMs),
		"--memory-kb", itoa(execution.Limits.MemoryKB),
		"--processes", itoa(execution.Limits.Processes),
		"--output-bytes", fmt.Sprintf("%d", execution.Limits.OutputBytes),
		"--workspace-bytes", fmt.Sprintf("%d", limits.workspaceB),
		"--workspace-inodes", fmt.Sprintf("%d", limits.workspaceIDs),
	}
	if s.Policy.CPUSet != "" {
		args = append(args, "--cpu-set", s.Policy.CPUSet)
	}
	if execution.Limits.StackKB > 0 {
		args = append(args, "--stack-kb", itoa(execution.Limits.StackKB))
	}
	if stdinName != "" {
		args = append(args, "--stdin", stdinName)
	}
	if stdinFD >= 0 {
		args = append(args, "--stdin-fd", itoa(stdinFD))
	}
	if stdoutFD >= 0 {
		args = append(args, "--stdout-fd", itoa(stdoutFD))
	}
	for _, entry := range execution.Environment {
		args = append(args, "--env", entry)
	}
	args = append(args, "--")
	args = append(args, execution.Command...)
	return args, nil
}

func configureCancellation(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(os.Interrupt)
	}
}

func (s *box) resultAfterWait(runErr error, stderr string, elapsed int64) (*Result, error) {
	metaPath := s.MetaFilePath()
	if runErr != nil {
		if _, statErr := os.Stat(metaPath); statErr == nil {
			if meta, parseErr := ParseMeta(metaPath); parseErr == nil {
				return &Result{
					Meta: meta, StdoutPath: s.StdoutPath(), StderrPath: s.StderrPath(),
					RunTimeMs: int(elapsed),
				}, nil
			}
		}
		return nil, fmt.Errorf("vertex-sandbox run: %w: %s", runErr, strings.TrimSpace(stderr))
	}

	meta, parseErr := ParseMeta(metaPath)
	if parseErr != nil {
		meta = &Meta{Status: "XX", TerminationReason: TerminationSetupError, Message: "sandbox meta file missing"}
	}
	return &Result{
		Meta: meta, StdoutPath: s.StdoutPath(), StderrPath: s.StderrPath(),
		RunTimeMs: int(elapsed),
	}, nil
}

func (s *box) MetaFilePath() string { return s.controlPath("meta") }
func (s *box) StdoutPath() string   { return s.controlPath("stdout") }
func (s *box) StderrPath() string   { return s.controlPath("stderr") }

func runnerEnvironment() []string {
	return []string{"PATH=/vertex:/usr/local/bin:/usr/bin:/bin", "LANG=C"}
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
