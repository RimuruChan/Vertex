package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/builder"
	"github.com/RimuruChan/Vertex/worker/internal/scheduler"
)

// ---------- wire types ----------

type buildClaimRequest struct {
	CheckProtocol string `json:"checkProtocol"`
	WorkerID      string `json:"workerId"`
	WaitSeconds   int    `json:"waitSeconds"`
}

type buildLimits struct {
	GeneratorTimeMs int `json:"generatorTimeMs"`
	ValidatorTimeMs int `json:"validatorTimeMs"`
	SolutionTimeMs  int `json:"solutionTimeMs"`
	CheckerTimeMs   int `json:"checkerTimeMs"`
	MemoryLimitKB   int `json:"memoryLimitKb"`
}

type buildJobResponse struct {
	Check          *builder.FrozenSnapshot `json:"check"`
	DomainID       string                  `json:"domainId"`
	BuildID        string                  `json:"buildId"`
	ProblemID      string                  `json:"problemId"`
	Attempt        int                     `json:"attempt"`
	LeaseToken     string                  `json:"leaseToken"`
	LeaseExpiresAt time.Time               `json:"leaseExpiresAt"`
	Limits         buildLimits             `json:"limits"`
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
	ToolchainKey string                      `json:"toolchainKey,omitempty"`
	WorkerID     string                      `json:"workerId"`
	LeaseToken   string                      `json:"leaseToken"`
	Success      bool                        `json:"success"`
	Stage        string                      `json:"stage,omitempty"`
	Checker      string                      `json:"checker,omitempty"`
	Log          string                      `json:"log,omitempty"`
	ErrorMessage string                      `json:"errorMessage,omitempty"`
	Tests        []builder.TestOutcome       `json:"tests,omitempty"`
	Solutions    []builder.SolutionOutcome   `json:"solutions,omitempty"`
	Validation   []builder.ValidationOutcome `json:"validation,omitempty"`
}

// ---------- protocol ----------

// ClaimBuild long-polls for one package build job. A nil job means the poll
// expired with nothing queued.
func (c *Client) ClaimBuild(ctx context.Context) (*builder.Job, error) {
	delay := c.retryBase
	for {
		var response buildJobResponse
		status, err := c.doJSON(ctx, http.MethodPost, "/builds/claim", buildClaimRequest{
			CheckProtocol: builder.CheckProtocol,
			WorkerID:      c.workerID, WaitSeconds: c.waitSeconds,
		}, &response)
		if err == nil {
			switch {
			case status == http.StatusNoContent:
				return nil, nil
			case status == http.StatusOK:
				if strings.TrimSpace(response.DomainID) == "" || response.Check == nil || response.Check.SchemaVersion != 1 || response.Check.PolicyVersion != builder.CheckProtocol {
					return nil, fmt.Errorf("invalid build snapshot: domainId and the current frozen check snapshot are required")
				}
				job := buildJobFromResponse(response)
				job.FetchContent = func(ctx context.Context, ref builder.BlobRef) (io.ReadCloser, error) {
					return c.buildContent(ctx, job, ref)
				}
				return job, nil
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
	return &builder.Job{Check: response.Check, DomainID: response.DomainID, BuildID: response.BuildID, ProblemID: response.ProblemID,
		Attempt: response.Attempt, LeaseToken: response.LeaseToken, LeaseExpires: response.LeaseExpiresAt,
		Limits: builder.Limits{GeneratorTimeMs: response.Limits.GeneratorTimeMs, ValidatorTimeMs: response.Limits.ValidatorTimeMs, SolutionTimeMs: response.Limits.SolutionTimeMs, CheckerTimeMs: response.Limits.CheckerTimeMs, MemoryLimitKB: response.Limits.MemoryLimitKB}}
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
		ToolchainKey: report.ToolchainKey,
		WorkerID:     c.workerID, LeaseToken: job.LeaseToken, Success: report.Success,
		Stage: report.Stage, Checker: report.Checker, Log: log,
		ErrorMessage: report.ErrorMessage, Tests: report.Tests, Solutions: report.Solutions, Validation: report.Validation,
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

func (c *Client) buildContent(ctx context.Context, job *builder.Job, ref builder.BlobRef) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/builds/"+url.PathEscape(job.BuildID)+"/content/"+url.PathEscape(ref.SHA256), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("X-Vertex-Worker-Id", c.workerID)
	request.Header.Set("X-Vertex-Lease-Token", job.LeaseToken)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		if response.StatusCode == http.StatusConflict {
			return nil, scheduler.ErrLeaseLost
		}
		return nil, fmt.Errorf("build content download returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength >= 0 && response.ContentLength != ref.Bytes {
		response.Body.Close()
		return nil, fmt.Errorf("build content length does not match snapshot")
	}
	return response.Body, nil
}
