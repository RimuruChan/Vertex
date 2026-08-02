package verdict

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
// sandbox meta 关键字段:
//   - status: RE(非零退出)/SG(被信号杀)/TO(超时)/XX(内部错误)
//   - time / time-wall / max-rss / exitcode / exitsig
//   - cg-oom-killed: cgroup OOM 是否杀掉了进程
//   - output-limit: stdout/stderr 是否实际达到限制
//   - killed: 是否因限制被杀
//
// 映射规则(沙箱强制清单):
//   - cg-oom-killed = 1 → MLE(不是 TLE!)
//   - output-limit = 1 → OLE
//   - status TO → TLE
//   - status SG 且有信号 → RE(携带信号)
//   - status RE → RE(携带退出码)
//   - status XX → SE
func FromSandboxMeta(m *SandboxMeta) string {
	switch {
	case m == nil:
		return SE
	case m.CgOOMKilled:
		return MLE
	case m.Status == "TO":
		return TLE
	case m.OutputLimit:
		return OLE
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
