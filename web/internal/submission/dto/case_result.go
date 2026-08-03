package dto

import "github.com/RimuruChan/Vertex/web/internal/submission"

type CaseResultResponse struct {
	CaseIndex     int    `json:"caseIndex"`
	Verdict       string `json:"verdict"`
	TimeMs        int    `json:"timeMs"`
	MemoryKB      int    `json:"memoryKb"`
	ExitStatus    string `json:"exitStatus,omitempty"`
	CheckerOutput string `json:"checkerOutput,omitempty"`
}

func FromCaseResult(value submission.CaseResult) CaseResultResponse {
	return CaseResultResponse{
		CaseIndex: value.CaseIndex, Verdict: value.Verdict, TimeMs: value.TimeMs,
		MemoryKB: value.MemoryKb, ExitStatus: value.ExitStatus, CheckerOutput: value.CheckerOutput,
	}
}
