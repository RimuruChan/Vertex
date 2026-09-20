package packages

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func TestStandardExportDoesNotSilentlyDropScoring(t *testing.T) {
	store, blobs := testStore(t)
	plan, err := Import(packageZIP(t, kattisFixture("2025-09")), domain.ImportOptions{}, store)
	if err != nil {
		t.Fatal(err)
	}
	for index, entry := range plan.Tree.Entries {
		if entry.Kind != domain.EntryTest {
			continue
		}
		view, err := domain.DecodeMaterial(entry, blobs[entry.Blob.SHA256])
		if err != nil {
			t.Fatal(err)
		}
		view.Test.Points = 25
		data, err := json.Marshal(view.Test)
		if err != nil {
			t.Fatal(err)
		}
		plan.Tree.Entries[index].Blob, err = store(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	read := func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	}
	if _, err := Export(plan.Tree, ExportOptions{Format: "kattis-2025-09"}, read); err == nil || !strings.Contains(err.Error(), "评测规则") {
		t.Fatalf("scoring was silently dropped: %v", err)
	}
	if _, err := Export(plan.Tree, ExportOptions{Format: "vertex"}, read); err != nil {
		t.Fatalf("native preservation failed: %v", err)
	}
}

func TestNativeArchiveRoundTripIsDeterministic(t *testing.T) {
	store, blobs := testStore(t)
	plan, err := Import(packageZIP(t, kattisFixture("2025-09")), domain.ImportOptions{}, store)
	if err != nil {
		t.Fatal(err)
	}
	read := func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	}
	first, err := Export(plan.Tree, ExportOptions{Format: "vertex"}, read)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Export(plan.Tree, ExportOptions{Format: "vertex"}, read)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Data, second.Data) {
		t.Fatal("identical source exported different archive bytes")
	}
	roundtrip, err := Import(first.Data, domain.ImportOptions{}, store)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := plan.Tree.Hash()
	after, _ := roundtrip.Tree.Hash()
	if before != after {
		t.Fatal("native archive did not preserve exact source tree")
	}
}

func TestKattisRoundTripKeepsLimitsProtocolsAndAnswers(t *testing.T) {
	for _, version := range []string{"legacy-icpc", "2025-09"} {
		t.Run(version, func(t *testing.T) {
			store, blobs := testStore(t)
			first, err := Import(packageZIP(t, kattisFixture(version)), domain.ImportOptions{TimeLimitMs: 1500}, store)
			if err != nil {
				t.Fatal(err)
			}
			exported, err := Export(first.Tree, ExportOptions{Format: "kattis-" + version, Identity: "stable-problem-identity"}, func(ref domain.BlobRef) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			second, err := Import(exported.Data, domain.ImportOptions{}, store)
			if err != nil {
				t.Fatal(err)
			}
			readMeta := func(tree domain.ContentTree) domain.PackageMetadata {
				t.Helper()
				for _, entry := range tree.Entries {
					if entry.ID == "problem" {
						var result domain.PackageMetadata
						if err := json.Unmarshal(blobs[entry.Blob.SHA256], &result); err != nil {
							t.Fatal(err)
						}
						return result
					}
				}
				t.Fatal("missing metadata")
				return domain.PackageMetadata{}
			}
			a, b := readMeta(first.Tree), readMeta(second.Tree)
			if a.Title != b.Title || a.TimeLimitMs != b.TimeLimitMs || a.MemoryLimitKB != b.MemoryLimitKB || a.Comparison != b.Comparison || len(b.Requirements) != 0 {
				t.Fatalf("semantic roundtrip changed: %+v -> %+v", a, b)
			}
			found := false
			for _, entry := range second.Tree.Entries {
				if entry.Kind == domain.EntryAnswer && string(blobs[entry.Blob.SHA256]) == "03\n" {
					found = true
				}
			}
			if !found {
				t.Fatal("imported answer formatting was lost")
			}
		})
	}
}
