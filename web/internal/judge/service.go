package judge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const MaxResultCases = 10000

var ErrInvalidWorker = errors.New("invalid worker ID")

type Repository interface {
	Claim(ctx context.Context, workerID string, leaseTTL time.Duration) (*Job, error)
	Heartbeat(ctx context.Context, jobID string, generation int, leaseToken, workerID string, leaseTTL time.Duration) error
	Complete(ctx context.Context, result Result) error
}

// Dispatcher wakes one long-poll waiter for each newly queued job. Database
// state remains authoritative; the periodic fallback covers lost signals.
type Dispatcher struct{ wake chan struct{} }

func NewDispatcher(capacity int) *Dispatcher {
	if capacity < 1 {
		capacity = 1
	}
	return &Dispatcher{wake: make(chan struct{}, capacity)}
}

func (d *Dispatcher) Notify() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// RunFallback emits one bounded wake-up per interval for the whole Web
// instance. Waiters never run independent database polling loops.
func (d *Dispatcher) RunFallback(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.Notify()
		}
	}
}

type Service struct {
	repository Repository
	dispatcher *Dispatcher
	leaseTTL   time.Duration
	maxWait    time.Duration
}

func NewService(repository Repository, dispatcher *Dispatcher, leaseTTL, maxWait time.Duration) (*Service, error) {
	if repository == nil || dispatcher == nil {
		return nil, errors.New("judge dependencies must not be nil")
	}
	if leaseTTL <= 0 || maxWait <= 0 || leaseTTL <= maxWait {
		return nil, errors.New("judge lease TTL must be greater than the positive long-poll timeout")
	}
	return &Service{
		repository: repository, dispatcher: dispatcher, leaseTTL: leaseTTL,
		maxWait: maxWait,
	}, nil
}

func (s *Service) Claim(ctx context.Context, workerID string, wait time.Duration) (*Job, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || len(workerID) > 128 {
		return nil, fmt.Errorf("%w: must contain 1-128 characters", ErrInvalidWorker)
	}
	if wait <= 0 || wait > s.maxWait {
		wait = s.maxWait
	}
	deadline := time.Now().Add(wait)
	for {
		job, err := s.repository.Claim(ctx, workerID, s.leaseTTL)
		if err != nil {
			return job, err
		}
		if job != nil {
			// At most one additional waiter checks for another queued job.
			s.dispatcher.Notify()
			return job, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, nil
		}
		timer := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-s.dispatcher.wake:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func (s *Service) Heartbeat(ctx context.Context, jobID string, generation int, leaseToken, workerID string) error {
	if jobID == "" || generation <= 0 || leaseToken == "" || workerID == "" {
		return ErrStaleLease
	}
	return s.repository.Heartbeat(ctx, jobID, generation, leaseToken, workerID, s.leaseTTL)
}

func (s *Service) Complete(ctx context.Context, result Result) error {
	if result.JobID == "" || result.SubmissionID == "" || result.Generation <= 0 || result.LeaseToken == "" || result.WorkerID == "" {
		return ErrStaleLease
	}
	if err := validateResult(result); err != nil {
		return err
	}
	return s.repository.Complete(ctx, result)
}

func validateResult(result Result) error {
	if !validVerdict(result.Status) {
		return fmt.Errorf("%w: unsupported status %q", ErrInvalidResult, result.Status)
	}
	if result.Score < 0 || result.Score > 100 || result.TotalTimeMs < 0 || result.PeakMemoryKB < 0 {
		return fmt.Errorf("%w: negative resource usage or score outside 0-100", ErrInvalidResult)
	}
	if len(result.Cases) > MaxResultCases {
		return fmt.Errorf("%w: case result count exceeds %d", ErrInvalidResult, MaxResultCases)
	}
	caseIndexes := make(map[int]struct{}, len(result.Cases))
	for _, item := range result.Cases {
		if item.CaseIndex <= 0 || item.TimeMs < 0 || item.MemoryKB < 0 || !validVerdict(item.Verdict) {
			return fmt.Errorf("%w: invalid case result", ErrInvalidResult)
		}
		if _, duplicate := caseIndexes[item.CaseIndex]; duplicate {
			return fmt.Errorf("%w: duplicate case index %d", ErrInvalidResult, item.CaseIndex)
		}
		caseIndexes[item.CaseIndex] = struct{}{}
	}
	return nil
}

func validVerdict(status string) bool {
	switch status {
	case "Accepted", "Wrong Answer", "Time Limit Exceeded", "Memory Limit Exceeded",
		"Runtime Error", "Compile Error", "Output Limit Exceeded", "System Error", "Skipped":
		return true
	default:
		return false
	}
}
