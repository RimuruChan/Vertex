package dto

import (
	"time"

	judgedomain "github.com/RimuruChan/Vertex/server/internal/judge/domain"
)

type ClaimRequest struct {
	WorkerID     string   `json:"workerId" binding:"required"`
	WaitSeconds  int      `json:"waitSeconds,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type JobResponse struct {
	DomainID       string           `json:"domainId"`
	ProblemVersion int              `json:"problemVersion"`
	JobID          string           `json:"jobId"`
	SubmissionID   string           `json:"submissionId"`
	Generation     int              `json:"generation"`
	Attempt        int              `json:"attempt"`
	LeaseToken     string           `json:"leaseToken"`
	LeaseExpiresAt time.Time        `json:"leaseExpiresAt"`
	Language       string           `json:"language"`
	SourceCode     string           `json:"sourceCode"`
	ProblemID      string           `json:"problemId"`
	ContestID      *string          `json:"contestId,omitempty"`
	TimeLimitMs    int              `json:"timeLimitMs"`
	MemoryLimitKB  int              `json:"memoryLimitKb"`
	Testdata       TestdataResponse `json:"testdata"`
}

type TestdataResponse struct {
	StoragePath string `json:"storagePath"`
	DataVersion int    `json:"dataVersion"`
	SHA256      string `json:"sha256"`
	CaseCount   int    `json:"caseCount"`
	Checker     string `json:"checker"`
}

func JobFromDomain(job *judgedomain.Job) JobResponse {
	return JobResponse{
		DomainID: job.DomainID, ProblemVersion: job.ProblemVersion,
		JobID: job.ID, SubmissionID: job.SubmissionID, Generation: job.Generation,
		Attempt: job.Attempt, LeaseToken: job.LeaseToken, LeaseExpiresAt: job.LeaseExpiresAt,
		Language: job.Language, SourceCode: job.SourceCode, ProblemID: job.ProblemID,
		ContestID: job.ContestID, TimeLimitMs: job.TimeLimitMs, MemoryLimitKB: job.MemoryLimitKB,
		Testdata: TestdataResponse{
			StoragePath: job.Testdata.StoragePath, DataVersion: job.Testdata.DataVersion,
			SHA256: job.Testdata.SHA256, CaseCount: job.Testdata.CaseCount, Checker: job.Testdata.Checker,
		},
	}
}
