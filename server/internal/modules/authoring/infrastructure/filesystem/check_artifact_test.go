package filesystem

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func TestCopyCheckedArtifactUsesReleaseMetadataAndIndependentFiles(t *testing.T) {
	manifest, files := artifactFixture(t)
	manifest.Snapshot.Metadata.Title = "private check title must not be copied"
	root := t.TempDir()
	publisher := NewTestdataPublisher(root)
	source, err := publisher.Publish("source", artifactZip(t, manifest, files))
	if err != nil {
		t.Fatal(err)
	}
	published := manifest.Snapshot
	published.TreeHash = strings.Repeat("b", 64)
	published.Metadata.Title = "Published title"
	copy, err := publisher.CloneChecked(context.Background(), "source", "destination", *source, published)
	if err != nil {
		t.Fatal(err)
	}
	if copy.Artifact.Snapshot.TreeHash != published.TreeHash || copy.Artifact.Snapshot.Metadata.Title != published.Metadata.Title {
		t.Fatal("copy retained uncommitted metadata")
	}
	data, err := publisher.Read(context.Background(), "destination", copy.StoragePath, "artifact.json", 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("private check title")) {
		t.Fatal("copied manifest leaked private metadata")
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(source.StoragePath), "1.out"), []byte("changed source\n"), 0644); err != nil {
		t.Fatal(err)
	}
	answer, err := publisher.Read(context.Background(), "destination", copy.StoragePath, "1.out", 100)
	if err != nil || string(answer) != files["1.out"] {
		t.Fatal("copy still depended on source file")
	}
	if _, err := publisher.CloneChecked(context.Background(), "source", "third", *source, published); err == nil {
		t.Fatal("tampered source artifact copied")
	}
	published.Metadata.TimeLimitMs++
	if _, err := publisher.CloneChecked(context.Background(), "source", "fourth", *source, published); err == nil {
		t.Fatal("different judging policy copied as already checked")
	}
}

func artifactFixture(t *testing.T) (domain.CheckArtifact, map[string]string) {
	t.Helper()
	input, answer, source := "1\n", "2\n", "int main(){return 0;}\n"
	snapshot := domain.CheckSnapshot{SchemaVersion: 1, TreeHash: strings.Repeat("a", 64), PolicyVersion: domain.CheckPolicyVersion, Metadata: domain.PackageMetadata{SchemaVersion: 1, Title: "Fixture", JudgeType: "normal", TimeLimitMs: 1000, MemoryLimitKB: 262144, MainSolution: "reference", Comparison: domain.OutputComparison{Kind: "exact"}}, Tests: []domain.SnapshotTest{{ID: "test"}}, Programs: []domain.SnapshotProgram{{ID: "reference", EntryPoint: "src/main.cpp", Definition: domain.ProgramMaterial{SchemaVersion: 1, Name: "Reference", Language: "cpp", Protocol: "stdio", Role: "solution"}, Files: []domain.SnapshotFile{{ID: "source", Path: "src/main.cpp", Blob: domain.Reference([]byte(source))}}}}, Groups: []domain.SnapshotGroup{}}
	snapshot.DataHash, _ = snapshot.DataFingerprint()
	manifest := domain.CheckArtifact{SchemaVersion: 1, ToolchainKey: strings.Repeat("b", 64), Snapshot: snapshot, Tests: []domain.ArtifactTest{{ID: "test", Input: domain.Reference([]byte(input)), Answer: domain.Reference([]byte(answer))}}}
	return manifest, map[string]string{"1.in": input, "1.out": answer, "programs/reference/src/main.cpp": source}
}

func TestRenderedStatementArtifactBinding(t *testing.T) {
	manifest, files := artifactFixture(t)
	manifest.Snapshot.Statements = []domain.SnapshotStatement{{ID: "wording", Language: "en", Source: domain.SnapshotFile{ID: "wording", Path: "statement/problem.tex", Blob: domain.Reference([]byte("TeX source"))}}}
	manifest.Snapshot.DataHash, _ = manifest.Snapshot.DataFingerprint()
	pdf := "%PDF-1.4\nfixture"
	manifest.Statements = []domain.ArtifactStatement{{ID: "wording", PDF: domain.Reference([]byte(pdf))}}
	files["statements/wording.pdf"] = pdf
	publisher := NewTestdataPublisher(t.TempDir())
	if _, err := publisher.Publish("valid", artifactZip(t, manifest, files)); err != nil {
		t.Fatal(err)
	}
	files["statements/wording.pdf"] = "%PDF-tampered"
	if _, err := publisher.Publish("corrupt", artifactZip(t, manifest, files)); err == nil {
		t.Fatal("tampered PDF accepted")
	}
	delete(files, "statements/wording.pdf")
	manifest.Statements = nil
	if _, err := publisher.Publish("missing", artifactZip(t, manifest, files)); err == nil {
		t.Fatal("missing compiled statement accepted")
	}
}

func artifactZip(t *testing.T, manifest domain.CheckArtifact, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := writer.Create("artifact.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(data); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
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
	return buffer.Bytes()
}

func TestFrozenArtifactIntegrity(t *testing.T) {
	manifest, files := artifactFixture(t)
	publisher := NewTestdataPublisher(t.TempDir())
	upload, err := publisher.Publish("problem", artifactZip(t, manifest, files))
	if err != nil {
		t.Fatal(err)
	}
	if upload.Artifact == nil || upload.CaseCount != 1 || upload.Checker != "exact" {
		t.Fatalf("bad artifact metadata %+v", upload)
	}
	original := upload.SHA256
	files["programs/reference/src/main.cpp"] = "int main(){return 1;}\n"
	if _, err := publisher.Publish("problem", artifactZip(t, manifest, files)); err == nil {
		t.Fatal("changed source accepted without matching digest")
	}
	manifest.Snapshot.Programs[0].Files[0].Blob = domain.Reference([]byte(files["programs/reference/src/main.cpp"]))
	manifest.Snapshot.DataHash, _ = manifest.Snapshot.DataFingerprint()
	changed, err := publisher.Publish("problem", artifactZip(t, manifest, files))
	if err != nil {
		t.Fatal(err)
	}
	if changed.SHA256 == original {
		t.Fatal("nested source edit did not change stored artifact identity")
	}
}

func TestFrozenArtifactRejectsUndeclaredAndMissingFiles(t *testing.T) {
	for _, mode := range []string{"undeclared", "missing", "traversal", "fingerprint"} {
		t.Run(mode, func(t *testing.T) {
			manifest, files := artifactFixture(t)
			switch mode {
			case "undeclared":
				files["extra.txt"] = "unexpected"
			case "missing":
				delete(files, "1.out")
			case "traversal":
				files["../outside"] = "escape"
			case "fingerprint":
				manifest.Snapshot.Metadata.TimeLimitMs++
			}
			if _, err := NewTestdataPublisher(t.TempDir()).Publish("problem", artifactZip(t, manifest, files)); err == nil {
				t.Fatal("invalid frozen artifact accepted")
			}
		})
	}
}
