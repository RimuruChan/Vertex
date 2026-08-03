package run

import (
	"math"
	"testing"
)

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
	cfg := &Config{TimeLimitSec: 1.25, WallLimitSec: 2.5}
	policy := DefaultPolicy()
	limits, err := calculateNativeLimits(cfg, policy)
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

func TestSecondsToMillisecondsMinimum(t *testing.T) {
	got, err := secondsToMilliseconds(0.0001)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("milliseconds = %d, want 1", got)
	}
}

func TestInvalidNativeLimits(t *testing.T) {
	const maxInt64 = int64(1<<63 - 1)
	tests := []struct {
		name   string
		cfg    Config
		policy Policy
	}{
		{"zero CPU", Config{WallLimitSec: 1}, DefaultPolicy()},
		{"NaN wall", Config{TimeLimitSec: 1, WallLimitSec: math.NaN()}, DefaultPolicy()},
		{"negative overshoot", Config{TimeLimitSec: 1, WallLimitSec: 1}, Policy{TimeOvershootMs: -1, WorkspaceBytes: 1, WorkspaceInodes: 1}},
		{"zero bytes", Config{TimeLimitSec: 1, WallLimitSec: 1}, Policy{WorkspaceInodes: 1}},
		{"zero inodes", Config{TimeLimitSec: 1, WallLimitSec: 1}, Policy{WorkspaceBytes: 1}},
		{"CPU set whitespace", Config{TimeLimitSec: 1, WallLimitSec: 1}, Policy{WorkspaceBytes: 1, WorkspaceInodes: 1, CPUSet: " 0"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := calculateNativeLimits(&test.cfg, test.policy); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	if _, err := addMilliseconds(maxInt64, 1); err == nil {
		t.Fatal("expected hard-limit overflow error")
	}
	if _, err := secondsToMilliseconds(float64(math.MaxInt64) / 1000); err == nil {
		t.Fatal("expected millisecond conversion overflow error")
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
