package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	contestpg "github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	domain "github.com/RimuruChan/Vertex/server/internal/modules/submission/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/submission/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

type Queries struct {
	db      *database.DB
	queries *dbgen.Queries
}

func NewQueries(db *database.DB) *Queries { return &Queries{db: db, queries: dbgen.New(db.Pool.DB)} }

type readScope struct {
	at                         time.Time
	userID, domainID           string
	manager, member, canSubmit bool
}

func (r *Queries) resolve(ctx context.Context, viewer domain.Viewer) (readScope, error) {
	scope, err := tenancypg.ResourceScope(ctx, r.db.Pool, viewer.UserID)
	if err != nil {
		return readScope{}, err
	}
	scope.Domain.Archived = false
	return readScope{at: tenancy.ReadTime(ctx).Truncate(time.Microsecond), userID: viewer.UserID, domainID: scope.Domain.ID, manager: scope.Allows(tenancy.ManageResources), member: scope.ActiveMember(), canSubmit: scope.Allows(tenancy.CreateSubmission)}, nil
}

func contestNumber(contestID *string, number string) *string {
	if contestID == nil {
		return nil
	}
	return &number
}

func (r *Queries) sourceAccess(ctx context.Context, sub *domain.SubmissionRecord, viewer readScope) (bool, error) {
	if sub.UserID == viewer.userID || viewer.manager {
		return true, nil
	}
	if sub.ContestID != nil {
		access, err := contestpg.LoadAccess(ctx, r.db.Pool, *sub.ContestID, viewer.userID)
		if err != nil || access.Permissions.ViewJury {
			return access.Permissions.ViewJury, err
		}
		contest, err := contestpg.NewQueries(r.db).Get(ctx, *sub.ContestID)
		if err != nil {
			return false, err
		}
		return !sub.FrozenResult && contest.SourceCodeVisibility == "after_end" && contest.Ended(viewer.at) && !contest.Frozen(viewer.at), nil
	}
	access, err := problempg.LoadAccess(ctx, r.db.Pool, sub.ProblemID, viewer.userID)
	return access.Scope.ActiveMember() && access.OwnerID == viewer.userID, err
}

func (r *Queries) Get(ctx context.Context, id string, viewer domain.Viewer) (*domain.SubmissionRecord, error) {
	scope, err := r.resolve(ctx, viewer)
	if err != nil {
		return nil, err
	}
	row, err := r.queries.GetVisibleSubmission(ctx, dbgen.GetVisibleSubmissionParams{AsOf: scope.at, SubmissionID: id, DomainID: scope.domainID, ViewerID: scope.userID, IsManager: scope.manager, ActiveMember: scope.member, CanSubmit: scope.canSubmit})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	item := &domain.SubmissionRecord{AsOf: scope.at, Submission: domain.Submission{ID: row.ID, PublicID: row.PublicID, UserID: row.UserID, ProblemID: row.ProblemID,
		Language: row.Language,

		ContestID: row.ContestID, SubmittedAt: row.SubmittedAt,
		SourceCode: row.SourceCode}, FrozenResult: row.FrozenResult, Username: row.Username,
		ProblemPublicID: row.ProblemPublicID, ProblemTitle: row.ProblemTitle,

		ContestPublicID: contestNumber(row.ContestID, row.ContestPublicID), Judgement: domain.Judgement{Status: row.Status, Score: row.Score,
			TotalTimeMs: row.TotalTimeMs, PeakMemoryKb: row.PeakMemoryKb, JudgedCases: row.JudgedCases, TotalCases: row.TotalCases,

			ProblemVersion: row.ProblemVersion, CompileResult: row.CompileResult, JudgedAt: row.JudgedAt}}
	if err := json.Unmarshal(row.CaseResults, &item.CaseResults); err != nil {
		return nil, err
	}
	item.CanReadSource, err = r.sourceAccess(ctx, item, scope)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r *Queries) Progress(ctx context.Context, id string, viewer domain.Viewer) (*domain.SubmissionProgress, error) {
	scope, err := r.resolve(ctx, viewer)
	if err != nil {
		return nil, err
	}
	row, err := r.queries.GetVisibleProgress(ctx, dbgen.GetVisibleProgressParams{AsOf: scope.at, SubmissionID: id, DomainID: scope.domainID, ViewerID: scope.userID, IsManager: scope.manager, ActiveMember: scope.member, CanSubmit: scope.canSubmit})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	item := &domain.SubmissionProgress{AsOf: scope.at, PublicID: row.PublicID, FrozenResult: row.FrozenResult, ID: row.ID, UserID: row.UserID, ContestID: row.ContestID, Status: row.Status, Score: row.Score,
		TotalTimeMs: row.TotalTimeMs, PeakMemoryKb: row.PeakMemoryKb, CompileResult: row.CompileResult, JudgedCases: row.JudgedCases, TotalCases: row.TotalCases}
	if err := json.Unmarshal(row.CaseResults, &item.CaseResults); err != nil {
		return nil, err
	}
	// Polling never returns source. Only diagnostics require the additional
	// source policy lookup; empty diagnostics carry no source to disclose.
	if item.CompileResult != "" {
		item.CanReadSource, err = r.sourceAccess(ctx, &domain.SubmissionRecord{Submission: domain.Submission{UserID: row.UserID, ProblemID: row.ProblemID, ContestID: row.ContestID}, FrozenResult: row.FrozenResult}, scope)
		if err != nil {
			return nil, err
		}
	}
	return item, nil
}

func (r *Queries) List(ctx context.Context, f domain.Filters, viewer domain.Viewer) ([]domain.SubmissionRecord, int, error) {
	scope, err := r.resolve(ctx, viewer)
	if err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	total, err := r.queries.CountVisibleSubmissions(ctx, dbgen.CountVisibleSubmissionsParams{AsOf: scope.at, DomainID: scope.domainID, ViewerID: scope.userID,
		IsManager: scope.manager, ActiveMember: scope.member, CanSubmit: scope.canSubmit, UserFilter: f.UserID, ProblemFilter: f.ProblemID, ContestFilter: f.ContestID, LanguageFilter: f.Language, StatusFilter: f.Status})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListVisibleSubmissions(ctx, dbgen.ListVisibleSubmissionsParams{AsOf: scope.at, DomainID: scope.domainID, ViewerID: scope.userID,
		IsManager: scope.manager, ActiveMember: scope.member, CanSubmit: scope.canSubmit, UserFilter: f.UserID, ProblemFilter: f.ProblemID, ContestFilter: f.ContestID,
		LanguageFilter: f.Language, StatusFilter: f.Status, PageLimit: f.Limit, PageOffset: f.Offset})
	if err != nil {
		return nil, 0, err
	}
	items := make([]domain.SubmissionRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.SubmissionRecord{AsOf: scope.at, FrozenResult: row.FrozenResult, Submission: domain.Submission{ID: row.ID, PublicID: row.PublicID, UserID: row.UserID, ProblemID: row.ProblemID,
			Language: row.Language,

			ContestID: row.ContestID, SubmittedAt: row.SubmittedAt}, Username: row.Username,
			ProblemPublicID: row.ProblemPublicID, ProblemTitle: row.ProblemTitle,

			ContestPublicID: contestNumber(row.ContestID, row.ContestPublicID), Judgement: domain.Judgement{Status: row.Status, Score: row.Score,
				TotalTimeMs: row.TotalTimeMs, PeakMemoryKb: row.PeakMemoryKb, JudgedCases: row.JudgedCases, TotalCases: row.TotalCases,
				ProblemVersion: row.ProblemVersion}})
	}
	return items, int(total), nil
}
