package domain

import (
	"context"
	"time"
)

type Repository interface {
	Claim(ctx context.Context, workerID string, leaseTTL time.Duration) (*Job, error)
	Heartbeat(ctx context.Context, jobID string, generation int, leaseToken, workerID string, judgedCases int, leaseTTL time.Duration) error
	Complete(ctx context.Context, result Result) error
}
