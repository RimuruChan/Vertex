package run

import (
	"os"
	"testing"
)

func TestParseMeta(t *testing.T) {
	content := `status: TO
termination-reason: time-limit
time-result: hard
time-limit: cpu,wall
time: 1.05
time-wall: 2.10
max-rss: 123456
exitcode: 0
killed: 1
cg-oom-killed: 1
output-limit: 1
stdout-streamed: 1
stdout-bytes: 33554432
stderr-bytes: 1024
workspace-limit: 1
workspace-bytes: 67108864
workspace-inodes: 4096
cg-mem: 131072
message: cpu and wall time limits exceeded`
	path := t.TempDir() + "/meta"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := ParseMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != "TO" || m.TerminationReason != TerminationTimeLimit {
		t.Errorf("status/reason = %q/%q", m.Status, m.TerminationReason)
	}
	if m.TimeResult != TimeResultHard || m.TimeLimit != "cpu,wall" {
		t.Errorf("time result = %q/%q", m.TimeResult, m.TimeLimit)
	}
	if m.Time != 1.05 || m.TimeWall != 2.10 {
		t.Errorf("time = %v/%v", m.Time, m.TimeWall)
	}
	if !m.Killed || !m.CgOOMKilled || !m.OutputLimit || !m.StdoutStreamed {
		t.Error("boolean meta fields were not parsed")
	}
	if m.StdoutBytes != 33554432 || m.StderrBytes != 1024 {
		t.Errorf("stream bytes = %d/%d", m.StdoutBytes, m.StderrBytes)
	}
	if !m.WorkspaceLimit || m.WorkspaceBytes != 67108864 || m.WorkspaceInodes != 4096 {
		t.Errorf("workspace meta = %v/%d/%d", m.WorkspaceLimit, m.WorkspaceBytes, m.WorkspaceInodes)
	}
	if m.EffectiveMemoryKB() != 131072 {
		t.Errorf("EffectiveMemoryKB() = %d, want 131072", m.EffectiveMemoryKB())
	}
}

func TestParseMetaIgnoresUnknownFields(t *testing.T) {
	path := t.TempDir() + "/meta"
	if err := os.WriteFile(path, []byte("future-field:value\nexitcode:0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta, err := ParseMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ExitCode != 0 {
		t.Fatalf("exit code = %d", meta.ExitCode)
	}
}

func TestExitDescription(t *testing.T) {
	tests := []struct {
		name string
		m    *Meta
		want string
	}{
		{"signal", &Meta{Status: "SG", ExitSignal: 11}, "signal 11"},
		{"exit code", &Meta{Status: "RE", ExitCode: 1}, "exit 1"},
		{"message", &Meta{Status: "TO", Message: "timeout"}, "timeout"},
		{"workspace message", &Meta{Status: "SG", ExitSignal: 9, WorkspaceLimit: true, Message: "workspace limit"}, "workspace limit"},
		{"status only", &Meta{Status: "XX"}, "XX"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.m.ExitDescription(); got != test.want {
				t.Errorf("ExitDescription() = %q, want %q", got, test.want)
			}
		})
	}
}
