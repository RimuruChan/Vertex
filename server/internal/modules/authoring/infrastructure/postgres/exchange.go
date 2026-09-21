package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/packages"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
)

func (repo *RevisionRepository) PreviewImport(ctx context.Context, id, etag string, data []byte, options domain.ImportOptions) (*domain.ImportReceipt, error) {
	var result *domain.ImportReceipt
	err := repo.transaction(ctx, id, true, func(q *dbgen.Queries, actor string) error {
		copy, _, err := readCopy(ctx, q, id, actor)
		if err != nil {
			return err
		}
		if copy.ETag != etag {
			return domain.ErrWorkingCopyConflict
		}
		if copy.MergeID != "" {
			return domain.ErrMergeRequired
		}
		refs := map[string]domain.BlobRef{}
		put := func(reader io.Reader) (domain.BlobRef, error) {
			ref, err := repo.blobs.Put(ctx, id, reader)
			if err == nil {
				refs[ref.SHA256] = ref
			}
			return ref, err
		}
		archive, err := put(bytes.NewReader(data))
		if err != nil {
			return err
		}
		plan, err := packages.Import(data, options, put)
		if err != nil {
			return err
		}
		if err := packages.MergeData(copy.Tree, plan, func(ref domain.BlobRef) (io.ReadCloser, error) { return repo.blobs.Open(ctx, id, ref) }, put); err != nil {
			return err
		}
		hashes := make([]string, 0, len(refs))
		for hash := range refs {
			hashes = append(hashes, hash)
		}
		sort.Strings(hashes)
		sizes := make([]int64, 0, len(hashes))
		for _, hash := range hashes {
			sizes = append(sizes, refs[hash].Bytes)
		}
		if err := q.RegisterImportBlobs(ctx, dbgen.RegisterImportBlobsParams{ProblemID: id, Hashes: hashes, Sizes: sizes}); err != nil {
			return err
		}
		if err := q.GrantImportBlobs(ctx, dbgen.GrantImportBlobsParams{ProblemID: id, ActorID: actor, Hashes: hashes}); err != nil {
			return err
		}
		treeHash, err := repo.persistTree(ctx, q, id, actor, plan.Tree)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(plan)
		if err != nil {
			return err
		}
		row, err := q.CreatePackageImport(ctx, dbgen.CreatePackageImportParams{ProblemID: id, ActorID: actor, BaseEtag: etag, ArchiveHash: archive.SHA256, TreeHash: treeHash, PlanJson: encoded})
		if err != nil {
			return err
		}
		result = &domain.ImportReceipt{ID: row.ID, ETag: row.BaseEtag, Plan: *plan, ExpiresAt: row.ExpiresAt}
		return nil
	})
	return result, err
}

func (repo *RevisionRepository) Import(ctx context.Context, id, importID string) (*domain.ImportReceipt, error) {
	var result *domain.ImportReceipt
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, actor string) error {
		row, err := q.ReadPackageImport(ctx, dbgen.ReadPackageImportParams{ProblemID: id, ActorID: actor, ID: importID})
		if err != nil {
			return err
		}
		var plan domain.ImportPlan
		if err := json.Unmarshal(row.PlanJson, &plan); err != nil {
			return err
		}
		result = &domain.ImportReceipt{ID: row.ID, ETag: row.BaseEtag, Plan: plan, ExpiresAt: row.ExpiresAt, Applied: row.AppliedAt != nil}
		return nil
	})
	return result, err
}

func (repo *RevisionRepository) ApplyImport(ctx context.Context, id, importID, etag string) (*domain.WorkingCopy, error) {
	var result *domain.WorkingCopy
	err := repo.transaction(ctx, id, true, func(q *dbgen.Queries, actor string) error {
		row, err := q.ReadPackageImport(ctx, dbgen.ReadPackageImportParams{ProblemID: id, ActorID: actor, ID: importID})
		if err != nil {
			return err
		}
		copy, record, err := readCopy(ctx, q, id, actor)
		if err != nil {
			return err
		}
		if row.AppliedAt != nil {
			result = copy
			return nil
		}
		if copy.ETag != etag || row.BaseEtag != etag {
			return domain.ErrWorkingCopyConflict
		}
		if copy.MergeID != "" {
			return domain.ErrMergeRequired
		}
		if time.Now().After(row.ExpiresAt) {
			return domain.InvalidInput("导入预检已过期，请重新预检")
		}
		var plan domain.ImportPlan
		if err := json.Unmarshal(row.PlanJson, &plan); err != nil {
			return err
		}
		if !plan.CanApply {
			return domain.InvalidInput("导入预检存在必须先处理的错误")
		}
		hash, err := repo.persistTree(ctx, q, id, actor, plan.Tree)
		if err != nil {
			return err
		}
		if hash != record.TreeHash {
			if err := replaceCopy(ctx, q, id, actor, etag, hash, record.BaseRevision); err != nil {
				return err
			}
		}
		affected, err := q.ApplyPackageImport(ctx, dbgen.ApplyPackageImportParams{ProblemID: id, ActorID: actor, ID: importID})
		if err != nil {
			return err
		}
		if affected != 1 {
			return domain.ErrWorkingCopyConflict
		}
		result, _, err = readCopy(ctx, q, id, actor)
		return err
	})
	return result, err
}

func (repo *RevisionRepository) ExportPackage(ctx context.Context, id string, revision int64, format string) (*domain.PackageExport, error) {
	if revision < 0 {
		return nil, domain.InvalidInput("invalid export revision")
	}
	var result *domain.PackageExport
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, actor string) error {
		var tree domain.ContentTree
		if revision == 0 {
			copy, _, err := readCopy(ctx, q, id, actor)
			if err != nil {
				return err
			}
			tree = copy.Tree
		} else {
			row, err := q.ReadContentCommit(ctx, dbgen.ReadContentCommitParams{ProblemID: id, Revision: revision})
			if err != nil {
				return err
			}
			tree, err = loadTree(ctx, q, id, row.TreeHash)
			if err != nil {
				return err
			}
		}
		exported, err := packages.Export(tree, packages.ExportOptions{Format: format, Identity: id, ReadTest: repo.exportTests(ctx, q, id, actor, tree)}, func(ref domain.BlobRef) (io.ReadCloser, error) { return repo.blobs.Open(ctx, id, ref) })
		if err != nil {
			return err
		}
		file, err := repo.blobs.Put(ctx, id, bytes.NewReader(exported.Data))
		if err != nil {
			return err
		}
		if err := registerBlob(ctx, q, id, actor, file); err != nil {
			return err
		}
		result = &domain.PackageExport{File: file, Filename: exported.Filename, Format: exported.Format, Issues: exported.Issues}
		return nil
	})
	return result, err
}

// Resolve generated data only from an authorized successful check of identical
// judging inputs. A private check of another author is not a reusable capability.
func (repo *RevisionRepository) exportTests(ctx context.Context, q *dbgen.Queries, id, actor string, tree domain.ContentTree) func(string, bool) (io.ReadCloser, error) {
	var manifest *domain.CheckArtifact
	var storagePath string
	return func(testID string, answer bool) (io.ReadCloser, error) {
		if repo.artifacts == nil {
			return nil, domain.InvalidInput("题包产物存储尚未配置")
		}
		if manifest == nil {
			inspection, snapshot, err := repo.inspectTree(ctx, id, tree)
			if err != nil {
				return nil, err
			}
			if !inspection.CanBuild {
				return nil, domain.ErrNotBuildable
			}
			row, err := q.FindExportCheck(ctx, dbgen.FindExportCheckParams{ProblemID: id, ActorID: &actor, DataHash: snapshot.DataHash, CheckPolicy: domain.CheckPolicyVersion})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil, domain.InvalidInput("生成数据需要先完成当前材料的检查")
				}
				return nil, err
			}
			var artifact domain.CheckArtifact
			if err := json.Unmarshal(row.PackageManifest, &artifact); err != nil {
				return nil, err
			}
			if artifact.Snapshot.DataHash != snapshot.DataHash || artifact.ToolchainKey != row.ToolchainKey || !toolchainKeyPattern.MatchString(row.ToolchainKey) || len(artifact.Tests) != len(snapshot.Tests) {
				return nil, domain.ErrNotPublished
			}
			manifest, storagePath = &artifact, row.PackagePath
		}
		for index, test := range manifest.Tests {
			if test.ID != testID {
				continue
			}
			ref, suffix := test.Input, ".in"
			if answer {
				ref, suffix = test.Answer, ".out"
			}
			if ref.Bytes > packages.MaxEntryBytes {
				return nil, domain.ErrPackageTooBig
			}
			data, err := repo.artifacts.Read(ctx, id, storagePath, fmt.Sprintf("%d%s", index+1, suffix), packages.MaxEntryBytes)
			if err != nil {
				return nil, err
			}
			if err := domain.ValidateBlob(ref, data); err != nil {
				return nil, err
			}
			return io.NopCloser(bytes.NewReader(data)), nil
		}
		return nil, domain.InvalidInput("检查产物缺少导出测试点")
	}
}
