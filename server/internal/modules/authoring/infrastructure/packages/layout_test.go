package packages

import (
	"archive/zip"
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func TestStandardArchiveHasMatchingSingleRoot(t *testing.T) {
	for _, format := range []string{"kattis-legacy", "kattis-legacy-icpc", "kattis-2025-09", "domjudge"} {
		t.Run(format, func(t *testing.T) {
			result := adapterExport(t, "testlib", format)
			root := strings.TrimSuffix(result.Filename, ".zip")
			if !regexp.MustCompile(`^[a-z0-9]+$`).MatchString(root) {
				t.Fatalf("invalid standard package base name %q", root)
			}
			reader, err := zip.NewReader(bytes.NewReader(result.Data), int64(len(result.Data)))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, file := range reader.File {
				if !strings.HasPrefix(file.Name, root+"/") {
					t.Fatalf("file outside package root: %s", file.Name)
				}
				found = found || file.Name == root+"/problem.yaml"
			}
			if !found {
				t.Fatal("metadata is not directly inside the package root")
			}
		})
	}
}

func TestMultilingualNamesSurviveStandardRoundTrip(t *testing.T) {
	files := kattisFixture("2025-09")
	files["problem.yaml"] = "problem_format_version: 2025-09\nname:\n  en: Sum\n  zh: 两数之和\nlicense: cc0\ntype: pass-fail\nlimits:\n  memory: 256\n  time_limit: 1.5\n"
	files["statement/problem.zh.md"] = "# 两数之和\n\n输出两数的和。\n"
	put, blobs := testStore(t)
	plan, err := Import(packageZIP(t, files), domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	read := func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	}
	result, err := Export(plan.Tree, ExportOptions{Format: "kattis-2025-09", Identity: "multilingual"}, read)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := readArchive(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.selectRoot(); err != nil {
		t.Fatal(err)
	}
	data, err := archive.read("problem.yaml", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	config, err := readYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	names, ok := config["name"].(map[string]any)
	if !ok || len(names) != 2 || names["en"] != "Sum" || names["zh"] != "两数之和" {
		t.Fatalf("lost localized names: %+v", config["name"])
	}
	second, err := Import(result.Data, domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range second.Tree.Entries {
		if entry.Kind == domain.EntryStatement && entry.Attributes["language"] == "zh" {
			if entry.Attributes["title"] != "两数之和" {
				t.Fatal("localized statement title was discarded")
			}
			return
		}
	}
	t.Fatal("Chinese statement missing after import")
}

func TestStandardExportRejectsIgnoredIntegralFilename(t *testing.T) {
	files := kattisFixture("2025-09")
	files["attachments/diagram with spaces.png"] = "bytes"
	put, blobs := testStore(t)
	plan, err := Import(packageZIP(t, files), domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	read := func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	}
	if _, err := Export(plan.Tree, ExportOptions{Format: "kattis-2025-09"}, read); err == nil || !strings.Contains(err.Error(), "文件名") {
		t.Fatalf("silently exported ignored attachment: %v", err)
	}
	if _, err := Export(plan.Tree, ExportOptions{Format: "vertex"}, read); err != nil {
		t.Fatalf("native preservation failed: %v", err)
	}
}

func TestStandardExportDoesNotPromotePrivateFiles(t *testing.T) {
	put, blobs := testStore(t)
	plan, err := Import(packageZIP(t, kattisFixture("2025-09")), domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	read := func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	}
	ref, err := put(strings.NewReader("private jury notes"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []domain.TreeEntry{{ID: "private-asset", Path: "attachments/notes.txt", Kind: domain.EntryAsset, Blob: ref, Attributes: map[string]string{"visibility": "private"}}, {ID: "private-picture", Path: "statement/notes.png", Kind: domain.EntryResource, Blob: ref, Attributes: map[string]string{}}} {
		tree := plan.Tree
		tree.Entries = append(append([]domain.TreeEntry{}, tree.Entries...), entry)
		if _, err := Export(tree, ExportOptions{Format: "kattis-2025-09"}, read); err == nil || !strings.Contains(err.Error(), "私有") {
			t.Fatalf("private file was promoted to public material: %v", err)
		}
		if _, err := Export(tree, ExportOptions{Format: "vertex"}, read); err != nil {
			t.Fatalf("native preservation: %v", err)
		}
	}
}
