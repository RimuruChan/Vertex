package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	authoringapp "github.com/RimuruChan/Vertex/server/internal/modules/authoring/application"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/filesystem"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
)

func TestFrozenCheckRepository(t *testing.T) {
	ctx := dbtest.Context()
	db, release, err := dbtest.Shared(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if db == nil {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	if err := dbtest.Reset(ctx, db, "TRUNCATE users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	users := identitypg.NewUserRepository(db)
	owner, err := users.Create(ctx, "check_owner", "check-owner@test.local", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	editor, err := users.Create(ctx, "check_editor", "check-editor@test.local", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	as := func(actor string) context.Context {
		return tenancy.WithScope(context.Background(), tenancy.Scope{Domain: tenancy.Domain{ID: tenancy.OfficialID}, UserID: actor})
	}
	ownerCtx, editorCtx := as(owner.ID), as(editor.ID)
	var id string
	if err := db.Pool.QueryRowContext(ctx, "INSERT INTO problems(domain_id,title,statement_md,owner_id,visibility) VALUES($1,'Sum','Add two integers',$2,'private') RETURNING id", tenancy.OfficialID, owner.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.ExecContext(ctx, "INSERT INTO problem_access(domain_id,problem_id,user_id,role) VALUES($1,$2,$3,'editor')", tenancy.OfficialID, id, editor.ID); err != nil {
		t.Fatal(err)
	}
	blobs, err := filesystem.NewBlobStore(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	repo := authoringpg.NewRevisionRepository(db, blobs)
	service := authoringapp.NewWorkbench(repo)
	builds := authoringpg.NewBuildRepository(db)
	copy, err := service.Open(ownerCtx, id)
	if err != nil {
		t.Fatal(err)
	}
	save := func(entry domain.TreeEntry, text string) {
		t.Helper()
		next, err := service.SaveEntry(ownerCtx, id, copy.ETag, entry, &text)
		if err != nil {
			t.Fatal(err)
		}
		copy = next
	}
	document := func(entryID, name, kind string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		save(domain.TreeEntry{ID: entryID, Path: name, Kind: kind, Attributes: map[string]string{"format": "json"}}, string(data))
	}
	save(domain.TreeEntry{ID: "main", Path: "programs/reference/main.cpp", Kind: domain.EntrySource}, "#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<a+b<<'\\n';}\n")
	document("reference", "vertex/programs/reference.json", domain.EntryProgram, domain.ProgramMaterial{SchemaVersion: 1, Name: "Reference", Role: "solution", Language: "cpp", Protocol: "stdio", Files: []string{"main"}, EntryPoint: "main", ExpectedVerdicts: []string{"Accepted"}})
	save(domain.TreeEntry{ID: "input", Path: "data/secret/1.in", Kind: domain.EntryInput}, "1 2\n")
	save(domain.TreeEntry{ID: "answer", Path: "data/secret/1.ans", Kind: domain.EntryAnswer}, "3\n")
	document("case1", "vertex/tests/case1.json", domain.EntryTest, domain.TestMaterial{SchemaVersion: 1, Name: "Case 1", Input: domain.TestInput{Kind: "file", Entry: "input"}, Answer: domain.TestAnswer{Kind: "file", Entry: "answer"}})
	meta, err := service.Material(ownerCtx, id, "problem", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Metadata.TestOrder) != 1 || meta.Metadata.TestOrder[0] != "case1" {
		t.Fatal("adding a test did not atomically update test order")
	}
	meta.Metadata.MainSolution = "reference"
	document("problem", "vertex/problem.json", domain.EntryMetadata, meta.Metadata)
	report, err := repo.Inspect(ownerCtx, id, 0, copy.ETag)
	if err != nil || !report.CanBuild {
		t.Fatalf("complete fixture is not ready: %+v %v", report, err)
	}
	check, err := repo.StartCheck(ownerCtx, id, domain.CheckSelection{ETag: copy.ETag})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := repo.StartCheck(ownerCtx, id, domain.CheckSelection{ETag: copy.ETag})
	if err != nil || duplicate.ID != check.ID {
		t.Fatal("duplicate start queued another check")
	}
	if check.Revision != nil || check.TreeHash != report.TreeHash || check.DataHash != report.DataHash {
		t.Fatalf("wrong source binding: %+v", check)
	}
	if _, err := repo.Check(editorCtx, id, check.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("editor read private check: %v", err)
	}
	ownerList, err := repo.Library(ownerCtx, domain.LibraryQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	editorList, err := repo.Library(editorCtx, domain.LibraryQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(ownerList.Items) != 1 || ownerList.Items[0].CheckID != check.ID || !ownerList.Items[0].CheckMatches {
		t.Fatalf("owner summary missing check: %+v", ownerList)
	}
	if len(editorList.Items) != 1 || editorList.Items[0].CheckID != "" || editorList.Items[0].HasCopy {
		t.Fatalf("summary leaked private check or copy: %+v", editorList)
	}
	if _, err := repo.CancelCheck(editorCtx, id, check.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("editor cancelled private check: %v", err)
	}
	visible, err := repo.Checks(editorCtx, id, 30)
	if err != nil || len(visible) != 0 {
		t.Fatalf("private check appeared in listing: %+v %v", visible, err)
	}
	if job, _, err := builds.Claim(context.Background(), "old-worker", time.Minute); err != nil || job != nil {
		t.Fatalf("worker without protocol claimed new snapshot: %+v %v", job, err)
	}
	workerCtx := domain.WithCheckProtocol(context.Background(), domain.CheckPolicyVersion)
	job, pkg, err := builds.Claim(workerCtx, "new-worker", time.Minute)
	if err != nil || job == nil || pkg.Check == nil {
		t.Fatalf("snapshot claim: %+v %v", pkg, err)
	}
	if pkg.Check.TreeHash != check.TreeHash || pkg.Check.Tests[0].Input == nil || pkg.Check.Tests[0].Answer == nil {
		t.Fatalf("incomplete sealed input: %+v", pkg.Check)
	}
	inputRef := *pkg.Check.Tests[0].Input
	firstCommit, err := repo.Commit(ownerCtx, id, domain.CommitInput{ETag: copy.ETag, Message: "Share checked materials", RequestID: "share-checked"})
	if err != nil {
		t.Fatal(err)
	}
	copy = &firstCommit.Copy
	readable, err := repo.Check(editorCtx, id, check.ID)
	if err != nil || readable.Revision != nil || readable.MatchingRevision == nil || *readable.MatchingRevision != 1 {
		t.Fatalf("committing identical content did not make its check reviewable: %+v %v", readable, err)
	}
	reused, err := repo.StartCheck(ownerCtx, id, domain.CheckSelection{Revision: 1})
	if err != nil || reused.ID != check.ID {
		t.Fatalf("identical committed content did not reuse the active check: %+v %v", reused, err)
	}
	save(domain.TreeEntry{ID: "input", Path: "data/secret/1.in", Kind: domain.EntryInput}, "2 5\n")
	changed, err := repo.Inspect(ownerCtx, id, 0, copy.ETag)
	if err != nil || changed.DataHash == check.DataHash {
		t.Fatal("input edit did not change current fingerprint")
	}
	_, body, err := repo.CheckContent(context.Background(), job.ID, "new-worker", job.LeaseToken, inputRef.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(body)
	body.Close()
	if err != nil || string(data) != "1 2\n" {
		t.Fatalf("running check read newer input: %q %v", data, err)
	}
	if _, err := db.Pool.ExecContext(ctx, "UPDATE problem_build_jobs SET input_json='{}' WHERE id=$1", job.ID); err == nil {
		t.Fatal("database allowed frozen input mutation")
	}
	if _, err := db.Pool.ExecContext(ctx, "UPDATE problem_build_jobs SET lease_expires_at=now()-interval '1 second' WHERE id=$1", job.ID); err != nil {
		t.Fatal(err)
	}
	reclaimed, retryPkg, err := builds.Claim(workerCtx, "retry-worker", time.Minute)
	if err != nil || reclaimed == nil || retryPkg.Check.TreeHash != check.TreeHash {
		t.Fatalf("retry changed frozen input: %+v %v", retryPkg, err)
	}
	if _, reader, err := repo.CheckContent(context.Background(), job.ID, "new-worker", job.LeaseToken, inputRef.SHA256); !errors.Is(err, domain.ErrStaleLease) {
		if reader != nil {
			reader.Close()
		}
		t.Fatalf("obsolete worker downloaded data: %v", err)
	}
	upload := domain.PackageUpload{StoragePath: id + "/" + strings.Repeat("a", 64), SHA256: strings.Repeat("a", 64), CaseCount: 1, Checker: "diff"}
	upload.Artifact = &domain.CheckArtifact{SchemaVersion: 1, ToolchainKey: strings.Repeat("b", 64), Snapshot: *retryPkg.Check, Tests: []domain.ArtifactTest{{ID: "case1", Input: *retryPkg.Check.Tests[0].Input, Answer: *retryPkg.Check.Tests[0].Answer}}}
	if err := builds.RecordPackage(context.Background(), reclaimed.ID, id, "retry-worker", reclaimed.LeaseToken, upload); err != nil {
		t.Fatal(err)
	}
	if err := builds.Progress(ctx, domain.Progress{BuildID: reclaimed.ID, WorkerID: "retry-worker", LeaseToken: reclaimed.LeaseToken, Stage: "checking", Log: "binary\x00\xff diagnostic"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := builds.Complete(context.Background(), domain.BuildResult{BuildID: reclaimed.ID, WorkerID: "retry-worker", LeaseToken: reclaimed.LeaseToken, Success: true, Log: "result\x00 diagnostic", Tests: []domain.TestOutcome{{InputHead: "\x00binary\xff", AnswerHead: strings.Repeat("中", 3000)}}}, "diff"); err != nil {
		t.Fatal(err)
	}
	var savedJSON, savedLog string
	if err := db.Pool.QueryRowContext(ctx, "SELECT tests_json::text, log FROM problem_build_jobs WHERE id=$1", reclaimed.ID).Scan(&savedJSON, &savedLog); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(savedJSON, "�binary�") || !strings.Contains(savedLog, "binary�� diagnostic") || !strings.Contains(savedJSON, "truncated") {
		t.Fatalf("binary report was not safely retained: %s", savedLog)
	}
	failed, err := repo.Check(ownerCtx, id, check.ID)
	if err != nil || failed.State != "failed" || !strings.Contains(failed.ErrorMessage, "工具链") {
		t.Fatalf("unbound result accepted: %+v %v", failed, err)
	}
	committed, err := repo.Commit(ownerCtx, id, domain.CommitInput{ETag: copy.ETag, Message: "Shared check input", RequestID: "shared-check"})
	if err != nil {
		t.Fatal(err)
	}
	shared, err := repo.StartCheck(ownerCtx, id, domain.CheckSelection{Revision: committed.Commit.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Check(editorCtx, id, shared.ID); err != nil {
		t.Fatalf("shared revision check unreadable: %v", err)
	}
	if cancelled, err := repo.CancelCheck(editorCtx, id, shared.ID); err != nil || cancelled.State != "cancelled" {
		t.Fatalf("shared check cancellation: %+v %v", cancelled, err)
	}
	// Installation now runs under one storage transaction. Lease checks must
	// use wall-clock time, not PostgreSQL's frozen transaction-start now().
	if _, err := repo.StartCheck(ownerCtx, id, domain.CheckSelection{Revision: committed.Commit.Revision}); err != nil {
		t.Fatal(err)
	}
	late, lateInput, err := builds.Claim(workerCtx, "slow-upload", time.Minute)
	if err != nil || late == nil {
		t.Fatalf("claim late upload: %v", err)
	}
	if _, err := db.Pool.ExecContext(ctx, "UPDATE problem_build_jobs SET lease_expires_at=clock_timestamp()+interval '200 milliseconds' WHERE id=$1", late.ID); err != nil {
		t.Fatal(err)
	}
	lateArtifact := domain.CheckArtifact{SchemaVersion: 1, ToolchainKey: strings.Repeat("b", 64), Snapshot: *lateInput.Check, Tests: []domain.ArtifactTest{{ID: lateInput.Check.Tests[0].ID, Input: *lateInput.Check.Tests[0].Input, Answer: *lateInput.Check.Tests[0].Answer}}}
	err = builds.WithArtifactStorage(ctx, id, func(guarded context.Context) error {
		time.Sleep(300 * time.Millisecond)
		return builds.RecordPackage(guarded, late.ID, id, "slow-upload", late.LeaseToken, domain.PackageUpload{StoragePath: id + "/" + strings.Repeat("a", 64), SHA256: strings.Repeat("a", 64), CaseCount: 1, Artifact: &lateArtifact})
	})
	if !errors.Is(err, domain.ErrStaleLease) {
		t.Fatalf("expired lease won after a slow storage transaction: %v", err)
	}
}
