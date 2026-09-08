package postgres

import (
	"context"
	"database/sql"
	"errors"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/profile/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/profile/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

// Queries reads public practice data in the routed domain. Contest activity
// and unpublished/private problems never contribute to the public profile.
type Queries struct {
	db      *database.DB
	queries *dbgen.Queries
}

var _ domain.Queries = (*Queries)(nil)

func NewQueries(db *database.DB) *Queries { return &Queries{db: db, queries: dbgen.New(db.Pool.DB)} }

func (r *Queries) ByUsername(ctx context.Context, username string) (*domain.Profile, error) {
	if _, err := tenancypg.ResourceScope(ctx, r.db.Pool, tenancy.ActorID(ctx)); err != nil {
		return nil, err
	}
	account, err := r.queries.GetProfileAccount(ctx, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	domainID := tenancy.ID(ctx)
	stats, err := r.queries.GetProfileSubmissionStats(ctx, dbgen.GetProfileSubmissionStatsParams{UserID: account.ID, DomainID: domainID})
	if err != nil {
		return nil, err
	}
	difficulties, err := r.queries.ListProfileDifficultyStats(ctx, dbgen.ListProfileDifficultyStatsParams{UserID: account.ID, DomainID: domainID})
	if err != nil {
		return nil, err
	}
	activity, err := r.queries.ListProfileActivity(ctx, dbgen.ListProfileActivityParams{UserID: account.ID, DomainID: domainID, WindowDays: domain.ActivityWindowDays})
	if err != nil {
		return nil, err
	}
	result := &domain.Profile{UserID: account.ID, Username: account.Username, Role: account.Role, Rating: account.Rating, JoinedAt: account.CreatedAt,
		SolvedCount: stats.SolvedCount, AttemptedCount: stats.AttemptedCount, SubmissionCount: stats.SubmissionCount, AcceptedCount: stats.AcceptedCount,
		ByDifficulty: make([]domain.DifficultyProgress, 0, len(difficulties)), Activity: make([]domain.ActivityDay, 0, len(activity))}
	for _, row := range difficulties {
		result.ByDifficulty = append(result.ByDifficulty, domain.DifficultyProgress{Difficulty: row.Difficulty, Solved: row.Solved, Total: row.Total})
	}
	for _, row := range activity {
		result.Activity = append(result.Activity, domain.ActivityDay{Date: row.Date, Count: row.Count})
	}
	return result, nil
}
