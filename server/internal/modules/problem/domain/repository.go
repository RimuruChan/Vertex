package domain

import "context"

type Queries interface {
	Access(ctx context.Context, id, userID string) (Access, error)
	Grants(ctx context.Context, id string) ([]AccessGrant, error)
	List(ctx context.Context, filters Filters) ([]Problem, int, error)
	UserStatuses(ctx context.Context, viewerID string, problemIDs []string) (map[string]string, error)
	Tags(ctx context.Context) ([]Tag, error)
	Get(ctx context.Context, id string) (*Problem, error)
	GetWorkspace(ctx context.Context, id string) (*Problem, error)
}

type Repository interface {
	SetGrant(ctx context.Context, id string, input GrantInput) error
	RemoveGrant(ctx context.Context, id string, grantID int64) error
	Transfer(ctx context.Context, id, username string) error
	Create(ctx context.Context, authorID string, input *CreateInput) (*Problem, error)
	Update(ctx context.Context, id string, input *UpdateInput) (*Problem, error)
	Delete(ctx context.Context, id string) error
	SaveTestdata(ctx context.Context, problemID string, zipData []byte, checker string) (caseCount int, sha256 string, err error)
}

// TestdataArtifact identifies immutable candidate data on the artifact store.
type TestdataArtifact struct {
	CaseCount           int
	SHA256, StoragePath string
}

// ArtifactStorage owns archive materialization and file removal. PostgreSQL
// invokes it only while holding the aggregate's authorization lock.
type ArtifactStorage interface {
	Materialize(problemID string, archive []byte) (TestdataArtifact, error)
	RemoveProblem(problemID string) error
}
