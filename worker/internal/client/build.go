package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/builder"
	"github.com/RimuruChan/Vertex/worker/internal/scheduler"
)

// ---------- wire types ----------

type buildClaimRequest struct {
	WorkerID    string `json:"workerId"`
	WaitSeconds int    `json:"waitSeconds"`
}

type buildFile struct {
	Name       string `json:"name"`
	Language   string `json:"language"`
	SourceCode string `json:"sourceCode"`
}

type buildSolution struct {
	Name            string `json:"name"`
	Language        string `json:"language"`
	SourceCode      string `json:"sourceCode"`
	ExpectedVerdict string `json:"expectedVerdict"`
	IsMain          bool   `json:"isMain"`
}

type buildTest struct {
	Index       int    `json:"index"`
	Group       string `json:"group"`
	Source      string `json:"source"`
	InputData   string `json:"inputData"`
	GenerateCmd string `json:"generateCmd"`
	IsSample    bool   `json:"isSample"`
	Points      int    `json:"points"`
}

type buildLimits struct {
	GeneratorTimeMs int `json:"generatorTimeMs"`
	ValidatorTimeMs int `json:"validatorTimeMs"`
	SolutionTimeMs  int `json:"solutionTimeMs"`
	CheckerTimeMs   int `json:"checkerTimeMs"`
	MemoryLimitKB   int `json:"memoryLimitKb"`
}

type buildJobResponse struct {
	DomainID       string          `json:"domainId"`
	DataRevision   int             `json:"dataRevision"`
	BuildID        string          `json:"buildId"`
	ProblemID      string          `json:"problemId"`
	Revision       int             `json:"revision"`
	Attempt        int             `json:"attempt"`
	LeaseToken     string          `json:"leaseToken"`
	LeaseExpiresAt time.Time       `json:"leaseExpiresAt"`
	TimeLimitMs    int             `json:"timeLimitMs"`
	MemoryLimitKB  int             `json:"memoryLimitKb"`
	JudgeType      string          `json:"judgeType"`
	Checker        *buildFile      `json:"checker"`
	Validator      *buildFile      `json:"validator"`
	Interactor     *buildFile      `json:"interactor"`
	Generators     []buildFile     `json:"generators"`
	Solutions      []buildSolution `json:"solutions"`
	Tests          []buildTest     `json:"tests"`
	Limits         buildLimits     `json:"limits"`
}

type buildProgressRequest struct {
	WorkerID   string `json:"workerId"`
	LeaseToken string `json:"leaseToken"`
	Stage      string `json:"stage,omitempty"`
	Done       int    `json:"done,omitempty"`
	Total      int    `json:"total,omitempty"`
	Log        string `json:"log,omitempty"`
}

type buildResultRequest struct {
	WorkerID     string                    `json:"workerId"`
	LeaseToken   string                    `json:"leaseToken"`
	Success      bool                      `json:"success"`
	Stage        string                    `json:"stage,omitempty"`
	Checker      string                    `json:"checker,omitempty"`
	Log          string                    `json:"log,omitempty"`
	ErrorMessage string                    `json:"errorMessage,omitempty"`
	Tests        []builder.TestOutcome     `json:"tests,omitempty"`
	Solutions    []builder.SolutionOutcome `json:"solutions,omitempty"`
}

// ---------- protocol ----------

// ClaimBuild long-polls for one package build job. A nil job means the poll
// expired with nothing queued.
func (c *Client) ClaimBuild(ctx context.Context) (*builder.Job, error) {
	delay := c.retryBase
	for {
		var response buildJobResponse
		status, err := c.doJSON(ctx, http.MethodPost, "/builds/claim", buildClaimRequest{
			WorkerID: c.workerID, WaitSeconds: c.waitSeconds,
		}, &response)
		if err == nil {
			switch {
			case status == http.StatusNoContent:
				return nil, nil
			case status == http.StatusOK:
				return buildJobFromResponse(response), nil
			case status < http.StatusInternalServerError:
				return nil, fmt.Errorf("build claim returned HTTP %d", status)
			}
		}
		if err := waitRetry(ctx, jitter(delay)); err != nil {
			return nil, err
		}
		delay = min(delay*2, c.retryMax)
	}
}

func buildJobFromResponse(response buildJobResponse) *builder.Job {
	job := &builder.Job{
		DomainID: response.DomainID, DataRevision: response.DataRevision,
		BuildID: response.BuildID, ProblemID: response.ProblemID,
		Revision: response.Revision, Attempt: response.Attempt,
		LeaseToken: response.LeaseToken, LeaseExpires: response.LeaseExpiresAt,
		TimeLimitMs: response.TimeLimitMs, MemoryLimitKB: response.MemoryLimitKB,
		JudgeType:  response.JudgeType,
		Generators: make([]builder.SourceFile, 0, len(response.Generators)),
		Solutions:  make([]builder.Solution, 0, len(response.Solutions)),
		Tests:      make([]builder.TestSpec, 0, len(response.Tests)),
		Limits: builder.Limits{
			GeneratorTimeMs: response.Limits.GeneratorTimeMs,
			ValidatorTimeMs: response.Limits.ValidatorTimeMs,
			SolutionTimeMs:  response.Limits.SolutionTimeMs,
			CheckerTimeMs:   response.Limits.CheckerTimeMs,
			MemoryLimitKB:   response.Limits.MemoryLimitKB,
		},
	}
	job.Checker = sourceFile(response.Checker)
	job.Validator = sourceFile(response.Validator)
	job.Interactor = sourceFile(response.Interactor)
	for _, generator := range response.Generators {
		job.Generators = append(job.Generators, builder.SourceFile{
			Name: generator.Name, Language: generator.Language, SourceCode: generator.SourceCode,
		})
	}
	for _, solution := range response.Solutions {
		job.Solutions = append(job.Solutions, builder.Solution{
			SourceFile: builder.SourceFile{
				Name: solution.Name, Language: solution.Language, SourceCode: solution.SourceCode,
			},
			ExpectedVerdict: solution.ExpectedVerdict, IsMain: solution.IsMain,
		})
	}
	for _, test := range response.Tests {
		job.Tests = append(job.Tests, builder.TestSpec{
			Index: test.Index, Group: test.Group, Source: test.Source,
			InputData: test.InputData, GenerateCmd: test.GenerateCmd,
			IsSample: test.IsSample, Points: test.Points,
		})
	}
	return job
}

func sourceFile(file *buildFile) *builder.SourceFile {
	if file == nil {
		return nil
	}
	return &builder.SourceFile{Name: file.Name, Language: file.Language, SourceCode: file.SourceCode}
}

// ReportBuildProgress renews the lease and publishes stage progress. Unlike
// the judge heartbeat it is not retried: the next tick carries the same state,
// and a lost lease must surface immediately so the build can stop.
func (c *Client) ReportBuildProgress(
	ctx context.Context, job *builder.Job, stage string, done, total int, log string,
) error {
	status, err := c.doJSON(ctx, http.MethodPost, "/builds/"+job.BuildID+"/progress", buildProgressRequest{
		WorkerID: c.workerID, LeaseToken: job.LeaseToken,
		Stage: stage, Done: done, Total: total, Log: log,
	}, nil)
	if err != nil {
		return err
	}
	switch {
	case status == http.StatusNoContent:
		return nil
	case status == http.StatusConflict:
		return scheduler.ErrLeaseLost
	default:
		return fmt.Errorf("build progress returned HTTP %d", status)
	}
}

// UploadBuildPackage stores the produced archive against the leased build.
func (c *Client) UploadBuildPackage(ctx context.Context, job *builder.Job, archive []byte) error {
	endpoint := c.baseURL + "/builds/" + job.BuildID + "/package?problemId=" + url.QueryEscape(job.ProblemID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(archive))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-Vertex-Worker-Id", c.workerID)
	request.Header.Set("X-Vertex-Lease-Token", job.LeaseToken)
	request.ContentLength = int64(len(archive))

	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return nil
	case response.StatusCode == http.StatusConflict:
		return scheduler.ErrLeaseLost
	default:
		return fmt.Errorf("package upload returned HTTP %d: %s", response.StatusCode, bytes.TrimSpace(body))
	}
}

// CompleteBuild posts the fenced terminal result.
func (c *Client) CompleteBuild(ctx context.Context, job *builder.Job, report *builder.Report, log string) error {
	request := buildResultRequest{
		WorkerID: c.workerID, LeaseToken: job.LeaseToken, Success: report.Success,
		Stage: report.Stage, Checker: report.Checker, Log: log,
		ErrorMessage: report.ErrorMessage, Tests: report.Tests, Solutions: report.Solutions,
	}
	delay := c.retryBase
	for {
		status, err := c.doJSON(ctx, http.MethodPut, "/builds/"+job.BuildID+"/result", request, nil)
		if err == nil {
			switch {
			case status == http.StatusNoContent:
				return nil
			case status == http.StatusConflict:
				return scheduler.ErrLeaseLost
			case status < http.StatusInternalServerError:
				return fmt.Errorf("build result returned HTTP %d", status)
			}
		}
		if waitErr := waitRetry(ctx, jitter(delay)); waitErr != nil {
			return waitErr
		}
		delay = min(delay*2, c.retryMax)
	}
}
