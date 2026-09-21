package packages

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// The frontend test writes this optional fixture using the same encoder as the
// browser download. This gate checks cross-language archive interoperability.
func TestBrowserNativeArchiveInterop(t *testing.T) {
	name := os.Getenv("VERTEX_MOCK_ARCHIVE")
	if name == "" {
		t.Skip("VERTEX_MOCK_ARCHIVE not provided")
	}
	archive, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	store, blobs := testStore(t)
	plan, err := Import(archive, domain.ImportOptions{}, store)
	if err != nil {
		t.Fatal(err)
	}
	report, snapshot, err := domain.InspectMaterials(plan.Tree, func(ref domain.BlobRef) ([]byte, error) { return blobs[ref.SHA256], nil })
	if err != nil || !report.CanBuild || report.TestCount != 6 || snapshot.Metadata.Title != "出题工作台 · A + B" {
		t.Fatalf("browser package lost materials: %+v %v", report, err)
	}
	exported, err := Export(plan.Tree, ExportOptions{Format: "vertex"}, func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Import(exported.Data, domain.ImportOptions{}, store)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := plan.Tree.Hash()
	after, _ := restored.Tree.Hash()
	if before != after {
		t.Fatal("backend changed the browser content tree")
	}
	if target := os.Getenv("VERTEX_SERVER_ARCHIVE"); target != "" {
		if err := os.WriteFile(target, exported.Data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
