package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	domain "github.com/RimuruChan/Vertex/server/internal/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/authoring/infrastructure/postgres/internal/dbgen"
)

// renderWorkspaceStatement updates only the working projection in the caller's
// transaction. Publication chooses and snapshots its statement separately.
func renderWorkspaceStatement(ctx context.Context, tx dbgen.DBTX, problemID string, tests []domain.TestOutcome) error {
	q := dbgen.New(tx)
	language, err := q.GetWorkspaceStatementLanguage(ctx, problemID)
	if err != nil {
		return err
	}
	statement, err := loadStatement(ctx, tx, problemID, language)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return q.SaveRenderedStatement(ctx, dbgen.SaveRenderedStatementParams{
		ProblemID: problemID, StatementName: statement.Name,
		StatementMd: domain.RenderStatement(*statement, domain.SamplesFromOutcomes(tests)),
	})
}

func loadStatement(ctx context.Context, conn dbgen.DBTX, problemID, language string) (*domain.Statement, error) {
	row, err := dbgen.New(conn).GetProblemStatement(ctx, dbgen.GetProblemStatementParams{ProblemID: problemID, Language: language})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	item := statementFromRecord(row)
	return &item, nil
}

// loadCandidateSamples provides preview examples without publishing them.
func loadCandidateSamples(ctx context.Context, conn dbgen.DBTX, problemID string) ([]domain.TestOutcome, error) {
	payload, err := dbgen.New(conn).GetBuiltSamples(ctx, problemID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var tests []domain.TestOutcome
	if err := json.Unmarshal(payload, &tests); err != nil {
		return nil, nil
	}
	return tests, nil
}

func statementFromRecord(row dbgen.ProblemStatement) domain.Statement {
	return domain.Statement{ProblemID: row.ProblemID, Language: row.Language, Name: row.Name,
		Legend: row.Legend, InputFormat: row.InputFormat, OutputFormat: row.OutputFormat,
		Notes: row.Notes, Tutorial: row.Tutorial, Scoring: row.Scoring, UpdatedAt: row.UpdatedAt}
}
