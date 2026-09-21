package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
)

func (repo *RevisionRepository) StartCheck(ctx context.Context, id string, selection domain.CheckSelection) (*domain.CheckRun, error) {
	if err := selection.Validate(); err != nil {
		return nil, err
	}
	var result *domain.CheckRun
	err := repo.transaction(ctx, id, true, func(q *dbgen.Queries, actor string) error {
		tree, _, err := selectCheckTree(ctx, q, id, actor, selection.Revision, selection.ETag)
		if err != nil {
			return err
		}
		report, snapshot, err := repo.inspectTree(ctx, id, tree)
		if err != nil {
			return err
		}
		if !report.CanBuild {
			return domain.ErrNotBuildable
		}
		active, err := q.FindActiveCheck(ctx, dbgen.FindActiveCheckParams{ProblemID: id, TreeHash: snapshot.TreeHash, ActorID: &actor})
		if err == nil {
			result, err = readCheck(ctx, q, id, actor, active)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		pkg := domain.CheckInput{DomainID: tenancy.ID(ctx), ProblemID: id, Check: snapshot}
		input, err := json.Marshal(pkg)
		if err != nil {
			return err
		}
		sourceRevision := sql.NullInt64{Int64: selection.Revision, Valid: selection.Revision > 0}
		buildID, err := q.EnqueueMaterialCheck(ctx, dbgen.EnqueueMaterialCheckParams{ProblemID: id, TreeHash: snapshot.TreeHash, SourceRevision: sourceRevision, InputJson: input, DataHash: snapshot.DataHash, CheckPolicy: snapshot.PolicyVersion, ActorID: &actor})
		if err != nil {
			return err
		}
		if err := q.NotifyBuildJob(ctx, buildID); err != nil {
			return err
		}
		result, err = readCheck(ctx, q, id, actor, buildID)
		return err
	})
	return result, err
}

func (repo *RevisionRepository) Check(ctx context.Context, id, checkID string) (*domain.CheckRun, error) {
	var result *domain.CheckRun
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, actor string) error {
		var err error
		result, err = readCheck(ctx, q, id, actor, checkID)
		return err
	})
	return result, err
}

func (repo *RevisionRepository) CancelCheck(ctx context.Context, id, checkID string) (*domain.CheckRun, error) {
	var result *domain.CheckRun
	err := repo.transaction(ctx, id, true, func(q *dbgen.Queries, actor string) error {
		if _, err := readCheck(ctx, q, id, actor, checkID); err != nil {
			return err
		}
		if err := q.CancelMaterialCheck(ctx, dbgen.CancelMaterialCheckParams{ProblemID: id, ID: checkID, ActorID: &actor}); err != nil {
			return err
		}
		var err error
		result, err = readCheck(ctx, q, id, actor, checkID)
		return err
	})
	return result, err
}

func (repo *RevisionRepository) Checks(ctx context.Context, id string, limit int) ([]domain.CheckRun, error) {
	if limit < 1 || limit > 100 {
		return nil, domain.InvalidInput("check page size must be 1-100")
	}
	result := []domain.CheckRun{}
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, actor string) error {
		rows, err := q.ListMaterialChecks(ctx, dbgen.ListMaterialChecksParams{ProblemID: id, ActorID: &actor, PageLimit: limit})
		if err != nil {
			return err
		}
		for _, row := range rows {
			result = append(result, domain.CheckRun{ID: row.ID, TreeHash: row.SourceTreeHash, Revision: revisionPointer(row.SourceRevision), MatchingRevision: revisionPointer(sql.NullInt64{Int64: row.MatchingRevision, Valid: row.MatchingRevision > 0}), DataHash: row.DataHash, PolicyVersion: row.CheckPolicy, ToolchainKey: row.ToolchainKey, State: row.State, Stage: row.Stage, ProgressDone: row.ProgressDone, ProgressTotal: row.ProgressTotal, PackageCases: row.PackageCases, CreatedAt: row.CreatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt})
		}
		return nil
	})
	return result, err
}

func readCheck(ctx context.Context, q *dbgen.Queries, id, actor, checkID string) (*domain.CheckRun, error) {
	row, err := q.ReadMaterialCheck(ctx, dbgen.ReadMaterialCheckParams{ProblemID: id, ID: checkID, ActorID: &actor})
	if err != nil {
		return nil, err
	}
	result := &domain.CheckRun{ID: row.ID, TreeHash: row.SourceTreeHash, Revision: revisionPointer(row.SourceRevision), MatchingRevision: revisionPointer(sql.NullInt64{Int64: row.MatchingRevision, Valid: row.MatchingRevision > 0}), DataHash: row.DataHash, PolicyVersion: row.CheckPolicy, ToolchainKey: row.ToolchainKey, State: row.State, Stage: row.Stage, ProgressDone: row.ProgressDone, ProgressTotal: row.ProgressTotal, Log: row.Log, ErrorMessage: row.ErrorMessage, PackageCases: row.PackageCases, CreatedAt: row.CreatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt}
	if err := json.Unmarshal(row.TestsJson, &result.Tests); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(row.SolutionsJson, &result.Solutions); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(row.ValidationJson, &result.Validation); err != nil {
		return nil, err
	}
	if row.State == domain.BuildSucceeded {
		artifact, err := q.PublicationCheckArtifact(ctx, dbgen.PublicationCheckArtifactParams{ProblemID: id, ID: checkID})
		if err != nil {
			return nil, err
		}
		var manifest domain.CheckArtifact
		if err := json.Unmarshal(artifact.PackageManifest, &manifest); err != nil {
			return nil, err
		}
		for _, statement := range manifest.Snapshot.Statements {
			result.Statements = append(result.Statements, domain.StatementPreview{ID: statement.ID, Language: statement.Language})
		}
	}
	return result, nil
}

func (repo *RevisionRepository) CheckStatement(ctx context.Context, id, checkID, statementID string) ([]byte, error) {
	var data []byte
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, actor string) error {
		check, err := readCheck(ctx, q, id, actor, checkID)
		if err != nil {
			return err
		}
		if check.State != domain.BuildSucceeded || repo.artifacts == nil {
			return domain.ErrNotFound
		}
		artifact, err := q.PublicationCheckArtifact(ctx, dbgen.PublicationCheckArtifactParams{ProblemID: id, ID: checkID})
		if err != nil {
			return err
		}
		var manifest domain.CheckArtifact
		if err := json.Unmarshal(artifact.PackageManifest, &manifest); err != nil {
			return err
		}
		for _, statement := range manifest.Statements {
			if statement.ID != statementID {
				continue
			}
			data, err = repo.artifacts.Read(ctx, id, artifact.PackagePath, "statements/"+statement.ID+".pdf", 32<<20)
			if err != nil {
				return err
			}
			return domain.ValidateBlob(statement.PDF, data)
		}
		return domain.ErrNotFound
	})
	return data, err
}

func (repo *RevisionRepository) CheckContent(ctx context.Context, checkID, workerID, leaseToken, digest string) (domain.BlobRef, io.ReadCloser, error) {
	if workerID == "" || leaseToken == "" {
		return domain.BlobRef{}, nil, domain.ErrStaleLease
	}
	row, err := dbgen.New(repo.db.Pool).ResolveCheckContent(ctx, dbgen.ResolveCheckContentParams{BuildID: checkID, WorkerID: sql.NullString{String: workerID, Valid: true}, LeaseToken: &leaseToken, Sha256: digest})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.BlobRef{}, nil, domain.ErrStaleLease
	}
	if err != nil {
		return domain.BlobRef{}, nil, err
	}
	ref := domain.BlobRef{SHA256: row.Sha256, Bytes: row.ByteSize}
	reader, err := repo.blobs.Open(ctx, row.ProblemID, ref)
	return ref, reader, err
}
