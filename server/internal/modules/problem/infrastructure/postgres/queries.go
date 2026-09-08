package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

// Queries implements the problem read models; all SQL stays in private dbgen.
type Queries struct {
	db      *database.DB
	queries *dbgen.Queries
}

var _ domain.Queries = (*Queries)(nil)

func NewQueries(db *database.DB) *Queries { return &Queries{db: db, queries: dbgen.New(db.Pool.DB)} }

func problemFromRow(row dbgen.GetProblemRow) (domain.Problem, error) {
	p := domain.Problem{ID: row.ID, PublicID: row.PublicID, Title: row.Title, StatementMD: row.StatementMd, Difficulty: row.Difficulty, Source: row.Source,
		TimeLimitMs: row.TimeLimitMs, MemoryLimitKb: row.MemoryLimitKb, Visibility: row.Visibility, AuthorID: row.AuthorID,
		SubmissionCount: row.SubmissionCount, AcceptedCount: row.AcceptedCount, SolvedUserCount: row.SolvedUserCount, JudgeType: row.JudgeType,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, OwnerID: row.OwnerID, DomainID: row.DomainID, OwnerName: row.OwnerName, PublishedVersion: row.PublishedVersion}
	if err := json.Unmarshal(row.Tags, &p.Tags); err != nil {
		return domain.Problem{}, err
	}
	return p, nil
}

func listedProblem(row dbgen.GetProblemRow, scope tenancy.Scope) (domain.Problem, error) {
	p, err := problemFromRow(row)
	if err != nil {
		return domain.Problem{}, err
	}
	p.Permissions = domain.EffectivePermissions(scope, p.OwnerID, p.Visibility, rankRole(row.GrantRank))
	if p.PublishedVersion == 0 && !p.Permissions.ReadPackage {
		p.Permissions.View = false
	}
	return p, nil
}

func (r *Queries) List(ctx context.Context, f domain.Filters) ([]domain.Problem, int, error) {
	scope, err := tenancypg.ResourceScope(ctx, r.db.Pool, f.ViewerID)
	if err != nil {
		return nil, 0, accessError(err)
	}
	readScope := scope
	readScope.Domain.Archived = false
	manager := readScope.Allows(tenancy.ManageResources)
	if f.Workspace && !manager && !scope.ActiveMember() {
		return []domain.Problem{}, 0, nil
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	if !domain.IsUserStatus(f.Status) {
		f.Status = ""
	}
	countArgs := dbgen.CountPublicProblemsParams{ViewerID: scope.UserID, DomainID: scope.Domain.ID, IsManager: manager, ActiveMember: scope.ActiveMember(), Visibility: f.Visibility, Difficulty: f.Difficulty, Keyword: f.Keyword, Tag: f.Tag, Status: f.Status}
	listArgs := dbgen.ListPublicProblemsParams{ViewerID: scope.UserID, DomainID: scope.Domain.ID, IsManager: manager, ActiveMember: scope.ActiveMember(), Visibility: f.Visibility, Difficulty: f.Difficulty, Keyword: f.Keyword, Tag: f.Tag, Status: f.Status, PageLimit: f.Limit, PageOffset: f.Offset}
	if f.Workspace {
		total, err := r.queries.CountWorkspaceProblems(ctx, dbgen.CountWorkspaceProblemsParams(countArgs))
		if err != nil {
			return nil, 0, err
		}
		rows, err := r.queries.ListWorkspaceProblems(ctx, dbgen.ListWorkspaceProblemsParams(listArgs))
		if err != nil {
			return nil, 0, err
		}
		list := make([]domain.Problem, 0, len(rows))
		for _, row := range rows {
			p, err := listedProblem(dbgen.GetProblemRow(row), scope)
			if err != nil {
				return nil, 0, err
			}
			list = append(list, p)
		}
		return list, int(total), nil
	}
	total, err := r.queries.CountPublicProblems(ctx, countArgs)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListPublicProblems(ctx, listArgs)
	if err != nil {
		return nil, 0, err
	}
	list := make([]domain.Problem, 0, len(rows))
	for _, row := range rows {
		p, err := listedProblem(dbgen.GetProblemRow(row), scope)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, p)
	}
	return list, int(total), nil
}

func (r *Queries) Get(ctx context.Context, id string) (*domain.Problem, error) {
	row, err := r.queries.GetProblem(ctx, dbgen.GetProblemParams{ProblemID: id, DomainID: tenancy.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p, err := problemFromRow(row)
	if err != nil {
		return nil, err
	}
	// Preserve the untagged detail model's nil slice; HTTP DTOs normalize it.
	if len(p.Tags) == 0 {
		p.Tags = nil
	}
	return &p, nil
}

func (r *Queries) GetWorkspace(ctx context.Context, id string) (*domain.Problem, error) {
	row, err := r.queries.GetProblemWorkspace(ctx, dbgen.GetProblemWorkspaceParams{ProblemID: id, DomainID: tenancy.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p, err := problemFromRow(dbgen.GetProblemRow(row))
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Queries) Testdata(ctx context.Context, problemID string) (*domain.TestdataInfo, error) {
	row, err := r.queries.GetPublishedTestdata(ctx, dbgen.GetPublishedTestdataParams{ProblemID: problemID, DomainID: tenancy.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	result := &domain.TestdataInfo{ProblemID: row.ProblemID, DataVersion: row.ArtifactVersion, StoragePath: row.TestdataPath, SHA256: row.Sha256, CaseCount: row.CaseCount, Checker: row.Checker, SPJSource: row.SpjSource}
	if err := json.Unmarshal(row.ConfigJson, &result.Config); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Queries) UserStatuses(ctx context.Context, viewerID string, ids []string) (map[string]string, error) {
	statuses := make(map[string]string, len(ids))
	if viewerID == "" || len(ids) == 0 {
		return statuses, nil
	}
	rows, err := r.queries.ListUserProblemStatuses(ctx, dbgen.ListUserProblemStatusesParams{ViewerID: viewerID, ProblemIds: ids, DomainID: tenancy.ID(ctx)})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		statuses[row.ProblemID] = domain.UserStatusAttempted
		if row.Solved {
			statuses[row.ProblemID] = domain.UserStatusSolved
		}
	}
	return statuses, nil
}

func (r *Queries) Tags(ctx context.Context) ([]domain.Tag, error) {
	if _, err := tenancypg.ResourceScope(ctx, r.db.Pool, tenancy.ActorID(ctx)); err != nil {
		return nil, accessError(err)
	}
	rows, err := r.queries.ListPublicProblemTags(ctx, tenancy.ID(ctx))
	if err != nil {
		return nil, err
	}
	result := make([]domain.Tag, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.Tag{Name: row.Name, ProblemCount: row.ProblemCount})
	}
	return result, nil
}
