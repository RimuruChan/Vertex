package verdict

import (
	"os"
	"testing"
)

func TestFromSandboxMeta(t *testing.T) {
	tests := []struct {
		name string
		meta *SandboxMeta
		want string
	}{
		{
			name: "nil meta is system error",
			meta: nil,
			want: SE,
		},
		{
			name: "cgroup oom killed is MLE not TLE",
			meta: &SandboxMeta{Status: "TO", Killed: true, CgOOMKilled: true, TimeWall: 2.5},
			want: MLE,
		},
		{
			name: "timeout is TLE",
			meta: &SandboxMeta{Status: "TO", Killed: true, TimeWall: 2.5},
			want: TLE,
		},
		{
			name: "killed by signal is RE",
			meta: &SandboxMeta{Status: "SG", ExitSignal: 11}, // SIGSEGV
			want: RE,
		},
		{
			name: "observed output limit is OLE",
			meta: &SandboxMeta{Status: "SG", ExitSignal: 25, OutputLimit: true},
			want: OLE,
		},
		{
			name: "workspace limit is RE",
			meta: &SandboxMeta{Status: "SG", ExitSignal: 9, WorkspaceLimit: true},
			want: RE,
		},
		{
			name: "bare file size signal is RE",
			meta: &SandboxMeta{Status: "SG", ExitSignal: 25},
			want: RE,
		},
		{
			name: "nonzero exit is RE",
			meta: &SandboxMeta{Status: "RE", ExitCode: 1},
			want: RE,
		},
		{
			name: "internal sandbox error is SE",
			meta: &SandboxMeta{Status: "XX"},
			want: SE,
		},
		{
			name: "normal exit returns empty (diff checker decides)",
			meta: &SandboxMeta{Status: "", ExitCode: 0},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FromSandboxMeta(tt.meta); got != tt.want {
				t.Errorf("FromSandboxMeta() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseSandboxMeta(t *testing.T) {
	content := `status: TO
time: 1.05
time-wall: 2.10
max-rss: 123456
exitcode: 0
killed: 1
cg-oom-killed: 1
output-limit: 1
workspace-limit: 1
workspace-bytes: 67108864
workspace-inodes: 4096
cg-mem: 131072
message: cpu time limit exceeded`
	path := t.TempDir() + "/meta"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := ParseSandboxMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != "TO" {
		t.Errorf("status = %q, want TO", m.Status)
	}
	if m.Time != 1.05 {
		t.Errorf("time = %v, want 1.05", m.Time)
	}
	if !m.Killed {
		t.Error("killed should be true")
	}
	if !m.CgOOMKilled {
		t.Error("cg-oom-killed should be true")
	}
	if !m.OutputLimit {
		t.Error("output-limit should be true")
	}
	if !m.WorkspaceLimit || m.WorkspaceBytes != 67108864 || m.WorkspaceInodes != 4096 {
		t.Errorf("workspace meta = %v/%d/%d", m.WorkspaceLimit, m.WorkspaceBytes, m.WorkspaceInodes)
	}
	if m.EffectiveMemoryKB() != 131072 {
		t.Errorf("EffectiveMemoryKB() = %d, want 131072 (cg-mem wins)", m.EffectiveMemoryKB())
	}
}

func TestExitDescription(t *testing.T) {
	tests := []struct {
		name string
		m    *SandboxMeta
		want string
	}{
		{"signal", &SandboxMeta{Status: "SG", ExitSignal: 11}, "signal 11"},
		{"exit code", &SandboxMeta{Status: "RE", ExitCode: 1}, "exit 1"},
		{"message", &SandboxMeta{Status: "TO", Message: "timeout"}, "timeout"},
		{"workspace message", &SandboxMeta{Status: "SG", ExitSignal: 9, WorkspaceLimit: true, Message: "workspace limit"}, "workspace limit"},
		{"status only", &SandboxMeta{Status: "XX"}, "XX"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.ExitDescription(); got != tt.want {
				t.Errorf("ExitDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}
