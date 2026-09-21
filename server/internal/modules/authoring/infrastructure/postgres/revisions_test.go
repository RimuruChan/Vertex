package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/filesystem"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
)

func TestRevisionRepository(t *testing.T) {
	ctx := dbtest.Context()
	db, release, err := dbtest.Shared(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if db == nil {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	if err := dbtest.Reset(ctx, db, "TRUNCATE problems,users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	blobs, err := filesystem.NewBlobStore(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	repo := authoringpg.NewRevisionRepository(db, blobs)
	var actors []string
	for i := range 4 {
		user, err := identitypg.NewUserRepository(db).Create(ctx, fmt.Sprintf("author%d", i), fmt.Sprintf("a%d@test.local", i), "fixture")
		if err != nil {
			t.Fatal(err)
		}
		actors = append(actors, user.ID)
	}
	actorContext := func(index int) context.Context {
		return tenancy.WithScope(context.Background(), tenancy.Scope{Domain: tenancy.Domain{ID: tenancy.OfficialID}, UserID: actors[index]})
	}
	owner, editor, reader, outsider := actorContext(0), actorContext(1), actorContext(2), actorContext(3)
	var id string
	if err := db.Pool.QueryRowContext(ctx, "INSERT INTO problems(domain_id,title,visibility,owner_id) VALUES($1,'Working copies','draft',$2) RETURNING id", tenancy.OfficialID, actors[0]).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for i, role := range []string{"editor", "reader"} {
		if _, err := db.Pool.ExecContext(ctx, "INSERT INTO problem_access(domain_id,problem_id,user_id,role) VALUES($1,$2,$3,$4)", tenancy.OfficialID, id, actors[i+1], role); err != nil {
			t.Fatal(err)
		}
	}
	upload := func(actor context.Context, text string) domain.BlobRef {
		t.Helper()
		ref, err := repo.Upload(actor, id, strings.NewReader(text))
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	save := func(actor context.Context, copy *domain.WorkingCopy, tree domain.ContentTree) *domain.WorkingCopy {
		t.Helper()
		result, err := repo.Save(actor, id, copy.ETag, tree)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	commit := func(actor context.Context, copy *domain.WorkingCopy, request string) *domain.CommitOutcome {
		t.Helper()
		result, err := repo.Commit(actor, id, domain.CommitInput{ETag: copy.ETag, RequestID: request, Message: request})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if _, err := repo.Open(reader, id); !errors.Is(err, tenancy.ErrForbidden) {
		t.Fatalf("reader created draft: %v", err)
	}
	if _, err := repo.WorkingCopy(reader, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("read-only request created draft: %v", err)
	}
	copyA, err := repo.Open(owner, id)
	if err != nil {
		t.Fatal(err)
	}
	entry := domain.TreeEntry{ID: "statement-zh", Path: "statement/problem.zh.md", Kind: domain.EntryStatement, Blob: upload(owner, "first\nmiddle\nlast\n"), Attributes: map[string]string{"format": "markdown"}}
	copyA = save(owner, copyA, domain.ContentTree{Entries: []domain.TreeEntry{entry}})
	originalETag := copyA.ETag
	for range 20 {
		copyA = save(owner, copyA, copyA.Tree)
	}
	if copyA.ETag != originalETag || copyA.BaseRevision != nil {
		t.Fatal("no-op save changed token or created a revision")
	}
	history, err := repo.History(owner, id, 0, 50)
	if err != nil || len(history) != 0 {
		t.Fatalf("save created commits: %+v %v", history, err)
	}
	request := domain.CommitInput{ETag: copyA.ETag, RequestID: "initial", Message: "initial"}
	first, err := repo.Commit(owner, id, request)
	if err != nil || first.Commit == nil || first.Commit.Revision != 1 {
		t.Fatalf("initial commit: %+v %v", first, err)
	}
	copyA = &first.Copy
	retry, err := repo.Commit(owner, id, request)
	if err != nil || retry.Commit.Revision != 1 || retry.Copy.ETag != copyA.ETag {
		t.Fatalf("retry was not idempotent: %+v %v", retry, err)
	}
	request.Message = "different"
	if _, err := repo.Commit(owner, id, request); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("reused request accepted: %v", err)
	}
	if got := commit(owner, copyA, "empty"); got.Commit != nil {
		t.Fatal("empty commit created history")
	}
	copyB, err := repo.Open(editor, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.History(outsider, id, 0, 50); !errors.Is(err, tenancy.ErrForbidden) {
		t.Fatalf("outsider read history: %v", err)
	}
	if _, err := repo.History(reader, id, 0, 50); err != nil {
		t.Fatalf("reader cannot review history: %v", err)
	}

	// A hash known from another collaborator's private upload is not a download
	// capability and cannot be attached to a guessed manifest.
	secret := upload(owner, "uncommitted private solution")
	if _, body, err := repo.Blob(editor, id, secret.SHA256); !errors.Is(err, domain.ErrNotFound) {
		if body != nil {
			body.Close()
		}
		t.Fatalf("private blob exposed: %v", err)
	}
	stolen := domain.ContentTree{Entries: []domain.TreeEntry{{ID: "stolen", Path: "secret.cpp", Kind: domain.EntrySource, Blob: secret}}}
	if _, err := repo.Save(editor, id, copyB.ETag, stolen); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("private blob reference accepted: %v", err)
	}
	unchanged, err := repo.WorkingCopy(editor, id)
	if err != nil || unchanged.ETag != copyB.ETag {
		t.Fatal("failed save changed draft")
	}

	// Concurrent authors edit different lines of the same file. The second
	// commit must incorporate the first instead of replacing it.
	treeA, _ := copyA.Tree.Canonical()
	treeB, _ := copyB.Tree.Canonical()
	treeA.Entries[0].Blob = upload(owner, "FIRST\nmiddle\nlast\n")
	treeB.Entries[0].Blob = upload(editor, "first\nmiddle\nLAST\n")
	copyA = save(owner, copyA, treeA)
	copyB = save(editor, copyB, treeB)
	var wait sync.WaitGroup
	results := make([]*domain.CommitOutcome, 2)
	failures := make([]error, 2)
	for i, input := range []struct {
		actor context.Context
		copy  *domain.WorkingCopy
	}{{owner, copyA}, {editor, copyB}} {
		wait.Go(func() {
			results[i], failures[i] = repo.Commit(input.actor, id, domain.CommitInput{ETag: input.copy.ETag, RequestID: fmt.Sprintf("parallel%d", i), Message: "independent edits"})
		})
	}
	wait.Wait()
	for i := range failures {
		if failures[i] != nil || results[i].Commit == nil {
			t.Fatalf("parallel commit %d: %+v %v", i, results[i], failures[i])
		}
	}
	_, tree, err := repo.Revision(reader, id, 3)
	if err != nil {
		t.Fatal(err)
	}
	_, body, err := repo.Blob(reader, id, tree.Entries[0].Blob.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(body)
	body.Close()
	if err != nil || string(data) != "FIRST\nmiddle\nLAST\n" {
		t.Fatalf("merge lost content: %q %v", data, err)
	}

	// Both browser tabs of the same actor retain their read token. Exactly one
	// may save, and the loser cannot overwrite the winner.
	copyA, err = repo.Reset(owner, id, results[0].Copy.ETag, 0)
	if err != nil {
		t.Fatal(err)
	}
	drafts := make([]domain.ContentTree, 2)
	for i := range drafts {
		drafts[i], _ = copyA.Tree.Canonical()
		drafts[i].Entries[0].Attributes["label"] = fmt.Sprintf("tab%d", i)
	}
	for i := range drafts {
		wait.Go(func() { _, failures[i] = repo.Save(owner, id, copyA.ETag, drafts[i]) })
	}
	wait.Wait()
	success, conflict := 0, 0
	for _, err := range failures {
		if err == nil {
			success++
		} else if errors.Is(err, domain.ErrWorkingCopyConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("tab write race = %d successes / %d conflicts", success, conflict)
	}

	// Restore changes only a private tree. A public release cannot appear as a
	// side effect of editing, committing or restoring history.
	copyA, err = repo.WorkingCopy(owner, id)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := repo.Reset(owner, id, copyA.ETag, 1)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Tree.Entries[0].Blob != entry.Blob {
		t.Fatal("restore did not load historical content")
	}
	var count int
	if err := db.Pool.GetContext(ctx, &count, "SELECT count(*) FROM problem_versions WHERE problem_id=$1", id); err != nil || count != 0 {
		t.Fatalf("editing published content: %d %v", count, err)
	}

	// Conflicts survive a reload; completing unresolved/stale sessions is not
	// allowed. The resolution becomes an edit, not an implicit shared commit.
	copyA, err = repo.Reset(owner, id, restored.ETag, 0)
	if err != nil {
		t.Fatal(err)
	}
	currentB, err := repo.WorkingCopy(editor, id)
	if err != nil {
		t.Fatal(err)
	}
	copyB, err = repo.Reset(editor, id, currentB.ETag, 0)
	if err != nil {
		t.Fatal(err)
	}
	treeA, _ = copyA.Tree.Canonical()
	treeB, _ = copyB.Tree.Canonical()
	treeA.Entries[0].Attributes["label"] = "mine"
	treeB.Entries[0].Attributes["label"] = "theirs"
	copyA = save(owner, copyA, treeA)
	copyB = save(editor, copyB, treeB)
	commit(owner, copyA, "conflict-head")
	conflicted := commit(editor, copyB, "needs-merge")
	if conflicted.Merge == nil || conflicted.Commit != nil || len(conflicted.Merge.Result.Conflicts) != 1 {
		t.Fatalf("missing conflict session: %+v", conflicted)
	}
	session, err := repo.Merge(editor, id, conflicted.Merge.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Merge(owner, id, session.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other actor read merge: %v", err)
	}
	if _, err := repo.CompleteMerge(editor, id, session.ID, session.ETag); !errors.Is(err, domain.ErrMergeRequired) {
		t.Fatalf("unresolved merge completed: %v", err)
	}
	resolved := session.Result.Tree
	resolved.Entries[0].Attributes["label"] = "chosen"
	key := session.Result.Conflicts[0]
	session, err = repo.SaveMerge(editor, id, session.ID, session.ETag, resolved, []domain.ConflictKey{{EntryID: key.EntryID, Field: key.Field}})
	if err != nil {
		t.Fatal(err)
	}
	// A new shared commit arrives while the editor is resolving conflicts.
	// Completing must preserve those decisions and reconcile the newer head.
	copyA, err = repo.WorkingCopy(owner, id)
	if err != nil {
		t.Fatal(err)
	}
	treeA, _ = copyA.Tree.Canonical()
	treeA.Entries[0].Attributes["review"] = "arrived during merge"
	copyA = save(owner, copyA, treeA)
	commit(owner, copyA, "new-head-during-merge")
	copyB, err = repo.CompleteMerge(editor, id, session.ID, session.ETag)
	if err != nil {
		t.Fatal(err)
	}
	if copyB.MergeID != "" || copyB.Tree.Entries[0].Attributes["label"] != "chosen" || copyB.Tree.Entries[0].Attributes["review"] != "arrived during merge" {
		t.Fatalf("bad resolved copy: %+v", copyB)
	}
	history, err = repo.History(reader, id, 0, 100)
	if err != nil || len(history) != 5 {
		t.Fatalf("merge created implicit commit: %d %v", len(history), err)
	}
	merged := commit(editor, copyB, "resolution")
	if merged.Commit == nil || merged.Commit.Revision != 6 {
		t.Fatalf("resolution commit: %+v", merged)
	}

	// Failure after inserting the commit and moving the head must roll back
	// both writes; retrying the same request must not leave a revision gap.
	copyA, err = repo.WorkingCopy(owner, id)
	if err != nil {
		t.Fatal(err)
	}
	copyA, err = repo.Reset(owner, id, copyA.ETag, 0)
	if err != nil {
		t.Fatal(err)
	}
	treeA, _ = copyA.Tree.Canonical()
	treeA.Entries[0].Attributes["rollback"] = "pending change"
	copyA = save(owner, copyA, treeA)
	if _, err := db.Pool.ExecContext(ctx, `CREATE FUNCTION fail_authoring_copy_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'injected copy update failure'; END $$;
CREATE TRIGGER fail_authoring_copy_update BEFORE UPDATE ON problem_working_copies FOR EACH ROW EXECUTE FUNCTION fail_authoring_copy_update();`); err != nil {
		t.Fatal(err)
	}
	rollbackRequest := domain.CommitInput{ETag: copyA.ETag, RequestID: "rollback-retry", Message: "Atomic commit"}
	_, failedCommit := repo.Commit(owner, id, rollbackRequest)
	if _, err := db.Pool.ExecContext(ctx, `DROP TRIGGER fail_authoring_copy_update ON problem_working_copies; DROP FUNCTION fail_authoring_copy_update();`); err != nil {
		t.Fatal(err)
	}
	if failedCommit == nil {
		t.Fatal("injected commit failure unexpectedly succeeded")
	}
	rolledBack, err := repo.WorkingCopy(owner, id)
	if err != nil || rolledBack.ETag != copyA.ETag || rolledBack.HeadRevision == nil || *rolledBack.HeadRevision != 6 {
		t.Fatalf("partial commit escaped rollback: %+v %v", rolledBack, err)
	}
	history, err = repo.History(reader, id, 0, 100)
	if err != nil || len(history) != 6 {
		t.Fatal("failed transaction left a commit")
	}
	retriedCommit, err := repo.Commit(owner, id, rollbackRequest)
	if err != nil || retriedCommit.Commit == nil || retriedCommit.Commit.Revision != 7 {
		t.Fatalf("failed commit was not retryable: %+v %v", retriedCommit, err)
	}

	// Explicitly revoke edit authority while the client still has a valid token.
	if _, err := db.Pool.ExecContext(ctx, "UPDATE problem_access SET role='reader' WHERE problem_id=$1 AND user_id=$2", id, actors[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Save(editor, id, merged.Copy.ETag, merged.Copy.Tree); !errors.Is(err, tenancy.ErrForbidden) {
		t.Fatalf("revoked editor saved: %v", err)
	}
}
