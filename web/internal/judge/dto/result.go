package dto

import "github.com/RimuruChan/Vertex/web/internal/judge"

type CaseResultRequest struct {
	CaseIndex     int    `json:"caseIndex"`
	Verdict       string `json:"verdict"`
	TimeMs        int    `json:"timeMs"`
	MemoryKB      int    `json:"memoryKb"`
	ExitStatus    string `json:"exitStatus,omitempty"`
	CheckerOutput string `json:"checkerOutput,omitempty"`
}

type ResultRequest struct {
	WorkerID      string              `json:"workerId" binding:"required"`
	SubmissionID  string              `json:"submissionId" binding:"required"`
	Generation    int                 `json:"generation" binding:"required"`
	LeaseToken    string              `json:"leaseToken" binding:"required"`
	Status        string              `json:"status" binding:"required"`
	Score         int                 `json:"score"`
	TotalTimeMs   int64               `json:"totalTimeMs"`
	PeakMemoryKB  int                 `json:"peakMemoryKb"`
	CompileResult string              `json:"compileResult,omitempty"`
	Cases         []CaseResultRequest `json:"cases,omitempty"`
}

func (request ResultRequest) Domain(jobID string) judge.Result {
	result := judge.Result{
		JobID: jobID, SubmissionID: request.SubmissionID,
		Generation: request.Generation, LeaseToken: request.LeaseToken, WorkerID: request.WorkerID,
		Status: request.Status, Score: request.Score, TotalTimeMs: request.TotalTimeMs,
		PeakMemoryKB: request.PeakMemoryKB, CompileResult: request.CompileResult,
		Cases: make([]judge.CaseResult, 0, len(request.Cases)),
	}
	for _, item := range request.Cases {
		result.Cases = append(result.Cases, judge.CaseResult{
			CaseIndex: item.CaseIndex, Verdict: item.Verdict, TimeMs: item.TimeMs,
			MemoryKB: item.MemoryKB, ExitStatus: item.ExitStatus, CheckerOutput: item.CheckerOutput,
		})
	}
	return result
}
