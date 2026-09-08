package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres/internal/dbgen"
	publicid "github.com/RimuruChan/Vertex/server/internal/modules/publicid/domain"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
)

func (r *Repository) Problems(ctx context.Context, contestID string) ([]domain.Problem, error) {
	rows, err := r.queries.ListContestProblems(ctx, dbgen.ListContestProblemsParams{ContestID: contestID, DomainID: tenancy.ID(ctx)})
	if err != nil {
		return nil, err
	}
	items := make([]domain.Problem, 0, len(rows))
	for _, row := range rows {
		item := domain.Problem{ContestID: row.ContestID, ContestPublicID: row.ContestPublicID, ProblemID: row.ProblemID, ProblemPublicID: row.ProblemPublicID,
			SortOrder: row.SortOrder, Label: row.Label, Color: row.Color, Points: row.Points, Title: row.Title, Difficulty: row.Difficulty, Visibility: row.Visibility, Version: row.ProblemVersion}
		if err := json.Unmarshal(row.TagsJson, &item.Tags); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// Problem resolves a contest-local reference against the pinned release.
func (r *Repository) Problem(ctx context.Context, contestID, ref string) (*domain.ProblemDetail, error) {
	var row dbgen.GetContestProblemByIDRow
	var err error
	if publicid.IsNumber(ref) {
		number, e := strconv.ParseInt(ref, 10, 64)
		if e != nil || number <= 0 {
			return nil, domain.ErrProblemNotInContest
		}
		found, e := r.queries.GetContestProblemByNumber(ctx, dbgen.GetContestProblemByNumberParams{ContestID: contestID, DomainID: tenancy.ID(ctx), PublicID: number})
		row, err = dbgen.GetContestProblemByIDRow(found), e
	} else if len(ref) <= 8 {
		found, e := r.queries.GetContestProblemByLabel(ctx, dbgen.GetContestProblemByLabelParams{ContestID: contestID, DomainID: tenancy.ID(ctx), Label: ref})
		row, err = dbgen.GetContestProblemByIDRow(found), e
	} else {
		row, err = r.queries.GetContestProblemByID(ctx, dbgen.GetContestProblemByIDParams{ContestID: contestID, DomainID: tenancy.ID(ctx), ProblemID: ref})
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrProblemNotInContest
	}
	if err != nil {
		return nil, err
	}
	item := &domain.ProblemDetail{Problem: domain.Problem{ContestID: row.ContestID, ContestPublicID: row.ContestPublicID, ProblemID: row.ProblemID,
		ProblemPublicID: row.ProblemPublicID, SortOrder: row.SortOrder, Label: row.Label, Color: row.Color, Points: row.Points,
		Title: row.Title, Difficulty: row.Difficulty, Visibility: row.Visibility, Version: row.ProblemVersion},
		StatementMD: row.StatementMd, Source: row.Source, TimeLimitMs: row.TimeLimitMs, MemoryLimitKB: row.MemoryLimitKb, JudgeType: row.JudgeType}
	if err := json.Unmarshal(row.TagsJson, &item.Tags); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *Repository) IsParticipant(ctx context.Context, contestID, userID string) (bool, error) {
	return r.queries.IsContestParticipant(ctx, dbgen.IsContestParticipantParams{ContestID: contestID, UserID: userID, DomainID: tenancy.ID(ctx)})
}

func (r *Repository) HasProblem(ctx context.Context, contestID, problemID string) (bool, error) {
	return r.queries.HasContestProblem(ctx, dbgen.HasContestProblemParams{ContestID: contestID, ProblemID: problemID, DomainID: tenancy.ID(ctx)})
}
