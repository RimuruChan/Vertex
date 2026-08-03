package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/judge/internal/executor"
	"github.com/RimuruChan/Vertex/judge/internal/scheduler"
)

const maxErrorBody = 4096

type Client struct {
	baseURL      string
	token        string
	workerID     string
	testdataRoot string
	http         *http.Client
	waitSeconds  int
	retryBase    time.Duration
	retryMax     time.Duration
}

func New(baseURL, token, workerID, testdataRoot string, httpClient *http.Client, waitSeconds int) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || token == "" || workerID == "" || testdataRoot == "" || httpClient == nil {
		return nil, errors.New("judge API URL, token, worker ID, testdata root and HTTP client are required")
	}
	if waitSeconds <= 0 {
		waitSeconds = 25
	}
	return &Client{
		baseURL: baseURL, token: token, workerID: workerID, testdataRoot: filepath.Clean(testdataRoot),
		http: httpClient, waitSeconds: waitSeconds, retryBase: 250 * time.Millisecond, retryMax: 5 * time.Second,
	}, nil
}

type claimRequest struct {
	WorkerID     string   `json:"workerId"`
	WaitSeconds  int      `json:"waitSeconds"`
	Capabilities []string `json:"capabilities"`
}

type jobResponse struct {
	JobID          string           `json:"jobId"`
	SubmissionID   string           `json:"submissionId"`
	Generation     int              `json:"generation"`
	Attempt        int              `json:"attempt"`
	LeaseToken     string           `json:"leaseToken"`
	LeaseExpiresAt time.Time        `json:"leaseExpiresAt"`
	Language       string           `json:"language"`
	SourceCode     string           `json:"sourceCode"`
	ProblemID      string           `json:"problemId"`
	ContestID      *string          `json:"contestId"`
	TimeLimitMs    int              `json:"timeLimitMs"`
	MemoryLimitKB  int              `json:"memoryLimitKb"`
	Testdata       testdataResponse `json:"testdata"`
}

type testdataResponse struct {
	StoragePath string `json:"storagePath"`
	DataVersion int    `json:"dataVersion"`
	SHA256      string `json:"sha256"`
	CaseCount   int    `json:"caseCount"`
	Checker     string `json:"checker"`
}

func (c *Client) ClaimNext(ctx context.Context) (*scheduler.Submission, error) {
	delay := c.retryBase
	for {
		var response jobResponse
		status, err := c.doJSON(ctx, http.MethodPost, "/jobs/claim", claimRequest{
			WorkerID: c.workerID, WaitSeconds: c.waitSeconds, Capabilities: []string{"c", "cpp", "python"},
		}, &response)
		if err == nil {
			switch {
			case status == http.StatusNoContent:
				return nil, nil
			case status == http.StatusOK:
				dir, pathErr := c.testdataDir(response.Testdata.StoragePath)
				if pathErr != nil {
					return nil, pathErr
				}
				return &scheduler.Submission{
					JobID: response.JobID, ID: response.SubmissionID, Generation: response.Generation,
					Attempt: response.Attempt, LeaseToken: response.LeaseToken, LeaseUntil: response.LeaseExpiresAt,
					ProblemID: response.ProblemID, Language: response.Language, SourceCode: response.SourceCode,
					ContestID: response.ContestID,
					Limits:    scheduler.ProblemLimits{TimeLimitMs: response.TimeLimitMs, MemLimitKB: response.MemoryLimitKB},
					Testdata: scheduler.Testdata{
						Dir: dir, DataVersion: response.Testdata.DataVersion, SHA256: response.Testdata.SHA256,
						CaseCount: response.Testdata.CaseCount, Checker: response.Testdata.Checker,
					},
				}, nil
			case status < http.StatusInternalServerError:
				return nil, fmt.Errorf("claim returned HTTP %d", status)
			}
		}
		if err := waitRetry(ctx, jitter(delay)); err != nil {
			return nil, err
		}
		delay = min(delay*2, c.retryMax)
	}
}

type leaseRequest struct {
	WorkerID   string `json:"workerId"`
	Generation int    `json:"generation"`
	LeaseToken string `json:"leaseToken"`
}

func (c *Client) Heartbeat(ctx context.Context, sub *scheduler.Submission) error {
	delay := c.retryBase
	for {
		status, err := c.doJSON(ctx, http.MethodPost, "/jobs/"+sub.JobID+"/heartbeat", leaseRequest{
			WorkerID: c.workerID, Generation: sub.Generation, LeaseToken: sub.LeaseToken,
		}, nil)
		if err == nil {
			switch {
			case status == http.StatusNoContent:
				return nil
			case status == http.StatusConflict:
				return scheduler.ErrLeaseLost
			case status < http.StatusInternalServerError:
				return fmt.Errorf("heartbeat returned HTTP %d", status)
			}
		}
		if waitErr := waitRetry(ctx, jitter(delay)); waitErr != nil {
			return waitErr
		}
		delay = min(delay*2, c.retryMax)
	}
}

type caseResultRequest struct {
	CaseIndex     int    `json:"caseIndex"`
	Verdict       string `json:"verdict"`
	TimeMs        int    `json:"timeMs"`
	MemoryKB      int    `json:"memoryKb"`
	ExitStatus    string `json:"exitStatus,omitempty"`
	CheckerOutput string `json:"checkerOutput,omitempty"`
}

type resultRequest struct {
	WorkerID      string              `json:"workerId"`
	SubmissionID  string              `json:"submissionId"`
	Generation    int                 `json:"generation"`
	LeaseToken    string              `json:"leaseToken"`
	Status        string              `json:"status"`
	Score         int                 `json:"score"`
	TotalTimeMs   int64               `json:"totalTimeMs"`
	PeakMemoryKB  int                 `json:"peakMemoryKb"`
	CompileResult string              `json:"compileResult"`
	Cases         []caseResultRequest `json:"cases"`
}

func (c *Client) MarkResult(ctx context.Context, sub *scheduler.Submission, status string, score int,
	totalTime int64, peakMem int, compileResult string, cases []executor.CaseResult) error {

	request := resultRequest{
		WorkerID: c.workerID, SubmissionID: sub.ID, Generation: sub.Generation,
		LeaseToken: sub.LeaseToken, Status: status, Score: score, TotalTimeMs: totalTime,
		PeakMemoryKB: peakMem, CompileResult: compileResult,
		Cases: make([]caseResultRequest, 0, len(cases)),
	}
	for _, item := range cases {
		request.Cases = append(request.Cases, caseResultRequest{
			CaseIndex: item.CaseIndex, Verdict: item.Verdict, TimeMs: item.TimeMs,
			MemoryKB: item.MemoryKb, ExitStatus: item.ExitStatus, CheckerOutput: item.CheckerOutput,
		})
	}

	delay := c.retryBase
	for {
		statusCode, err := c.doJSON(ctx, http.MethodPut, "/jobs/"+sub.JobID+"/result", request, nil)
		if err == nil {
			switch {
			case statusCode == http.StatusNoContent:
				return nil
			case statusCode == http.StatusConflict:
				return scheduler.ErrLeaseLost
			case statusCode < 500:
				return fmt.Errorf("result returned HTTP %d", statusCode)
			}
		}
		if waitErr := waitRetry(ctx, jitter(delay)); waitErr != nil {
			return waitErr
		}
		delay = min(delay*2, c.retryMax)
	}
}

func waitRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func jitter(delay time.Duration) time.Duration {
	if delay <= 1 {
		return delay
	}
	return time.Duration(rand.Int64N(int64(delay)))
}

func (c *Client) doJSON(ctx context.Context, method, path string, input, output any) (int, error) {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if output != nil && response.StatusCode >= 200 && response.StatusCode < 300 && response.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(response.Body).Decode(output); err != nil {
			return response.StatusCode, err
		}
		return response.StatusCode, nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxErrorBody))
	return response.StatusCode, nil
}

func (c *Client) testdataDir(storagePath string) (string, error) {
	if storagePath == "" {
		return "", nil
	}
	path := storagePath
	if !filepath.IsAbs(path) {
		path = filepath.Join(c.testdataRoot, path)
	}
	path = filepath.Clean(path)
	relative, err := filepath.Rel(c.testdataRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("testdata path %q escapes configured root", storagePath)
	}
	return path, nil
}
