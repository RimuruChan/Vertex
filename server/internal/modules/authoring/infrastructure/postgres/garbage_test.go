package postgres_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/filesystem"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
)

func TestAuthoringGarbageRootsAndCommitFailure(t *testing.T) {
	ctx := dbtest.Context()
	db, release, err := dbtest.Shared(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if db == nil {
		t.Skip("TEST_DATABASE_URL not configured")
	}
	if err := dbtest.Reset(ctx, db, "TRUNCATE users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	users := identitypg.NewUserRepository(db)
	owner, err := users.Create(ctx, "gc_owner", "gc-owner@test.local", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	editor, err := users.Create(ctx, "gc_editor", "gc-editor@test.local", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	as := func(actor string) context.Context {
		return tenancy.WithScope(ctx, tenancy.Scope{Domain: tenancy.Domain{ID: tenancy.OfficialID}, UserID: actor})
	}
	ownerCtx, editorCtx := as(owner.ID), as(editor.ID)
	var id string
	if err := db.Pool.QueryRowContext(ctx, "INSERT INTO problems(domain_id,title,owner_id,visibility) VALUES($1,'GC fixture',$2,'private') RETURNING id", tenancy.OfficialID, owner.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.ExecContext(ctx, "INSERT INTO problem_access(domain_id,problem_id,user_id,role) VALUES($1,$2,$3,'editor')", tenancy.OfficialID, id, editor.ID); err != nil {
		t.Fatal(err)
	}
	blobRoot := t.TempDir()
	blobs, err := filesystem.NewBlobStore(blobRoot, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	artifactRoot := t.TempDir()
	publisher := filesystem.NewTestdataPublisher(artifactRoot)
	storage := &observedGarbageStorage{GarbageStorage: filesystem.NewGarbageStorage(blobs, publisher)}
	repo := authoringpg.NewRevisionRepository(db, blobs, publisher)
	collector := authoringpg.NewGarbageRepository(db, storage)
	copy, err := repo.Open(ownerCtx, id)
	if err != nil {
		t.Fatal(err)
	}
	save := func(actor context.Context, copy *domain.WorkingCopy, text string) (*domain.WorkingCopy, domain.BlobRef) {
		t.Helper()
		ref, err := repo.Upload(actor, id, strings.NewReader(text))
		if err != nil {
			t.Fatal(err)
		}
		tree := domain.ContentTree{Entries: append([]domain.TreeEntry{}, copy.Tree.Entries...)}
		found := false
		for index := range tree.Entries {
			if tree.Entries[index].ID == "source" {
				tree.Entries[index].Blob = ref
				found = true
			}
		}
		if !found {
			tree.Entries = append(tree.Entries, domain.TreeEntry{ID: "source", Path: "solution.cpp", Kind: domain.EntrySource, Blob: ref})
		}
		next, err := repo.Save(actor, id, copy.ETag, tree)
		if err != nil {
			t.Fatal(err)
		}
		return next, ref
	}
	copy, discarded := save(ownerCtx, copy, "discarded autosave")
	copy, committedRef := save(ownerCtx, copy, "committed source")
	committed, err := repo.Commit(ownerCtx, id, domain.CommitInput{ETag: copy.ETag, RequestID: "gc-base", Message: "Shared base"})
	if err != nil {
		t.Fatal(err)
	}
	copy = &committed.Copy
	copy, checkedRef := save(ownerCtx, copy, "private check input")
	checkTree, _ := copy.Tree.Hash()
	if _, err := db.Pool.ExecContext(ctx, "INSERT INTO problem_build_jobs(problem_id,input_json,source_tree_hash,data_hash,check_policy,created_by) VALUES($1,'{}',$2,$3,$4,$5)", id, checkTree, strings.Repeat("a", 64), domain.CheckPolicyVersion, owner.ID); err != nil {
		t.Fatal(err)
	}
	for _, hash := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
		directory := filepath.Join(artifactRoot, id, hash)
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "data"), []byte("artifact fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Pool.ExecContext(ctx, "UPDATE problem_build_jobs SET package_path=$2 WHERE problem_id=$1", id, id+"/"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	copy, _ = save(ownerCtx, copy, "import snapshot")
	exported, err := repo.ExportPackage(ownerCtx, id, 0, "vertex")
	if err != nil {
		t.Fatal(err)
	}
	_, reader, err := repo.Blob(ownerCtx, id, exported.File.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	archive, _ := io.ReadAll(reader)
	reader.Close()
	activeImport, err := repo.PreviewImport(ownerCtx, id, copy.ETag, archive, domain.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	expiredImport, err := repo.PreviewImport(ownerCtx, id, copy.ETag, archive, domain.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.ExecContext(ctx, "UPDATE problem_imports SET expires_at=now()-interval '2 hours' WHERE id=$1", expiredImport.ID); err != nil {
		t.Fatal(err)
	}
	copy, _ = save(ownerCtx, copy, "local conflict")
	other, err := repo.Open(editorCtx, id)
	if err != nil {
		t.Fatal(err)
	}
	other, otherRef := save(editorCtx, other, "remote conflict")
	if _, err := repo.Commit(editorCtx, id, domain.CommitInput{ETag: other.ETag, RequestID: "gc-remote", Message: "Remote update"}); err != nil {
		t.Fatal(err)
	}
	merging, err := repo.Commit(ownerCtx, id, domain.CommitInput{ETag: copy.ETag, RequestID: "gc-local", Message: "Local update"})
	if err != nil || merging.Merge == nil {
		t.Fatalf("missing merge: %+v %v", merging, err)
	}
	mergedRef, err := repo.Upload(ownerCtx, id, strings.NewReader("manual resolution"))
	if err != nil {
		t.Fatal(err)
	}
	tree := merging.Merge.Result.Tree
	for index := range tree.Entries {
		if tree.Entries[index].ID == "source" {
			tree.Entries[index].Blob = mergedRef
		}
	}
	merge, err := repo.SaveMerge(ownerCtx, id, merging.Merge.ID, merging.Merge.ETag, tree, []domain.ConflictKey{{EntryID: "source", Field: "blob"}})
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := repo.Upload(ownerCtx, id, strings.NewReader("orphan upload"))
	if err != nil {
		t.Fatal(err)
	}
	publicSample, err := repo.Upload(ownerCtx, id, strings.NewReader("published sample outside source tree"))
	if err != nil {
		t.Fatal(err)
	}
	if err := dbtest.PublishedProblems(ctx, db, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.ExecContext(ctx, "INSERT INTO problem_version_files(problem_id,version_no,file_id,path,filename,media_type,purpose,blob_sha256,byte_size,sample_index) VALUES($1,1,'sample','samples/1.in','1.in','text/plain','sample-input',$2,$3,1)", id, publicSample.SHA256, publicSample.Bytes); err != nil {
		t.Fatal(err)
	}
	// Fail at COMMIT, after pruning SQL has run. File removal must not begin.
	if _, err := db.Pool.ExecContext(ctx, `CREATE FUNCTION fail_gc_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected GC commit failure'; END $$;
CREATE CONSTRAINT TRIGGER fail_gc_commit AFTER DELETE ON problem_content_trees DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_gc_commit();`); err != nil {
		t.Fatal(err)
	}
	defer db.Pool.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS fail_gc_commit ON problem_content_trees; DROP FUNCTION IF EXISTS fail_gc_commit()")
	cutoff := time.Now().Add(30 * time.Minute)
	if _, err := collector.Collect(ctx, cutoff, "", 100); err == nil {
		t.Fatal("injected collection failure did not fail")
	}
	if storage.sweeps != 0 {
		t.Fatal("files removed before database commit succeeded")
	}
	if _, err := os.Stat(filepath.Join(blobRoot, id, orphan.SHA256)); err != nil {
		t.Fatal("failed GC destroyed upload")
	}
	if _, err := db.Pool.ExecContext(ctx, "DROP TRIGGER fail_gc_commit ON problem_content_trees; DROP FUNCTION fail_gc_commit()"); err != nil {
		t.Fatal(err)
	}
	stats, err := collector.Collect(ctx, cutoff, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Imports != 1 || stats.Trees == 0 || stats.Blobs == 0 || stats.Objects == 0 {
		t.Fatalf("garbage was not collected: %+v", stats)
	}
	if _, err := os.Stat(filepath.Join(artifactRoot, id, strings.Repeat("a", 64))); err != nil {
		t.Fatal("referenced check artifact was removed")
	}
	if _, err := os.Stat(filepath.Join(artifactRoot, id, strings.Repeat("b", 64))); !os.IsNotExist(err) {
		t.Fatal("unreferenced artifact remained")
	}
	for _, ref := range []domain.BlobRef{discarded, orphan} {
		if _, err := os.Stat(filepath.Join(blobRoot, id, ref.SHA256)); !os.IsNotExist(err) {
			t.Fatal("unreferenced blob remains")
		}
	}
	for _, ref := range []domain.BlobRef{committedRef, checkedRef, otherRef, mergedRef, publicSample} {
		if _, err := os.Stat(filepath.Join(blobRoot, id, ref.SHA256)); err != nil {
			t.Fatal("a rooted blob was removed")
		}
	}
	if _, reader, err := repo.Blob(ownerCtx, id, checkedRef.SHA256); err != nil {
		t.Fatalf("old check input no longer readable: %v", err)
	} else {
		reader.Close()
	}
	if _, reader, err := repo.Blob(editorCtx, id, checkedRef.SHA256); !errors.Is(err, domain.ErrNotFound) {
		if reader != nil {
			reader.Close()
		}
		t.Fatalf("private checked input leaked: %v", err)
	}
	if _, err := repo.Import(ownerCtx, id, activeImport.ID); err != nil {
		t.Fatal("live preview disappeared")
	}
	if _, err := repo.Import(ownerCtx, id, expiredImport.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("expired preview remains")
	}
	if _, err := repo.CompleteMerge(ownerCtx, id, merge.ID, merge.ETag); err != nil {
		t.Fatalf("GC broke saved merge resolution: %v", err)
	}
	if _, err := db.Pool.ExecContext(ctx, "DELETE FROM problems WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if _, err := collector.Collect(ctx, cutoff, "", 100); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{blobRoot, artifactRoot} {
		if _, err := os.Stat(filepath.Join(root, id)); !os.IsNotExist(err) {
			t.Fatalf("deleted problem left canonical content: %s %v", root, err)
		}
	}
}

type observedGarbageStorage struct {
	domain.GarbageStorage
	sweeps int
}

func (storage *observedGarbageStorage) Sweep(ctx context.Context, id string, refs domain.StorageReferences, cutoff time.Time) (domain.SweepResult, error) {
	storage.sweeps++
	return storage.GarbageStorage.Sweep(ctx, id, refs, cutoff)
}

func TestGarbageCollectionFencesDeduplicatedUploads(t *testing.T) {
	ctx := dbtest.Context()
	db, release, err := dbtest.Shared(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if db == nil {
		t.Skip("TEST_DATABASE_URL not configured")
	}
	if err := dbtest.Reset(ctx, db, "TRUNCATE users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	owner, err := identitypg.NewUserRepository(db).Create(ctx, "gc_concurrent", "gc-concurrent@test.local", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	actor := tenancy.WithScope(ctx, tenancy.Scope{Domain: tenancy.Domain{ID: tenancy.OfficialID}, UserID: owner.ID})
	var id string
	if err := db.Pool.QueryRowContext(ctx, "INSERT INTO problems(domain_id,title,owner_id,visibility) VALUES($1,'GC concurrency',$2,'private') RETURNING id", tenancy.OfficialID, owner.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	blobs, err := filesystem.NewBlobStore(root, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := blobs.Put(ctx, id, strings.NewReader("old deduplicated upload"))
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	cutoff := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, id, ref.SHA256), old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.ExecContext(ctx, "INSERT INTO problem_blobs(problem_id,sha256,byte_size,created_at) VALUES($1,$2,$3,$4)", id, ref.SHA256, ref.Bytes, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.ExecContext(ctx, "INSERT INTO problem_blob_uploads(problem_id,sha256,actor_id,created_at) VALUES($1,$2,$3,$4)", id, ref.SHA256, owner.ID, old); err != nil {
		t.Fatal(err)
	}
	gate := &gatedContentStore{ContentStore: blobs, entered: make(chan struct{}), resume: make(chan struct{})}
	defer gate.open()
	repo := authoringpg.NewRevisionRepository(db, gate)
	files := filesystem.NewGarbageStorage(blobs, filesystem.NewTestdataPublisher(t.TempDir()))
	collector := authoringpg.NewGarbageRepository(db, files)
	uploaded := make(chan error, 1)
	go func() {
		_, err := repo.Upload(actor, id, strings.NewReader("old deduplicated upload"))
		uploaded <- err
	}()
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not enter storage")
	}
	stats, err := collector.Collect(ctx, cutoff, "", 100)
	if err != nil || stats.Busy != 1 {
		t.Fatalf("collector did not skip active upload: %+v %v", stats, err)
	}
	gate.open()
	if err := <-uploaded; err != nil {
		t.Fatal(err)
	}
	if _, err := collector.Collect(ctx, cutoff, "", 100); err != nil {
		t.Fatal(err)
	}
	baseRepo := authoringpg.NewRevisionRepository(db, blobs)
	if _, reader, err := baseRepo.Blob(actor, id, ref.SHA256); err != nil {
		t.Fatalf("reupload grant was not refreshed: %v", err)
	} else {
		reader.Close()
	}
	// Pause after DB COMMIT, before filesystem removal. A new writer must wait,
	// then recreate the removed object instead of retaining a dangling reference.
	orphan, err := blobs.Put(ctx, id, strings.NewReader("orphan racing with reupload"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, id, orphan.SHA256), old, old); err != nil {
		t.Fatal(err)
	}
	paused := &pausedGarbageStorage{GarbageStorage: files, entered: make(chan struct{}), resume: make(chan struct{})}
	defer paused.open()
	gc := authoringpg.NewGarbageRepository(db, paused)
	collected := make(chan error, 1)
	go func() { _, err := gc.Collect(ctx, cutoff, "", 100); collected <- err }()
	select {
	case <-paused.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("GC did not reach file removal")
	}
	go func() {
		_, err := baseRepo.Upload(actor, id, strings.NewReader("orphan racing with reupload"))
		uploaded <- err
	}()
	select {
	case err := <-uploaded:
		t.Fatalf("writer crossed post-commit collection lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	paused.open()
	if err := <-collected; err != nil {
		t.Fatal(err)
	}
	if err := <-uploaded; err != nil {
		t.Fatal(err)
	}
	if _, reader, err := baseRepo.Blob(actor, id, orphan.SHA256); err != nil {
		t.Fatalf("GC left a referenced missing file: %v", err)
	} else {
		reader.Close()
	}
	// Cancellation must release the session advisory lock back to the pool.
	paused = &pausedGarbageStorage{GarbageStorage: files, entered: make(chan struct{}), resume: make(chan struct{})}
	defer paused.open()
	gc = authoringpg.NewGarbageRepository(db, paused)
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _, err := gc.Collect(cancelCtx, cutoff, "", 100); collected <- err }()
	select {
	case <-paused.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled GC did not start")
	}
	cancel()
	if err := <-collected; !errors.Is(err, context.Canceled) {
		t.Fatalf("collection cancellation: %v", err)
	}
	retryCtx, stop := context.WithTimeout(actor, 5*time.Second)
	defer stop()
	if _, err := baseRepo.Upload(retryCtx, id, strings.NewReader("after cancelled collection")); err != nil {
		t.Fatalf("collection leaked session lock: %v", err)
	}
}

type gatedContentStore struct {
	domain.ContentStore
	entered, resume chan struct{}
	once            sync.Once
}

func (store *gatedContentStore) open() { store.once.Do(func() { close(store.resume) }) }
func (store *gatedContentStore) Put(ctx context.Context, id string, reader io.Reader) (domain.BlobRef, error) {
	close(store.entered)
	select {
	case <-ctx.Done():
		return domain.BlobRef{}, ctx.Err()
	case <-store.resume:
		return store.ContentStore.Put(ctx, id, reader)
	}
}

type pausedGarbageStorage struct {
	domain.GarbageStorage
	entered, resume chan struct{}
	once            sync.Once
}

func (store *pausedGarbageStorage) open() { store.once.Do(func() { close(store.resume) }) }
func (store *pausedGarbageStorage) Sweep(ctx context.Context, id string, refs domain.StorageReferences, cutoff time.Time) (domain.SweepResult, error) {
	close(store.entered)
	select {
	case <-ctx.Done():
		return domain.SweepResult{}, ctx.Err()
	case <-store.resume:
		return store.GarbageStorage.Sweep(ctx, id, refs, cutoff)
	}
}
