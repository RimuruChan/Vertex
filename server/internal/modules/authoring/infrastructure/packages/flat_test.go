package packages

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func TestDataImportMergesWithoutReplacingProblem(t *testing.T) {
	put, blobs := testStore(t)
	base, err := Import(packageZIP(t, kattisFixture("2025-09")), domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"game10.in": "2 5\n", "game10.out": "7\n", "game2.in": "1 2\n", "game2.out": "03\n", "config.yml": "game2.in:\n  timeLimit: 2500\n  memoryLimit: 512000\n"}
	data, err := Import(packageZIP(t, files), domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	if data.Scope != "data" || data.Format != "luogu-data" {
		t.Fatal("not classified as data-only")
	}
	read := func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	}
	if err := MergeData(base.Tree, data, read, put); err != nil {
		t.Fatal(err)
	}
	inspection, snapshot, err := domain.InspectMaterials(data.Tree, func(ref domain.BlobRef) ([]byte, error) { return blobs[ref.SHA256], nil })
	if err != nil || !inspection.CanBuild {
		t.Fatalf("merged inspection: %+v %v", inspection, err)
	}
	if snapshot.Metadata.Title != "Sum" || snapshot.Metadata.TimeLimitMs != 1000 || len(snapshot.Tests) != 4 || snapshot.Tests[2].Definition.Name != "game2" || snapshot.Tests[2].Definition.TimeLimitMs != 2500 || snapshot.Tests[2].Definition.MemoryLimitKB != 512000 {
		t.Fatalf("merged content: %+v", snapshot)
	}
	for _, old := range base.Tree.Entries {
		if old.ID == "problem" {
			continue
		}
		found := false
		for _, entry := range data.Tree.Entries {
			if old.ID == entry.ID {
				found = old.Blob == entry.Blob && old.Path == entry.Path
			}
		}
		if !found {
			t.Fatalf("existing material changed: %s", old.Path)
		}
	}
	files["game2.out"] = "3\n"
	updated, err := Import(packageZIP(t, files), domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	if err := MergeData(data.Tree, updated, read, put); err != nil {
		t.Fatal(err)
	}
	if len(updated.Tree.Entries) != len(data.Tree.Entries) {
		t.Fatal("reimport duplicated entries")
	}
	for _, entry := range updated.Tree.Entries {
		if entry.ID == "problem" {
			var meta domain.PackageMetadata
			json.Unmarshal(blobs[entry.Blob.SHA256], &meta)
			if len(meta.TestOrder) != 4 {
				t.Fatal("reimport duplicated tests")
			}
		}
		if entry.Path == "game2.out" && string(blobs[entry.Blob.SHA256]) != "3\n" {
			t.Fatal("reimport failed to update answer")
		}
	}
}

func TestDataImportRejectsAmbiguityAndRetainsUnsupportedConfig(t *testing.T) {
	put, blobs := testStore(t)
	for _, files := range []map[string]string{
		{"1.in": "a"}, {"1.in": "a", "1.out": "b", "1.ans": "c"}, {"1.in": "a", "1.out": "b", "2.out": "c"},
		{"1.in": "a", "1.out": "b", "config.yml": "1.in:\n  timeLimit: 1.5\n"},
		{"1.in": "a", "1.out": "b", "config.yml": "1.in: {}\n1.out: {}\n"},
	} {
		if _, err := Import(packageZIP(t, files), domain.ImportOptions{}, put); err == nil {
			t.Fatalf("ambiguous data accepted: %+v", files)
		}
	}
	plan, err := Import(packageZIP(t, map[string]string{"1.in": "a", "1.out": "b", "config.yml": "1.in:\n  subtaskId: 1\n  score: 30\n"}), domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range plan.Tree.Entries {
		if entry.ID == "problem" {
			var meta domain.PackageMetadata
			json.Unmarshal(blobs[entry.Blob.SHA256], &meta)
			if len(meta.Requirements) != 2 {
				t.Fatal("scoring semantics silently lost")
			}
		}
	}
}
