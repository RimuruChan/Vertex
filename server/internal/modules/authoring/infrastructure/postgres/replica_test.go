package postgres_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	authoringapp "github.com/RimuruChan/Vertex/server/internal/modules/authoring/application"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/filesystem"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/application"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
)

// This simulates a completed check to exercise actual PostgreSQL authorization,
// rollback, blob transfer and filesystem artifact copying. It is not execution.
func TestCommittedReleaseCopy(t *testing.T) {
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
	owner, err := users.Create(ctx, "source_setter", "source-setter@test.local", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	copier, err := users.Create(ctx, "target_setter", "target-setter@test.local", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	spaces := tenancyapp.NewService(tenancypg.NewRepository(db))
	sourceSpace, err := spaces.Create(ctx, owner.ID, tenancy.CreateInput{Slug: "replica-source", Name: "Source"})
	if err != nil {
		t.Fatal(err)
	}
	targetSpace, err := spaces.Create(ctx, copier.ID, tenancy.CreateInput{Slug: "replica-target", Name: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	if err := spaces.SetMember(ctx, sourceSpace.Domain.Slug, owner.ID, tenancy.MemberInput{Username: copier.Username, RoleKey: "member", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	sourceCtx := tenancy.WithScope(ctx, sourceSpace)
	targetCtx := tenancy.WithScope(ctx, targetSpace)
	copierSourceCtx := tenancy.WithScope(ctx, tenancy.Scope{Domain: sourceSpace.Domain, UserID: copier.ID})
	var id, number string
	if err := db.Pool.QueryRowContext(ctx, "INSERT INTO problems(domain_id,title,statement_md,owner_id,visibility) VALUES($1,'private-check-metadata','Published body',$2,'private') RETURNING id,public_id", sourceSpace.Domain.ID, owner.ID).Scan(&id, &number); err != nil {
		t.Fatal(err)
	}
	blobs, err := filesystem.NewBlobStore(t.TempDir(), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	publisher := filesystem.NewTestdataPublisher(t.TempDir())
	repo := authoringpg.NewRevisionRepository(db, blobs, publisher)
	service := authoringapp.NewWorkbench(repo)
	copy, err := repo.Open(sourceCtx, id)
	if err != nil {
		t.Fatal(err)
	}
	save := func(entryID, name, kind, text string) {
		t.Helper()
		entry := domain.TreeEntry{ID: entryID, Path: name, Kind: kind}
		var err error
		copy, err = service.SaveEntry(sourceCtx, id, copy.ETag, entry, &text)
		if err != nil {
			t.Fatal(err)
		}
	}
	doc := func(entryID, name, kind string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		save(entryID, name, kind, string(data))
	}
	save("main", "solutions/main.cpp", domain.EntrySource, "#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<a+b<<'\\n';}\n")
	doc("reference", "vertex/programs/reference.json", domain.EntryProgram, domain.ProgramMaterial{SchemaVersion: 1, Name: "Reference", Directory: "solutions", Role: "solution", Language: "cpp", Protocol: "stdio", Files: []string{"main"}, EntryPoint: "main", ExpectedVerdicts: []string{"Accepted"}})
	save("input", "data/1.in", domain.EntryInput, "1 2\n")
	save("answer", "data/1.ans", domain.EntryAnswer, "3\n")
	doc("case", "vertex/tests/1.json", domain.EntryTest, domain.TestMaterial{SchemaVersion: 1, Name: "Case", IsSample: true, Input: domain.TestInput{Kind: "file", Entry: "input"}, Answer: domain.TestAnswer{Kind: "file", Entry: "answer"}})
	meta, err := service.Material(sourceCtx, id, "problem", 0)
	if err != nil {
		t.Fatal(err)
	}
	meta.Metadata.MainSolution = "reference"
	doc("problem", "vertex/problem.json", domain.EntryMetadata, meta.Metadata)
	check, err := repo.StartCheck(sourceCtx, id, domain.CheckSelection{ETag: copy.ETag})
	if err != nil {
		t.Fatal(err)
	}
	load := func(ref domain.BlobRef) ([]byte, error) {
		reader, err := blobs.Open(ctx, id, ref)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	}
	_, snapshot, err := domain.InspectMaterials(copy.Tree, load)
	if err != nil {
		t.Fatal(err)
	}
	manifest := domain.CheckArtifact{SchemaVersion: 1, ToolchainKey: strings.Repeat("c", 64), Snapshot: *snapshot, Tests: []domain.ArtifactTest{{ID: "case", Input: *snapshot.Tests[0].Input, Answer: *snapshot.Tests[0].Answer}}}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	write := func(name string, data []byte) {
		t.Helper()
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	encoded, _ := json.Marshal(manifest)
	write("artifact.json", encoded)
	for index, test := range manifest.Tests {
		for suffix, ref := range map[string]domain.BlobRef{"in": test.Input, "out": test.Answer} {
			data, err := load(ref)
			if err != nil {
				t.Fatal(err)
			}
			write(fmt.Sprintf("%d.%s", index+1, suffix), data)
		}
	}
	for _, program := range snapshot.Programs {
		for _, file := range program.Files {
			data, err := load(file.Blob)
			if err != nil {
				t.Fatal(err)
			}
			write("programs/"+program.ID+"/"+file.Path, data)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	upload, err := publisher.Publish(id, archive.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.ExecContext(ctx, "UPDATE problem_build_jobs SET state='succeeded',toolchain_key=$2,package_manifest=$3,package_path=$4,package_sha256=$5,package_cases=1,finished_at=now() WHERE id=$1", check.ID, manifest.ToolchainKey, encoded, upload.StoragePath, upload.SHA256); err != nil {
		t.Fatal(err)
	}
	meta.Metadata.Title = "Released source title"
	doc("problem", "vertex/problem.json", domain.EntryMetadata, meta.Metadata)
	commit, err := repo.Commit(sourceCtx, id, domain.CommitInput{ETag: copy.ETag, RequestID: "source-release", Message: "Release metadata"})
	if err != nil {
		t.Fatal(err)
	}
	version, err := repo.PublishCommit(sourceCtx, id, domain.CommitPublication{Revision: commit.Commit.Revision, CheckID: check.ID, ExpectedVersion: 0})
	if err != nil {
		t.Fatal(err)
	}
	copy = &commit.Copy
	text := "uncommitted statement must stay private"
	entry := copy.Tree.Entries[0]
	for _, value := range copy.Tree.Entries {
		if value.Kind == domain.EntryStatement {
			entry = value
			break
		}
	}
	copy, err = service.SaveEntry(sourceCtx, id, copy.ETag, entry, &text)
	if err != nil {
		t.Fatal(err)
	}
	input := domain.CopyInput{SourceDomain: sourceSpace.Domain.Slug, SourceProblem: number, SourceVersion: version.Version, Attribution: "Training copy"}
	if _, err := db.Pool.ExecContext(ctx, "DELETE FROM problem_blobs WHERE problem_id=$1", id); err == nil {
		t.Fatal("deferred graph allowed deleting referenced blobs")
	}
	if _, err := repo.CopyRelease(targetCtx, input); err == nil {
		t.Fatal("copied source without package permission")
	}
	if _, err := db.Pool.ExecContext(ctx, "INSERT INTO problem_access(domain_id,problem_id,user_id,role) VALUES($1,$2,$3,'reader')", sourceSpace.Domain.ID, id, copier.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Check(copierSourceCtx, id, check.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("original private check unexpectedly readable: %v", err)
	}
	failing := authoringpg.NewRevisionRepository(db, blobs, failingCheckedCopy{publisher})
	if _, err := failing.CopyRelease(targetCtx, input); err == nil {
		t.Fatal("injected copy failure did not fail")
	}
	var count int
	if err := db.Pool.GetContext(ctx, &count, "SELECT count(*) FROM problems WHERE domain_id=$1", targetSpace.Domain.ID); err != nil || count != 0 {
		t.Fatal("failed copy left a partial problem")
	}
	result, err := repo.CopyRelease(targetCtx, input)
	if err != nil {
		t.Fatal(err)
	}
	cloned, err := repo.WorkingCopy(targetCtx, result.ProblemID)
	if err != nil {
		t.Fatal(err)
	}
	if cloned.BaseRevision != nil || cloned.HeadRevision != nil {
		t.Fatal("copy implicitly committed")
	}
	copyMeta, err := service.Material(targetCtx, result.ProblemID, "problem", 0)
	if err != nil || copyMeta.Metadata.Title != "Released source title" {
		t.Fatalf("wrong copied metadata: %+v %v", copyMeta, err)
	}
	for _, entry := range cloned.Tree.Entries {
		if entry.Kind == domain.EntryStatement {
			_, reader, err := repo.Blob(targetCtx, result.ProblemID, entry.Blob.SHA256)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(reader)
			reader.Close()
			if string(data) != "Published body" {
				t.Fatalf("copied uncommitted statement: %q", data)
			}
		}
	}
	checks, err := repo.Checks(targetCtx, result.ProblemID, 20)
	if err != nil || len(checks) != 1 || checks[0].State != domain.BuildSucceeded {
		t.Fatalf("copied check missing: %+v %v", checks, err)
	}
	if strings.Contains(checks[0].Log, "private-check-metadata") {
		t.Fatal("copied check leaked unpublished metadata")
	}
	var artifactPath string
	if err := db.Pool.GetContext(ctx, &artifactPath, "SELECT package_path FROM problem_build_jobs WHERE id=$1", checks[0].ID); err != nil {
		t.Fatal(err)
	}
	artifactData, err := publisher.Read(ctx, result.ProblemID, artifactPath, "artifact.json", 8<<20)
	if err != nil || bytes.Contains(artifactData, []byte("private-check-metadata")) {
		t.Fatal("copied artifact leaked private source snapshot")
	}
	if _, err := db.Pool.ExecContext(ctx, "DELETE FROM problems WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Remove(upload.StoragePath); err != nil {
		t.Fatal(err)
	}
	origin, err := repo.Origin(targetCtx, result.ProblemID)
	if err != nil || origin.SourceVersion != 1 || origin.SourceTitle != "Released source title" {
		t.Fatal("provenance did not survive source deletion")
	}
	clonedCommit, err := repo.Commit(targetCtx, result.ProblemID, domain.CommitInput{ETag: cloned.ETag, Message: "Adopt released source", RequestID: "copy-commit"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PublishCommit(targetCtx, result.ProblemID, domain.CommitPublication{Revision: clonedCommit.Commit.Revision, CheckID: checks[0].ID, ExpectedVersion: 0}); err != nil {
		t.Fatalf("copy still required its deleted source: %v", err)
	}
}

type failingCheckedCopy struct{ *filesystem.TestdataPublisher }

func (value failingCheckedCopy) CloneChecked(ctx context.Context, source, target string, upload domain.PackageUpload, snapshot domain.CheckSnapshot) (*domain.PackageUpload, error) {
	if _, err := value.TestdataPublisher.CloneChecked(ctx, source, target, upload, snapshot); err != nil {
		return nil, err
	}
	return nil, errors.New("injected post-copy failure")
}
