package run

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// TerminationReason describes why the isolated process tree stopped. It is
// intentionally independent from Judge verdicts so non-judging tasks can use
// the same execution contract.
type TerminationReason string

const (
	TerminationExited         TerminationReason = "exited"
	TerminationSignal         TerminationReason = "signal"
	TerminationTimeLimit      TerminationReason = "time-limit"
	TerminationMemoryLimit    TerminationReason = "memory-limit"
	TerminationOutputLimit    TerminationReason = "output-limit"
	TerminationWorkspaceLimit TerminationReason = "workspace-limit"
	TerminationCancelled      TerminationReason = "cancelled"
	TerminationSetupError     TerminationReason = "setup-error"
)

// TimeResult records whether an execution crossed its soft or hard time
// budget. A hard result does not necessarily imply Killed when the process
// exits between the final resource sample and cgroup termination.
type TimeResult string

const (
	TimeResultNone TimeResult = "none"
	TimeResultSoft TimeResult = "soft"
	TimeResultHard TimeResult = "hard"
)

// Meta is the stable, verdict-neutral metadata contract emitted by
// vertex-sandbox. Status is retained for compatibility with existing Judge
// mapping; new task types should prefer TerminationReason and limit fields.
type Meta struct {
	Status            string
	TerminationReason TerminationReason
	TimeResult        TimeResult
	TimeLimit         string
	Time              float64
	TimeWall          float64
	MaxRSS            int
	ExitCode          int
	ExitSignal        int
	Killed            bool
	CgOOMKilled       bool
	OutputLimit       bool
	StdoutStreamed    bool
	StdoutBytes       int64
	StderrBytes       int64
	WorkspaceLimit    bool
	WorkspaceBytes    int64
	WorkspaceInodes   int64
	CgMem             int
	Message           string
}

// ParseMeta reads the line-oriented key:value metadata emitted by the native
// runner. Unknown keys are ignored for forward compatibility.
func ParseMeta(path string) (*Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m := &Meta{}
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
		case "termination-reason":
			m.TerminationReason = TerminationReason(val)
		case "time-result":
			m.TimeResult = TimeResult(val)
		case "time-limit":
			m.TimeLimit = val
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
		case "stdout-streamed":
			m.StdoutStreamed = val == "1" || val == "true"
		case "stdout-bytes":
			m.StdoutBytes, _ = strconv.ParseInt(val, 10, 64)
		case "stderr-bytes":
			m.StderrBytes, _ = strconv.ParseInt(val, 10, 64)
		case "workspace-limit":
			m.WorkspaceLimit = val == "1" || val == "true"
		case "workspace-bytes":
			m.WorkspaceBytes, _ = strconv.ParseInt(val, 10, 64)
		case "workspace-inodes":
			m.WorkspaceInodes, _ = strconv.ParseInt(val, 10, 64)
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

func (m *Meta) EffectiveMemoryKB() int {
	if m.CgMem > m.MaxRSS {
		return m.CgMem
	}
	return m.MaxRSS
}

func (m *Meta) ExitDescription() string {
	if m.WorkspaceLimit && m.Message != "" {
		return m.Message
	}
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
