package packages

import (
	"bytes"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func TestStandardMarkdownOmitsOnlyDuplicateTitleAndKeepsSampleCommands(t *testing.T) {
	source := []byte("# Sum\n\nBody.\n\n{{nextsample}}\n\nAfter.\n")
	result, err := standardMarkdown(source, "Sum", "statement/problem.en.md", nil)
	if err != nil || string(result) != "Body.\n\n{{nextsample}}\n\nAfter.\n" {
		t.Fatalf("conversion lost content: %q %v", result, err)
	}
	if !bytes.HasPrefix(source, []byte("# Sum")) {
		t.Fatal("original source mutated")
	}
	result, err = standardMarkdown(source, "Other title", "statement/problem.en.md", nil)
	if err != nil || !bytes.Equal(result, source) {
		t.Fatal("unrelated heading removed")
	}
}

func TestKattisMarkdownReportsExternalAndUnrenderedContent(t *testing.T) {
	for _, source := range []string{"![remote](https://example.test/a.png)", "<script>bad()</script>", "![vector](diagram.svg)"} {
		files := kattisFixture("2025-09")
		files["statement/problem.en.md"] = source
		store, blobs := testStore(t)
		plan, err := Import(packageZIP(t, files), domain.ImportOptions{}, store)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, issue := range plan.Issues {
			if issue.Code == "kattis.markdown_presentation" && issue.Severity == "blocking" {
				found = true
			}
		}
		if !found {
			t.Fatalf("unrendered source had no diagnostic: %s", source)
		}
		for _, entry := range plan.Tree.Entries {
			if entry.Kind == domain.EntryStatement && string(blobs[entry.Blob.SHA256]) != source {
				t.Fatal("blocked original not preserved")
			}
		}
	}
}
