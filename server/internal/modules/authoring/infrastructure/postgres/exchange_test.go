package postgres_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	authoringapp "github.com/RimuruChan/Vertex/server/internal/modules/authoring/application"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/filesystem"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
)

func TestPackageImportTransactions(t *testing.T) {
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
	owner, err := users.Create(ctx, "import_owner", "import-owner@test.local", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	editor, err := users.Create(ctx, "import_editor", "import-editor@test.local", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	as := func(id string) context.Context {
		return tenancy.WithScope(context.Background(), tenancy.Scope{Domain: tenancy.Domain{ID: tenancy.OfficialID}, UserID: id})
	}
	ownerCtx, editorCtx := as(owner.ID), as(editor.ID)
	var id string
	if err := db.Pool.QueryRowContext(ctx, "INSERT INTO problems(domain_id,title,owner_id,visibility) VALUES($1,'Draft',$2,'private') RETURNING id", tenancy.OfficialID, owner.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.ExecContext(ctx, "INSERT INTO problem_access(domain_id,problem_id,user_id,role) VALUES($1,$2,$3,'editor')", tenancy.OfficialID, id, editor.ID); err != nil {
		t.Fatal(err)
	}
	store, err := filesystem.NewBlobStore(t.TempDir(), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	repo := authoringpg.NewRevisionRepository(db, store)
	copy, err := repo.Open(ownerCtx, id)
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for name, body := range map[string]string{"problem.yaml": "problem_format_version: 2025-09\nname: Imported\nlimits:\n  time_limit: 1\n  memory: 256\n", "statement/problem.en.md": "Imported statement\n", "data/secret/1.in": "1 2\n", "data/secret/1.ans": "3\n", "input_validators/main.cpp": "int main(){return 42;}\n", "submissions/accepted/main.cpp": "int main(){return 0;}\n"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	preview, err := repo.PreviewImport(ownerCtx, id, copy.ETag, data.Bytes(), domain.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := repo.WorkingCopy(ownerCtx, id)
	if err != nil || unchanged.ETag != copy.ETag {
		t.Fatal("preview changed the working copy")
	}
	if _, err := repo.Import(editorCtx, id, preview.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other editor read private import: %v", err)
	}
	if _, err := repo.ApplyImport(editorCtx, id, preview.ID, copy.ETag); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other editor applied private import: %v", err)
	}
	changed := copy.Tree
	changed.Entries = append([]domain.TreeEntry{}, copy.Tree.Entries...)
	changed.Entries[0].Attributes = map[string]string{"label": "local change"}
	copy, err = repo.Save(ownerCtx, id, copy.ETag, changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyImport(ownerCtx, id, preview.ID, preview.ETag); !errors.Is(err, domain.ErrWorkingCopyConflict) {
		t.Fatalf("stale preview replaced changes: %v", err)
	}
	preview, err = repo.PreviewImport(ownerCtx, id, copy.ETag, data.Bytes(), domain.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	copy, err = repo.ApplyImport(ownerCtx, id, preview.ID, copy.ETag)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := repo.ApplyImport(ownerCtx, id, preview.ID, preview.ETag)
	if err != nil || repeated.ETag != copy.ETag {
		t.Fatal("apply retry was not idempotent")
	}
	history, err := repo.History(ownerCtx, id, 0, 30)
	if err != nil || len(history) != 0 {
		t.Fatal("import implicitly committed")
	}
	report, err := repo.Inspect(ownerCtx, id, 0, copy.ETag)
	if err != nil || !report.CanBuild {
		t.Fatalf("imported source is not structurally usable: %+v %v", report, err)
	}
	export, err := repo.ExportPackage(ownerCtx, id, 0, "vertex")
	if err != nil {
		t.Fatal(err)
	}
	_, reader, err := repo.Blob(ownerCtx, id, export.File.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, reader, err := repo.Blob(editorCtx, id, export.File.SHA256); !errors.Is(err, domain.ErrNotFound) {
		if reader != nil {
			reader.Close()
		}
		t.Fatalf("private export leaked: %v", err)
	}
	native, err := repo.PreviewImport(ownerCtx, id, copy.ETag, archive, domain.ImportOptions{Format: "vertex"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := copy.Tree.Hash()
	after, _ := native.Plan.Tree.Hash()
	if before != after {
		t.Fatal("native export/import changed source")
	}

	// Database simulation for export authorization/integrity; actual native
	// execution is covered separately by TestEndToEndFrozenAuthoringCheck.
	artifactFiles := testExportArtifact{"1.in": []byte("1 2\n"), "1.out": []byte("3\n")}
	repo = authoringpg.NewRevisionRepository(db, store, artifactFiles)
	service := authoringapp.NewWorkbench(repo)
	for _, entry := range copy.Tree.Entries {
		if entry.Kind != domain.EntryTest {
			continue
		}
		view, err := service.Material(ownerCtx, id, entry.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		meta, err := service.Material(ownerCtx, id, "problem", 0)
		if err != nil {
			t.Fatal(err)
		}
		view.Test.Answer = domain.TestAnswer{Kind: "solution", Solution: meta.Metadata.MainSolution}
		encoded, _ := json.Marshal(view.Test)
		text := string(encoded)
		copy, err = service.SaveEntry(ownerCtx, id, copy.ETag, entry, &text)
		if err != nil {
			t.Fatal(err)
		}
	}
	check, err := repo.StartCheck(ownerCtx, id, domain.CheckSelection{ETag: copy.ETag})
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot, err := domain.InspectMaterials(copy.Tree, func(ref domain.BlobRef) ([]byte, error) {
		reader, err := store.Open(ctx, id, ref)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	})
	if err != nil {
		t.Fatal(err)
	}
	toolchain := strings.Repeat("a", 64)
	manifest := domain.CheckArtifact{SchemaVersion: 1, Snapshot: *snapshot, ToolchainKey: toolchain, Tests: []domain.ArtifactTest{{ID: snapshot.Tests[0].ID, Input: domain.Reference(artifactFiles["1.in"]), Answer: domain.Reference(artifactFiles["1.out"])}}}
	encoded, _ := json.Marshal(manifest)
	if _, err := db.Pool.ExecContext(ctx, "UPDATE problem_build_jobs SET state='succeeded',toolchain_key=$2,package_manifest=$3,package_path=$4,package_cases=1,finished_at=now() WHERE id=$1", check.ID, toolchain, encoded, id+"/"+strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ExportPackage(ownerCtx, id, 0, "kattis-2025-09"); err != nil {
		t.Fatalf("matching generated artifact export: %v", err)
	}
	artifactFiles["1.out"] = []byte("tampered")
	if _, err := repo.ExportPackage(ownerCtx, id, 0, "kattis-2025-09"); err == nil {
		t.Fatal("tampered artifact exported")
	}
	artifactFiles["1.out"] = []byte("3\n")
	export, err = repo.ExportPackage(ownerCtx, id, 0, "vertex")
	if err != nil {
		t.Fatal(err)
	}
	_, reader, err = repo.Blob(ownerCtx, id, export.File.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	archive, err = io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	editorCopy, err := repo.Open(editorCtx, id)
	if err != nil {
		t.Fatal(err)
	}
	editorPreview, err := repo.PreviewImport(editorCtx, id, editorCopy.ETag, archive, domain.ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	editorCopy, err = repo.ApplyImport(editorCtx, id, editorPreview.ID, editorCopy.ETag)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ExportPackage(editorCtx, id, 0, "kattis-2025-09"); err == nil {
		t.Fatal("editor reused owner's unshared private check")
	}
	if _, err := repo.Commit(ownerCtx, id, domain.CommitInput{ETag: copy.ETag, RequestID: "share-checked-package", Message: "Share checked source"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ExportPackage(editorCtx, id, 0, "kattis-2025-09"); err != nil {
		t.Fatalf("committed check could not be reused: %v", err)
	}
	latest, err := repo.WorkingCopy(ownerCtx, id)
	if err != nil {
		t.Fatal(err)
	}
	scoring, err := service.Material(ownerCtx, id, snapshot.Tests[0].ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	scoring.Test.Points = 0.5
	scoringData, _ := json.Marshal(scoring.Test)
	scoringText := string(scoringData)
	latest, err = service.SaveEntry(ownerCtx, id, latest.ETag, scoring.Entry, &scoringText)
	if err != nil {
		t.Fatal(err)
	}
	scoringCommit, err := repo.Commit(ownerCtx, id, domain.CommitInput{ETag: latest.ETag, RequestID: "scoring-draft", Message: "Configure point scoring"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PublishCommit(ownerCtx, id, domain.CommitPublication{Revision: scoringCommit.Commit.Revision, CheckID: check.ID, ExpectedVersion: 0}); err == nil || !strings.Contains(err.Error(), "逐点计分") {
		t.Fatalf("unsupported scoring did not have a publication guard: %v", err)
	}
}

type testExportArtifact map[string][]byte

func (files testExportArtifact) Read(_ context.Context, _, _, name string, _ int64) ([]byte, error) {
	return files[name], nil
}
