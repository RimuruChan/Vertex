package verdict

import (
	"os"
	"testing"
)

func TestFromIsolateMeta(t *testing.T) {
	tests := []struct {
		name string
		meta *IsolateMeta
		want string
	}{
		{
			name: "nil meta is system error",
			meta: nil,
			want: SE,
		},
		{
			name: "cgroup oom killed is MLE not TLE",
			meta: &IsolateMeta{Status: "TO", Killed: true, CgOOMKilled: true, TimeWall: 2.5},
			want: MLE,
		},
		{
			name: "timeout is TLE",
			meta: &IsolateMeta{Status: "TO", Killed: true, TimeWall: 2.5},
			want: TLE,
		},
		{
			name: "killed by signal is RE",
			meta: &IsolateMeta{Status: "SG", ExitSignal: 11}, // SIGSEGV
			want: RE,
		},
		{
			name: "nonzero exit is RE",
			meta: &IsolateMeta{Status: "RE", ExitCode: 1},
			want: RE,
		},
		{
			name: "internal isolate error is SE",
			meta: &IsolateMeta{Status: "XX"},
			want: SE,
		},
		{
			name: "normal exit returns empty (diff checker decides)",
			meta: &IsolateMeta{Status: "", ExitCode: 0},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FromIsolateMeta(tt.meta); got != tt.want {
				t.Errorf("FromIsolateMeta() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseIsolateMeta(t *testing.T) {
	content := `status: TO
time: 1.05
time-wall: 2.10
max-rss: 123456
exitcode: 0
killed: 1
cg-oom-killed: 1
cg-mem: 131072
message: cpu time limit exceeded`
	path := t.TempDir() + "/meta"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := ParseIsolateMeta(path)
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
	if m.EffectiveMemoryKB() != 131072 {
		t.Errorf("EffectiveMemoryKB() = %d, want 131072 (cg-mem wins)", m.EffectiveMemoryKB())
	}
}

func TestExitDescription(t *testing.T) {
	tests := []struct {
		name string
		m    *IsolateMeta
		want string
	}{
		{"signal", &IsolateMeta{Status: "SG", ExitSignal: 11}, "signal 11"},
		{"exit code", &IsolateMeta{Status: "RE", ExitCode: 1}, "exit 1"},
		{"message", &IsolateMeta{Status: "TO", Message: "timeout"}, "timeout"},
		{"status only", &IsolateMeta{Status: "XX"}, "XX"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.ExitDescription(); got != tt.want {
				t.Errorf("ExitDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}
