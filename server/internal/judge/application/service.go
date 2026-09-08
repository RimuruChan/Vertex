package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	judgedomain "github.com/RimuruChan/Vertex/server/internal/judge/domain"
)

var ErrInvalidWorker = errors.New("invalid worker ID")

type Service struct {
	repository judgedomain.Repository
	dispatcher *Dispatcher
	leaseTTL   time.Duration
	maxWait    time.Duration
}

func NewService(repository judgedomain.Repository, dispatcher *Dispatcher, leaseTTL, maxWait time.Duration) (*Service, error) {
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

func (s *Service) Claim(ctx context.Context, workerID string, wait time.Duration) (*judgedomain.Job, error) {
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

func (s *Service) Heartbeat(ctx context.Context, jobID string, generation int, leaseToken, workerID string, judgedCases int) error {
	if jobID == "" || generation <= 0 || leaseToken == "" || workerID == "" {
		return judgedomain.ErrStaleLease
	}
	// Progress is advisory display data; clamp rather than reject so a bad
	// number can never cost a worker its lease.
	if judgedCases < 0 {
		judgedCases = 0
	}
	if judgedCases > judgedomain.MaxResultCases {
		judgedCases = judgedomain.MaxResultCases
	}
	return s.repository.Heartbeat(ctx, jobID, generation, leaseToken, workerID, judgedCases, s.leaseTTL)
}

func (s *Service) Complete(ctx context.Context, result judgedomain.Result) error {
	if result.JobID == "" || result.SubmissionID == "" || result.Generation <= 0 || result.LeaseToken == "" || result.WorkerID == "" {
		return judgedomain.ErrStaleLease
	}
	if err := judgedomain.ValidateResult(result); err != nil {
		return err
	}
	return s.repository.Complete(ctx, result)
}
