package verdict

import "github.com/RimuruChan/Vertex/worker/internal/run"

// 判定分类(与数据库 CHECK 约束一致)。
const (
	AC   = "Accepted"
	WA   = "Wrong Answer"
	TLE  = "Time Limit Exceeded"
	MLE  = "Memory Limit Exceeded"
	RE   = "Runtime Error"
	CE   = "Compile Error"
	OLE  = "Output Limit Exceeded"
	SE   = "System Error"
	Skip = "Skipped"
)

// FromSandboxMeta maps native sandbox metadata to a judge verdict.
//
// 新 runner 优先提供 verdict-neutral termination reason；status 和 limit
// flags 作为旧 meta 的兼容回退。
//
// 映射规则(沙箱强制清单):
//   - memory-limit/cg-oom-killed → MLE(不是 TLE)
//   - output-limit → OLE
//   - workspace-limit → RE
//   - time-limit/status TO → TLE
//   - cancelled/setup-error/status XX → SE
//   - signal 或非零退出 → RE
func FromSandboxMeta(m *run.Meta) string {
	switch {
	case m == nil:
		return SE
	case m.CgOOMKilled || m.TerminationReason == run.TerminationMemoryLimit:
		return MLE
	case m.OutputLimit || m.TerminationReason == run.TerminationOutputLimit:
		return OLE
	case m.WorkspaceLimit || m.TerminationReason == run.TerminationWorkspaceLimit:
		return RE
	case m.TerminationReason == run.TerminationTimeLimit || m.Status == "TO":
		return TLE
	case m.TerminationReason == run.TerminationCancelled ||
		m.TerminationReason == run.TerminationSetupError:
		return SE
	case m.TerminationReason == run.TerminationSignal:
		return RE
	case m.Killed && m.Status == "SG" && m.ExitSignal == 0:
		// 兜底:被限制杀掉但没有明确信号时,若墙钟超限判 TLE
		if m.TimeWall > 0 {
			return TLE
		}
		return RE
	case m.Status == "XX":
		return SE
	case m.Status == "SG":
		return RE
	case m.Status == "RE":
		return RE
	default:
		return ""
	}
}

// IsFailedVerdict 判定是否非 AC(用于统计尝试)。
func IsFailedVerdict(v string) bool {
	return v != AC && v != ""
}
