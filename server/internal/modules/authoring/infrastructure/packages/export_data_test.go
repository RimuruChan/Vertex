package packages

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func TestDataExportKeepsBytesOrderAndLimitsWithoutPrivatePrograms(t *testing.T) {
	store, blobs := testStore(t)
	fixture := kattisFixture("2025-09")
	fixture["data/sample/01.in"] = "\x00\xffbinary\r\n"
	plan, err := Import(packageZIP(t, fixture), domain.ImportOptions{}, store)
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
		view.Test.TimeLimitMs = 1500
		data, _ := json.Marshal(view.Test)
		plan.Tree.Entries[index].Blob, err = store(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
	}
	read := func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	}
	first, err := Export(plan.Tree, ExportOptions{Format: "luogu-data"}, read)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Export(plan.Tree, ExportOptions{Format: "luogu-data"}, read)
	if err != nil || !bytes.Equal(first.Data, second.Data) {
		t.Fatal("data export not deterministic")
	}
	archive, err := zip.NewReader(bytes.NewReader(first.Data), int64(len(first.Data)))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	if strings.Join(names, ",") != "0001.in,0001.out,0002.in,0002.out,config.yml" {
		t.Fatalf("non-data files included: %v", names)
	}
	back, err := Import(first.Data, domain.ImportOptions{}, store)
	if err != nil {
		t.Fatal(err)
	}
	if back.Scope != "data" || first.Issues[0].Code != "data.only" {
		t.Fatal("data archive pretended to be a full problem")
	}
	matched := false
	for _, entry := range back.Tree.Entries {
		if entry.Path == "0001.in" {
			matched = string(blobs[entry.Blob.SHA256]) == fixture["data/sample/01.in"]
		}
		if entry.Kind == domain.EntryTest {
			view, err := domain.DecodeMaterial(entry, blobs[entry.Blob.SHA256])
			if err != nil || view.Test.TimeLimitMs != 1500 {
				t.Fatal("per-case limit lost")
			}
		}
	}
	if !matched {
		t.Fatal("test order or binary bytes changed")
	}
}
