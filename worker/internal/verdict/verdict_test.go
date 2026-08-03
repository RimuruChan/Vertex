package verdict

import (
	"testing"

	"github.com/RimuruChan/Vertex/worker/internal/run"
)

func TestFromSandboxMeta(t *testing.T) {
	tests := []struct {
		name string
		meta *run.Meta
		want string
	}{
		{
			name: "nil meta is system error",
			meta: nil,
			want: SE,
		},
		{
			name: "cgroup oom killed is MLE not TLE",
			meta: &run.Meta{Status: "TO", Killed: true, CgOOMKilled: true, TimeWall: 2.5},
			want: MLE,
		},
		{
			name: "timeout is TLE",
			meta: &run.Meta{Status: "TO", Killed: true, TimeWall: 2.5},
			want: TLE,
		},
		{
			name: "neutral timeout reason is TLE",
			meta: &run.Meta{TerminationReason: run.TerminationTimeLimit, TimeResult: run.TimeResultSoft},
			want: TLE,
		},
		{
			name: "killed by signal is RE",
			meta: &run.Meta{Status: "SG", ExitSignal: 11}, // SIGSEGV
			want: RE,
		},
		{
			name: "observed output limit is OLE",
			meta: &run.Meta{Status: "SG", ExitSignal: 25, OutputLimit: true},
			want: OLE,
		},
		{
			name: "workspace limit is RE",
			meta: &run.Meta{Status: "SG", ExitSignal: 9, WorkspaceLimit: true},
			want: RE,
		},
		{
			name: "bare file size signal is RE",
			meta: &run.Meta{Status: "SG", ExitSignal: 25},
			want: RE,
		},
		{
			name: "nonzero exit is RE",
			meta: &run.Meta{Status: "RE", ExitCode: 1},
			want: RE,
		},
		{
			name: "internal sandbox error is SE",
			meta: &run.Meta{Status: "XX"},
			want: SE,
		},
		{
			name: "neutral cancellation reason is SE",
			meta: &run.Meta{TerminationReason: run.TerminationCancelled},
			want: SE,
		},
		{
			name: "normal exit returns empty (diff checker decides)",
			meta: &run.Meta{Status: "", ExitCode: 0},
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
