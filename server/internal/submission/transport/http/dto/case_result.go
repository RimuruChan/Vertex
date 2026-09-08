package dto

import submissiondomain "github.com/RimuruChan/Vertex/server/internal/submission/domain"

type CaseResultResponse struct {
	CaseIndex     int    `json:"caseIndex"`
	Verdict       string `json:"verdict"`
	TimeMs        int    `json:"timeMs"`
	MemoryKB      int    `json:"memoryKb"`
	ExitStatus    string `json:"exitStatus,omitempty"`
	CheckerOutput string `json:"checkerOutput,omitempty"`
}

func FromCaseResult(value submissiondomain.CaseResult) CaseResultResponse {
	return CaseResultResponse{
		CaseIndex: value.CaseIndex, Verdict: value.Verdict, TimeMs: value.TimeMs,
		MemoryKB: value.MemoryKb, ExitStatus: value.ExitStatus, CheckerOutput: value.CheckerOutput,
	}
}
