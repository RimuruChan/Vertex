package verdict

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// SandboxMeta is the stable metadata contract emitted by vertex-sandbox.
// The line-oriented key:value format intentionally remains easy to diagnose.
type SandboxMeta struct {
	Status      string  // RE / SG / TO / XX
	Time        float64 // CPU 时间(秒)
	TimeWall    float64 // 墙钟时间(秒)
	MaxRSS      int     // 峰值内存(KB)
	ExitCode    int     // 退出码
	ExitSignal  int     // 信号编号(被信号杀时)
	Killed      bool    // 是否因限制被杀
	CgOOMKilled bool    // cgroup OOM 是否杀进程
	OutputLimit bool    // stdout/stderr 是否达到输出上限
	CgMem       int     // cgroup 峰值内存(KB)
	Message     string  // sandbox diagnostic
}

// ParseSandboxMeta reads one native sandbox metadata file.
func ParseSandboxMeta(path string) (*SandboxMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m := &SandboxMeta{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		switch key {
		case "status":
			m.Status = val
		case "time":
			m.Time, _ = strconv.ParseFloat(val, 64)
		case "time-wall":
			m.TimeWall, _ = strconv.ParseFloat(val, 64)
		case "max-rss":
			m.MaxRSS, _ = strconv.Atoi(val)
		case "exitcode":
			m.ExitCode, _ = strconv.Atoi(val)
		case "exitsig":
			m.ExitSignal, _ = strconv.Atoi(val)
		case "killed":
			m.Killed = val == "1" || val == "true"
		case "cg-oom-killed":
			m.CgOOMKilled = val == "1" || val == "true"
		case "output-limit":
			m.OutputLimit = val == "1" || val == "true"
		case "cg-mem":
			m.CgMem, _ = strconv.Atoi(val)
		case "message":
			m.Message = val
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

// WallTimeExceeded 判断墙钟时间是否超限(供 TLE 判定)。
// 调用方传入允许的墙钟秒数;若 meta 记录的墙钟超过该值视为超时。
func (m *SandboxMeta) WallTimeExceeded() bool {
	return m.Killed && (m.Status == "TO" || m.TimeWall > 0)
}

// EffectiveMemoryKB 返回 cgroup 峰值或 max-rss 中较大者(内存统计口径)。
func (m *SandboxMeta) EffectiveMemoryKB() int {
	if m.CgMem > m.MaxRSS {
		return m.CgMem
	}
	return m.MaxRSS
}

// ExitDescription 生成退出状态描述(存 exit_status 字段)。
func (m *SandboxMeta) ExitDescription() string {
	if m.Status == "SG" && m.ExitSignal > 0 {
		return "signal " + strconv.Itoa(m.ExitSignal)
	}
	if m.Status == "RE" {
		return "exit " + strconv.Itoa(m.ExitCode)
	}
	if m.Message != "" {
		return m.Message
	}
	return m.Status
}
