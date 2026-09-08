package postgres

import (
	"context"
	"database/sql"
	"errors"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"

	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	"github.com/jmoiron/sqlx"
)

// PackageRepository owns every editable part of a problem package. Each mutation
// bumps package_revision in the same transaction. Data-affecting changes also
// advance data_revision so statement edits need not force another build.
type PackageRepository struct {
	*PackageQueries
	db *database.DB
}

func NewPackageRepository(db *database.DB) *PackageRepository {
	return &PackageRepository{db: db, PackageQueries: NewPackageQueries(db)}
}

// bumpRevision marks the package dirty. Callers run it inside their own
// transaction so an edit and its revision move together.
func bumpRevision(ctx context.Context, tx *sqlx.Tx, problemID string, dataChanged bool) (int, error) {
	revision, err := dbgen.New(tx).BumpPackageRevision(ctx, dbgen.BumpPackageRevisionParams{ID: problemID, DomainID: tenancydomain.ID(ctx), DataChanged: dataChanged})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, authoringdomain.ErrNotFound
	}
	return revision, err
}

func (s *PackageRepository) withTx(ctx context.Context, problemID string, fn func(tx *sqlx.Tx) error) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Lock the parent before touching children, also for no-op reorders.
	access, err := problempg.LockAccess(ctx, tx, problemID, tenancydomain.ActorID(ctx))
	if err != nil {
		return packageReadError(err)
	}
	if !access.Permissions.Edit {
		return tenancydomain.ErrForbidden
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------- statements ----------

func (s *PackageRepository) Statements(ctx context.Context, problemID string) ([]authoringdomain.Statement, error) {
	if err := s.checkRead(ctx, problemID); err != nil {
		return nil, err
	}
	rows, err := dbgen.New(s.db.Pool).ListProblemStatements(ctx, dbgen.ListProblemStatementsParams{ProblemID: problemID, DomainID: tenancydomain.ID(ctx)})
	if err != nil {
		return nil, err
	}

	result := make([]authoringdomain.Statement, 0, len(rows))
	for _, row := range rows {
		result = append(result, statementFromRecord(row))
	}

	return result, nil
}

func (s *PackageRepository) SaveStatement(ctx context.Context, statement authoringdomain.Statement) (*authoringdomain.Statement, error) {
	var saved authoringdomain.Statement
	err := s.withTx(ctx, statement.ProblemID, func(tx *sqlx.Tx) error {
		if _, err := bumpRevision(ctx, tx, statement.ProblemID, false); err != nil {
			return err
		}
		row, err := dbgen.New(tx).UpsertProblemStatement(ctx, dbgen.UpsertProblemStatementParams{
			ProblemID: statement.ProblemID, Language: statement.Language, Name: statement.Name, Legend: statement.Legend,
			InputFormat: statement.InputFormat, OutputFormat: statement.OutputFormat, Notes: statement.Notes,
			Tutorial: statement.Tutorial, Scoring: statement.Scoring,
		})
		if err != nil {
			return err
		}
		saved = statementFromRecord(row)

		// Preview uses the selected candidate, without modifying the public row.
		samples, err := loadCandidateSamples(ctx, tx, statement.ProblemID)
		if err != nil {
			return err
		}
		return renderWorkspaceStatement(ctx, tx, statement.ProblemID, samples)
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (s *PackageRepository) DeleteStatement(ctx context.Context, problemID, language string) error {
	return s.withTx(ctx, problemID, func(tx *sqlx.Tx) error {
		affected, err := dbgen.New(tx).DeleteProblemStatement(ctx, dbgen.DeleteProblemStatementParams{ProblemID: problemID, Language: language})
		if err != nil {
			return err
		}
		if affected == 0 {
			return authoringdomain.ErrNotFound
		}
		if err := dbgen.New(tx).ClearDeletedWorkspaceStatement(ctx, dbgen.ClearDeletedWorkspaceStatementParams{ProblemID: problemID, StatementLanguage: language}); err != nil {
			return err
		}
		_, err = bumpRevision(ctx, tx, problemID, false)
		return err
	})
}

// ---------- files ----------

// SaveFile upserts by (problem_id, kind, name). Activating a file clears the
// previous active one of the same kind first, so the partial unique index can
// never see two winners.
func (s *PackageRepository) SaveFile(ctx context.Context, file authoringdomain.File) (*authoringdomain.File, error) {
	var saved authoringdomain.File
	err := s.withTx(ctx, file.ProblemID, func(tx *sqlx.Tx) error {
		if _, err := bumpRevision(ctx, tx, file.ProblemID, true); err != nil {
			return err
		}
		if file.IsActive {
			if err := dbgen.New(tx).DeactivateOtherProblemFiles(ctx, dbgen.DeactivateOtherProblemFilesParams{ProblemID: file.ProblemID, Kind: file.Kind, Name: file.Name}); err != nil {
				return err
			}
		}
		row, err := dbgen.New(tx).UpsertProblemFile(ctx, dbgen.UpsertProblemFileParams{ProblemID: file.ProblemID, Kind: file.Kind, Name: file.Name, Language: file.Language, SourceCode: file.SourceCode, ExpectedVerdict: file.ExpectedVerdict, IsActive: file.IsActive})
		if err == nil {
			saved = fileFromRecord(row)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (s *PackageRepository) DeleteFile(ctx context.Context, problemID string, id int64) error {
	return s.withTx(ctx, problemID, func(tx *sqlx.Tx) error {
		affected, err := dbgen.New(tx).DeleteProblemFile(ctx, dbgen.DeleteProblemFileParams{ProblemID: problemID, ID: id})
		if err != nil {
			return err
		}
		if affected == 0 {
			return authoringdomain.ErrNotFound
		}
		_, err = bumpRevision(ctx, tx, problemID, true)
		return err
	})
}

// ---------- tests ----------

// CreateTest appends a test at the end of the current plan.
func (s *PackageRepository) CreateTest(ctx context.Context, test authoringdomain.Test) (*authoringdomain.Test, error) {
	var saved authoringdomain.Test
	err := s.withTx(ctx, test.ProblemID, func(tx *sqlx.Tx) error {
		if _, err := bumpRevision(ctx, tx, test.ProblemID, true); err != nil {
			return err
		}
		row, err := dbgen.New(tx).CreateProblemTest(ctx, dbgen.CreateProblemTestParams{ProblemID: test.ProblemID, GroupName: test.Group, Source: test.Source, InputData: test.InputData, GenerateCmd: test.GenerateCmd, IsSample: test.IsSample, Points: test.Points, Description: test.Description})
		if err == nil {
			saved = testFromRecord(row)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (s *PackageRepository) UpdateTest(ctx context.Context, test authoringdomain.Test) (*authoringdomain.Test, error) {
	var saved authoringdomain.Test
	err := s.withTx(ctx, test.ProblemID, func(tx *sqlx.Tx) error {
		if _, err := bumpRevision(ctx, tx, test.ProblemID, true); err != nil {
			return err
		}
		row, err := dbgen.New(tx).UpdateProblemTest(ctx, dbgen.UpdateProblemTestParams{ProblemID: test.ProblemID, ID: test.ID, GroupName: test.Group, Source: test.Source, InputData: test.InputData, GenerateCmd: test.GenerateCmd, IsSample: test.IsSample, Points: test.Points, Description: test.Description})
		if err == nil {
			saved = testFromRecord(row)
		}
		if errors.Is(err, sql.ErrNoRows) {
			return authoringdomain.ErrNotFound
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

// DeleteTest removes a test and closes the gap so indexes stay 1..N, which is
// the contract the judge testdata layout depends on.
func (s *PackageRepository) DeleteTest(ctx context.Context, problemID string, id int64) error {
	return s.withTx(ctx, problemID, func(tx *sqlx.Tx) error {
		removedIndex, err := dbgen.New(tx).DeleteProblemTest(ctx, dbgen.DeleteProblemTestParams{ProblemID: problemID, ID: id})
		if errors.Is(err, sql.ErrNoRows) {
			return authoringdomain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := dbgen.New(tx).CloseTestOrderGap(ctx, dbgen.CloseTestOrderGapParams{ProblemID: problemID, TestIndex: removedIndex}); err != nil {
			return err
		}
		_, err = bumpRevision(ctx, tx, problemID, true)
		return err
	})
}

// ReorderTest parks the moved test after the last index, shifts its neighbors,
// then installs its final position under the package lock.
func (s *PackageRepository) ReorderTest(ctx context.Context, problemID string, id int64, target int) error {
	return s.withTx(ctx, problemID, func(tx *sqlx.Tx) error {
		bounds, err := dbgen.New(tx).GetTestOrderBounds(ctx, dbgen.GetTestOrderBoundsParams{ProblemID: problemID, ID: id})
		if errors.Is(err, sql.ErrNoRows) {
			return authoringdomain.ErrNotFound
		}
		if err != nil {
			return err
		}
		current, total := bounds.TestIndex, bounds.TotalTests
		if target < 1 {
			target = 1
		}
		if target > total {
			target = total
		}
		if target == current {
			return nil
		}
		// Park the moved row above every existing index while the others
		// shift. A negative placeholder would be simpler but violates the
		// test_index > 0 constraint.
		if err := dbgen.New(tx).ParkTestForReorder(ctx, dbgen.ParkTestForReorderParams{ProblemID: problemID, ID: id}); err != nil {
			return err
		}
		if target < current {
			err = dbgen.New(tx).ShiftTestsRight(ctx, dbgen.ShiftTestsRightParams{ProblemID: problemID, RangeStart: target, RangeEnd: current})
		} else {
			err = dbgen.New(tx).ShiftTestsLeft(ctx, dbgen.ShiftTestsLeftParams{ProblemID: problemID, RangeStart: current, RangeEnd: target})
		}
		if err != nil {
			return err
		}
		if err := dbgen.New(tx).SetTestOrder(ctx, dbgen.SetTestOrderParams{ProblemID: problemID, ID: id, TestIndex: target}); err != nil {
			return err
		}
		_, err = bumpRevision(ctx, tx, problemID, true)
		return err
	})
}

// TouchRevision marks the package dirty for edits that live on the problems
// row itself (limits, judge type) rather than in a package table.
func (s *PackageRepository) TouchRevision(ctx context.Context, problemID string) error {
	return s.withTx(ctx, problemID, func(tx *sqlx.Tx) error {
		_, err := bumpRevision(ctx, tx, problemID, true)
		return err
	})
}

var _ authoringdomain.PackageRepository = (*PackageRepository)(nil)
