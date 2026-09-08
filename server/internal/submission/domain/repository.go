package domain

import (
	"context"

	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
)

type Repository interface {
	Create(ctx context.Context, submission *Submission) (*Submission, error)
	List(ctx context.Context, filters Filters, viewer Viewer) ([]Submission, int, error)
	Get(ctx context.Context, id string, viewer Viewer) (*Submission, error)
	Progress(ctx context.Context, id string, viewer Viewer) (*SubmissionProgress, error)
	Rejudge(ctx context.Context, id string) error
	CreateRejudging(ctx context.Context, selector RejudgeSelector, createdBy string) (*Rejudging, error)
	Rejudging(ctx context.Context, id string) (*Rejudging, error)
	ListRejudgings(ctx context.Context, contestID string, limit int) ([]Rejudging, error)
	CancelRejudging(ctx context.Context, id string) error
	RejudgingChanges(ctx context.Context, id string, limit int) ([]RejudgingChange, error)
}

type ProblemReader interface {
	Access(ctx context.Context, id, userID string) (problemdomain.Access, error)
}

type ContestValidator interface {
	ValidateSubmission(ctx context.Context, contestID, userID, role, problemID string) error
}

type Limiter interface {
	Allow(key string) bool
}
