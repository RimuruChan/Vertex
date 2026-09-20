package postgres

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
)

type GarbageRepository struct {
	db    *database.DB
	files domain.GarbageStorage
}

func NewGarbageRepository(db *database.DB, files domain.GarbageStorage) *GarbageRepository {
	return &GarbageRepository{db, files}
}

func (repo *GarbageRepository) Collect(ctx context.Context, cutoff time.Time, after string, limit int) (domain.GarbageStats, error) {
	result := domain.GarbageStats{}
	if limit < 1 || limit > 1000 {
		return result, domain.InvalidInput("invalid collection batch")
	}
	ids, err := dbgen.New(repo.db.Pool).GarbageNamespaces(ctx, dbgen.GarbageNamespacesParams{AfterID: after, PageLimit: limit + 1})
	if err != nil {
		return result, err
	}
	stored, err := repo.files.Namespaces(ctx)
	if err != nil {
		return result, err
	}
	seen := map[string]bool{}
	for _, id := range append(ids, stored...) {
		if id > after {
			seen[id] = true
		}
	}
	ordered := make([]string, 0, len(seen))
	for id := range seen {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	if len(ordered) > limit {
		ordered = ordered[:limit]
		result.Next = ordered[len(ordered)-1]
	}
	var failures error
	for _, id := range ordered {
		stats, err := repo.collectNamespace(ctx, id, cutoff)
		result.Namespaces += stats.Namespaces
		result.Busy += stats.Busy
		result.Imports += stats.Imports
		result.Grants += stats.Grants
		result.Trees += stats.Trees
		result.Blobs += stats.Blobs
		result.Objects += stats.Objects
		result.Bytes += stats.Bytes
		if err != nil {
			failures = errors.Join(failures, fmt.Errorf("collect authoring namespace %s: %w", id, err))
			if ctx.Err() != nil {
				break
			}
		}
	}
	return result, failures
}

func (repo *GarbageRepository) collectNamespace(ctx context.Context, id string, cutoff time.Time) (result domain.GarbageStats, failure error) {
	result = domain.GarbageStats{Namespaces: 1}
	committed := false
	defer func() {
		if !committed {
			result.Imports = 0
			result.Grants = 0
			result.Trees = 0
			result.Blobs = 0
		}
	}()
	conn, err := repo.db.Pool.Connx(ctx)
	if err != nil {
		return result, err
	}
	defer conn.Close()
	key := tenancypg.ContentStorageKey(id)
	lockQueries := dbgen.New(conn)
	unlockRequired := true
	// Session lock deliberately outlives the pruning transaction. A failed
	// unlock discards the connection instead of poisoning the connection pool.
	defer func() {
		if !unlockRequired {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := lockQueries.ReleaseGarbageStorageLock(cleanup, key); err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	locked, err := lockQueries.TryGarbageStorageLock(ctx, key)
	if err != nil {
		return result, err
	}
	if !locked {
		unlockRequired = false
		result.Busy = 1
		return result, nil
	}
	tx, err := conn.BeginTxx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	q := dbgen.New(tx)
	result.Imports, err = q.ExpirePackageImports(ctx, dbgen.ExpirePackageImportsParams{ProblemID: id, Cutoff: cutoff})
	if err != nil {
		return result, err
	}
	result.Grants, err = q.ExpireBlobGrants(ctx, dbgen.ExpireBlobGrantsParams{ProblemID: id, Cutoff: cutoff})
	if err != nil {
		return result, err
	}
	result.Trees, err = q.PruneAuthoringTrees(ctx, dbgen.PruneAuthoringTreesParams{ProblemID: id, Cutoff: cutoff})
	if err != nil {
		return result, err
	}
	result.Blobs, err = q.PruneAuthoringBlobs(ctx, dbgen.PruneAuthoringBlobsParams{ProblemID: id, Cutoff: cutoff})
	if err != nil {
		return result, err
	}
	blobs, err := q.RetainedAuthoringBlobs(ctx, id)
	if err != nil {
		return result, err
	}
	artifacts, err := q.RetainedAuthoringArtifacts(ctx, id)
	if err != nil {
		return result, err
	}
	refs := domain.StorageReferences{Blobs: map[string]bool{}, Artifacts: map[string]bool{}}
	for _, hash := range blobs {
		refs.Blobs[hash] = true
	}
	for _, name := range artifacts {
		refs.Artifacts[name] = true
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	committed = true
	swept, err := repo.files.Sweep(ctx, id, refs, cutoff)
	result.Objects = swept.Objects
	result.Bytes = swept.Bytes
	return result, err
}
