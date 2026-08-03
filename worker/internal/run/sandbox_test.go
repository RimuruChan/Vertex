package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validLimits() Limits {
	return Limits{
		CPUTime:     time.Second,
		WallTime:    2 * time.Second,
		MemoryKB:    262144,
		Processes:   8,
		OutputBytes: 32 * 1024 * 1024,
	}
}

func TestNewSandboxUsesSafeDefaults(t *testing.T) {
	sandbox := NewSandbox(7, "")
	if sandbox.BaseDir != "/var/local/lib/vertex-sandbox" {
		t.Fatalf("BaseDir = %q", sandbox.BaseDir)
	}
	if sandbox.Policy != DefaultPolicy() {
		t.Fatalf("Policy = %+v, want %+v", sandbox.Policy, DefaultPolicy())
	}
}

func TestCalculateNativeLimits(t *testing.T) {
	limitsRequest := validLimits()
	limitsRequest.CPUTime = 1250 * time.Millisecond
	limitsRequest.WallTime = 2500 * time.Millisecond
	policy := DefaultPolicy()
	limits, err := calculateNativeLimits(limitsRequest, policy)
	if err != nil {
		t.Fatal(err)
	}
	if limits.timeMs != 1250 || limits.timeHardMs != 2250 {
		t.Fatalf("CPU limits = %d/%d, want 1250/2250", limits.timeMs, limits.timeHardMs)
	}
	if limits.wallMs != 2500 || limits.wallHardMs != 3500 {
		t.Fatalf("wall limits = %d/%d, want 2500/3500", limits.wallMs, limits.wallHardMs)
	}
	if limits.workspaceB != DefaultWorkspaceBytes ||
		limits.workspaceIDs != DefaultWorkspaceInodes {
		t.Fatalf("workspace limits = %d/%d", limits.workspaceB, limits.workspaceIDs)
	}
}

func TestCalculateNativeLimitsUsesExplicitHardAndWorkspaceBudgets(t *testing.T) {
	request := validLimits()
	request.CPUHardTime = 1500 * time.Millisecond
	request.WallHardTime = 2750 * time.Millisecond
	request.WorkspaceBytes = 1024
	request.WorkspaceInodes = 16
	limits, err := calculateNativeLimits(request, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if limits.timeHardMs != 1500 || limits.wallHardMs != 2750 {
		t.Fatalf("hard limits = %d/%d", limits.timeHardMs, limits.wallHardMs)
	}
	if limits.workspaceB != 1024 || limits.workspaceIDs != 16 {
		t.Fatalf("workspace limits = %d/%d", limits.workspaceB, limits.workspaceIDs)
	}
}

func TestDurationToMillisecondsRoundsUp(t *testing.T) {
	got, err := durationToMilliseconds(100 * time.Microsecond)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("milliseconds = %d, want 1", got)
	}
}

func TestExecutionArgsAddsInheritedStreamDescriptors(t *testing.T) {
	sandbox := NewSandbox(7, "/var/local/lib/vertex-sandbox")
	execution := Execution{Command: []string{"./prog"}, Limits: validLimits()}
	args, err := sandbox.executionArgs(execution, inheritedStdinFD, inheritedStdoutFD)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--stdin-fd 3") || !strings.Contains(joined, "--stdout-fd 4") {
		t.Fatalf("stream arguments missing: %s", joined)
	}
	execution.StdinFile = "input.txt"
	if _, err := sandbox.executionArgs(execution, inheritedStdinFD, inheritedStdoutFD); err == nil {
		t.Fatal("expected stdin file/stream conflict")
	}
}

func TestInvalidNativeLimits(t *testing.T) {
	const maxInt64 = int64(1<<63 - 1)
	tests := []struct {
		name   string
		limits Limits
		policy Policy
	}{
		{"zero CPU", func() Limits { v := validLimits(); v.CPUTime = 0; return v }(), DefaultPolicy()},
		{"zero wall", func() Limits { v := validLimits(); v.WallTime = 0; return v }(), DefaultPolicy()},
		{"hard CPU below soft", func() Limits { v := validLimits(); v.CPUHardTime = time.Millisecond; return v }(), DefaultPolicy()},
		{"hard wall below soft", func() Limits { v := validLimits(); v.WallHardTime = time.Millisecond; return v }(), DefaultPolicy()},
		{"negative hard CPU", func() Limits { v := validLimits(); v.CPUHardTime = -time.Millisecond; return v }(), DefaultPolicy()},
		{"zero memory", func() Limits { v := validLimits(); v.MemoryKB = 0; return v }(), DefaultPolicy()},
		{"zero processes", func() Limits { v := validLimits(); v.Processes = 0; return v }(), DefaultPolicy()},
		{"zero output", func() Limits { v := validLimits(); v.OutputBytes = 0; return v }(), DefaultPolicy()},
		{"negative stack", func() Limits { v := validLimits(); v.StackKB = -1; return v }(), DefaultPolicy()},
		{"workspace bytes above policy", func() Limits { v := validLimits(); v.WorkspaceBytes = DefaultWorkspaceBytes + 1; return v }(), DefaultPolicy()},
		{"workspace inodes above policy", func() Limits { v := validLimits(); v.WorkspaceInodes = DefaultWorkspaceInodes + 1; return v }(), DefaultPolicy()},
		{"negative overshoot", validLimits(), Policy{TimeOvershootMs: -1, WorkspaceBytes: 1, WorkspaceInodes: 1}},
		{"zero bytes", validLimits(), Policy{WorkspaceInodes: 1}},
		{"zero inodes", validLimits(), Policy{WorkspaceBytes: 1}},
		{"CPU set whitespace", validLimits(), Policy{WorkspaceBytes: 1, WorkspaceInodes: 1, CPUSet: " 0"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := calculateNativeLimits(test.limits, test.policy); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	if _, err := addMilliseconds(maxInt64, 1); err == nil {
		t.Fatal("expected hard-limit overflow error")
	}
}

func TestCopyOutRegularFile(t *testing.T) {
	base := t.TempDir()
	sandbox := NewSandbox(7, base)
	if err := os.MkdirAll(filepath.Dir(sandbox.BoxPath("artifact.bin")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sandbox.BoxPath("artifact.bin"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "artifact.bin")
	if err := sandbox.CopyOut(context.Background(), "artifact.bin", destination, 5); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("artifact = %q", data)
	}
}

func TestCopyOutRejectsUnsafeArtifacts(t *testing.T) {
	base := t.TempDir()
	sandbox := NewSandbox(7, base)
	boxDir := filepath.Dir(sandbox.BoxPath("unused"))
	if err := os.MkdirAll(boxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sandbox.BoxPath("large.bin"), []byte("too large"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sandbox.BoxPath("directory"), 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		boxName  string
		maxBytes int64
		prepare  func(string)
	}{
		{name: "path traversal", boxName: "../secret", maxBytes: 10},
		{name: "oversized", boxName: "large.bin", maxBytes: 3},
		{name: "directory", boxName: "directory", maxBytes: 10},
		{name: "existing destination", boxName: "large.bin", maxBytes: 32, prepare: func(path string) {
			if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "artifact")
			if test.prepare != nil {
				test.prepare(destination)
			}
			if err := sandbox.CopyOut(context.Background(), test.boxName, destination, test.maxBytes); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestCopyOutRejectsSymlink(t *testing.T) {
	base := t.TempDir()
	sandbox := NewSandbox(7, base)
	if err := os.MkdirAll(filepath.Dir(sandbox.BoxPath("link")), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, sandbox.BoxPath("link")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := sandbox.CopyOut(context.Background(), "link", filepath.Join(t.TempDir(), "artifact"), 32); err == nil {
		t.Fatal("expected symlink artifact to be rejected")
	}
}

func TestValidateBoxName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want bool
	}{
		{"", false},
		{"input.txt", true},
		{"dir/input.txt", false},
		{`dir\input.txt`, false},
		{".", false},
		{"../secret", false},
		{"dir/../../secret", false},
		{"/etc/passwd", false},
		{"dir/../input.txt", false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := validateBoxName(test.name) == nil; got != test.want {
				t.Fatalf("validateBoxName(%q) valid = %v, want %v", test.name, got, test.want)
			}
		})
	}
}

func TestRunnerEnvironmentIsAllowlisted(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://secret")
	t.Setenv("VERTEX_CGROUP_ROOT", "/sys/fs/cgroup/test")
	environment := runnerEnvironment()
	for _, entry := range environment {
		if entry == "DATABASE_URL=postgres://secret" {
			t.Fatal("runner environment leaked DATABASE_URL")
		}
	}
}
