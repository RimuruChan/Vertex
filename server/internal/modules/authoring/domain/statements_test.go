package domain

import "testing"

func TestExecutableStatementSnapshotAndFingerprint(t *testing.T) {
	f := newInspectionFixture(t)
	for i := range f.tree.Entries {
		if f.tree.Entries[i].Kind == EntryStatement {
			f.tree.Entries[i].Attributes["format"] = "tex"
		}
	}
	f.put("image", "statement/image.png", EntryAsset, []byte("image"))
	f.put("private", "resources/private.tex", EntryResource, []byte("private"))
	f.put("hidden-image", "statement/hidden.png", EntryAsset, []byte("secret image"))
	for i := range f.tree.Entries {
		if f.tree.Entries[i].ID == "hidden-image" {
			f.tree.Entries[i].Attributes["visibility"] = "private"
		}
	}
	first, snapshot := f.inspect(t)
	if !first.CanBuild || len(snapshot.Statements) != 1 || len(snapshot.Statements[0].Files) != 1 || snapshot.Statements[0].Files[0].ID != "image" {
		t.Fatalf("unexpected public rendering inputs: %+v", snapshot.Statements)
	}
	if snapshot.Statements[0].Title != f.meta.Title {
		t.Fatal("default statement title not frozen")
	}
	f.meta.Title = "Updated title"
	f.document(t, "problem", "vertex/problem.json", EntryMetadata, f.meta)
	renamed, _ := f.inspect(t)
	if renamed.DataHash == first.DataHash {
		t.Fatal("changed fallback title reused an obsolete PDF")
	}
	f.put("image", "statement/image.png", EntryAsset, []byte("updated illustration"))
	second, _ := f.inspect(t)
	if first.DataHash == second.DataHash {
		t.Fatal("changed TeX illustration reused stale rendered PDF")
	}
	f.put("private", "resources/private.tex", EntryResource, []byte("different private text"))
	third, _ := f.inspect(t)
	if second.DataHash != third.DataHash {
		t.Fatal("unrelated private resource invalidated rendered presentation")
	}
	for i := range f.tree.Entries {
		if f.tree.Entries[i].Kind == EntryStatement {
			f.tree.Entries[i].Attributes["format"] = "markdown"
		}
	}
	_, plain := f.inspect(t)
	if len(plain.Statements) != 0 {
		t.Fatal("Markdown was scheduled for executable rendering")
	}
}
