package dto

import submissiondomain "github.com/RimuruChan/Vertex/server/internal/modules/submission/domain"

// SubmissionProgressResponse is the polling contract. It excludes source,
// participant identity and problem metadata by design.
type SubmissionProgressResponse struct {
	ID            string               `json:"id"`
	Status        string               `json:"status"`
	Score         int                  `json:"score"`
	TotalTimeMs   int                  `json:"totalTimeMs"`
	PeakMemoryKB  int                  `json:"peakMemoryKb"`
	CompileResult string               `json:"compileResult,omitempty"`
	CaseResults   []CaseResultResponse `json:"caseResults,omitempty"`
	JudgedCases   int                  `json:"judgedCases"`
	TotalCases    int                  `json:"totalCases"`
}

func FromProgress(value submissiondomain.SubmissionProgress) SubmissionProgressResponse {
	response := SubmissionProgressResponse{
		ID: value.ID, Status: value.Status, Score: value.Score,
		TotalTimeMs: value.TotalTimeMs, PeakMemoryKB: value.PeakMemoryKb,
		CompileResult: value.CompileResult,
		JudgedCases:   value.JudgedCases, TotalCases: value.TotalCases,
		CaseResults: make([]CaseResultResponse, 0, len(value.CaseResults)),
	}
	for _, item := range value.CaseResults {
		response.CaseResults = append(response.CaseResults, FromCaseResult(item))
	}
	return response
}
