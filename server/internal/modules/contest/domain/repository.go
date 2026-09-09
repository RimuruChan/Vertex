package domain

import "context"

type Repository interface {
	UseProblemVersion(ctx context.Context, contestID, problemID string, version, expected int) error
	Access(ctx context.Context, contestID, userID string) (Access, error)
	Grants(ctx context.Context, id string) ([]AccessGrant, error)
	SetGrant(ctx context.Context, id string, input GrantInput) error
	RemoveGrant(ctx context.Context, id string, grantID int64) error
	Transfer(ctx context.Context, id, username string) error
	Delete(ctx context.Context, id string) error
	Create(ctx context.Context, createdBy string, input *PersistInput) (*Contest, error)
	Update(ctx context.Context, id string, input *PersistInput) (*Contest, error)
	List(ctx context.Context, limit, offset int, keyword ...string) ([]Contest, int, error)
	ListAdmin(ctx context.Context, limit, offset int, keyword ...string) ([]Contest, int, error)
	Get(ctx context.Context, id string) (*Contest, error)
	Problems(ctx context.Context, contestID string) ([]Problem, error)
	ProblemStatuses(ctx context.Context, contestID, userID string) (map[string]ProblemProgress, error)
	Problem(ctx context.Context, contestID, problemID string) (*ProblemDetail, error)
	SetProblems(ctx context.Context, contestID string, entries []ProblemEntry) error
	IsParticipant(ctx context.Context, contestID, userID string) (bool, error)
	Register(ctx context.Context, contestID, userID string, verifiedPasswordHash ...string) error
	HasProblem(ctx context.Context, contestID, problemID string) (bool, error)
	Rankboard(ctx context.Context, contestID string, jury bool) (*Rankboard, error)
	StaffRole(ctx context.Context, contestID, userID string) (string, error)
	ListStaff(ctx context.Context, contestID string) ([]Staff, error)
	AddStaff(ctx context.Context, contestID, username, role string) (*Staff, error)
	RemoveStaff(ctx context.Context, contestID, userID string) error
}
