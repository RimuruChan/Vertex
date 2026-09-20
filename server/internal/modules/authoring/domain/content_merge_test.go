package domain

import (
	"errors"
	"testing"
)

func TestMergeTreesIndependentEditsAndRename(t *testing.T) {
	a, b := treeEntry("a", "a.cpp", "a"), treeEntry("b", "b.cpp", "b")
	base := ContentTree{Entries: []TreeEntry{a, b}}
	local, _ := base.Canonical()
	remote, _ := base.Canonical()
	local.Entries[0].Path = "renamed.cpp"
	local.Entries[0].Attributes["role"] = "solution"
	remote.Entries[0].Blob = Reference([]byte("new a"))
	remote.Entries[0].Attributes["language"] = "cpp"
	remote.Entries[1].Blob = Reference([]byte("new b"))
	result, err := MergeTrees(base, local, remote, nil)
	if err != nil || len(result.Conflicts) != 0 {
		t.Fatalf("independent edits conflict: %+v %v", result, err)
	}
	got := result.Tree.Entries[0]
	if got.Path != "renamed.cpp" || got.Blob != remote.Entries[0].Blob || got.Attributes["role"] != "solution" || got.Attributes["language"] != "cpp" {
		t.Fatalf("lost merged edit: %+v", got)
	}
	got.Attributes["role"] = "changed"
	if local.Entries[0].Attributes["role"] != "solution" {
		t.Fatal("merge aliases input")
	}
	changes, err := DiffTrees(base, result.Tree)
	if err != nil || len(changes) != 2 || changes[0].Kind != "renamed" {
		t.Fatalf("bad diff: %+v %v", changes, err)
	}
}

func TestMergeTreesStructuralConflicts(t *testing.T) {
	baseEntry := treeEntry("a", "a.cpp", "base")
	base := ContentTree{Entries: []TreeEntry{baseEntry}}
	modified := treeEntry("a", "a.cpp", "modified")
	for _, test := range []struct {
		name                string
		base, local, remote ContentTree
		kind                string
	}{
		{"delete-edit", base, ContentTree{}, ContentTree{Entries: []TreeEntry{modified}}, "delete-modify"},
		{"edit-delete", base, ContentTree{Entries: []TreeEntry{modified}}, ContentTree{}, "delete-modify"},
		{"add-add", ContentTree{}, base, ContentTree{Entries: []TreeEntry{modified}}, "add-add"},
		{"rename-rename", base, ContentTree{Entries: []TreeEntry{treeEntry("a", "b.cpp", "base")}}, ContentTree{Entries: []TreeEntry{treeEntry("a", "c.cpp", "base")}}, "both-modified"},
		{"path-collision", ContentTree{}, base, ContentTree{Entries: []TreeEntry{treeEntry("b", "A.cpp", "other")}}, "path-collision"},
		{"file-directory", ContentTree{}, base, ContentTree{Entries: []TreeEntry{treeEntry("b", "a.cpp/nested", "other")}}, "path-collision"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := MergeTrees(test.base, test.local, test.remote, nil)
			if err != nil || len(result.Conflicts) == 0 || result.Conflicts[0].Kind != test.kind {
				t.Fatalf("lost conflict: %+v %v", result, err)
			}
		})
	}
}

func TestMergeTreesDeletionAndEmptyContentDiffer(t *testing.T) {
	base := ContentTree{Entries: []TreeEntry{treeEntry("a", "a.cpp", "base")}}
	local := ContentTree{Entries: []TreeEntry{treeEntry("a", "a.cpp", "")}}
	result, err := MergeTrees(base, local, base, nil)
	if err != nil || len(result.Tree.Entries) != 1 || result.Tree.Entries[0].Blob.Bytes != 0 {
		t.Fatalf("empty file became deletion: %+v %v", result, err)
	}
	result, err = MergeTrees(base, ContentTree{}, base, nil)
	if err != nil || len(result.Tree.Entries) != 0 || len(result.Conflicts) != 0 {
		t.Fatalf("clean deletion failed: %+v %v", result, err)
	}
	result, err = MergeTrees(base, local, local, nil)
	if err != nil || len(result.Conflicts) != 0 {
		t.Fatal("identical changes conflict")
	}
}

func TestMergeTreesAttributesDistinguishMissingAndEmpty(t *testing.T) {
	entry := treeEntry("a", "a.cpp", "base")
	entry.Attributes["label"] = "old"
	base := ContentTree{Entries: []TreeEntry{entry}}
	local, _ := base.Canonical()
	remote, _ := base.Canonical()
	delete(local.Entries[0].Attributes, "label")
	remote.Entries[0].Attributes["label"] = ""
	result, err := MergeTrees(base, local, remote, nil)
	if err != nil || len(result.Conflicts) != 1 || result.Conflicts[0].Field != "attributes.label" {
		t.Fatalf("lost missing/empty conflict: %+v %v", result, err)
	}
}

func TestMergeTreesNeverTextMergesTestData(t *testing.T) {
	for _, kind := range []string{EntryInput, EntryAnswer, EntryAsset, EntryResource} {
		entry := treeEntry("a", "a.in", "base")
		entry.Kind = kind
		base := ContentTree{Entries: []TreeEntry{entry}}
		local, _ := base.Canonical()
		remote, _ := base.Canonical()
		local.Entries[0].Blob = Reference([]byte("local"))
		remote.Entries[0].Blob = Reference([]byte("remote"))
		result, err := MergeTrees(base, local, remote, func(string, BlobRef, BlobRef, BlobRef) (BlobRef, bool, error) {
			t.Fatal("binary/test data reached text merger")
			return BlobRef{}, false, nil
		})
		if err != nil || len(result.Conflicts) != 1 {
			t.Fatalf("bad binary merge %+v %v", result, err)
		}
	}
}

func TestMergeTreesBlobFailureDoesNotBecomeSuccess(t *testing.T) {
	base := ContentTree{Entries: []TreeEntry{treeEntry("a", "a.cpp", "base")}}
	local := ContentTree{Entries: []TreeEntry{treeEntry("a", "a.cpp", "local")}}
	remote := ContentTree{Entries: []TreeEntry{treeEntry("a", "a.cpp", "remote")}}
	expected := errors.New("storage unavailable")
	_, err := MergeTrees(base, local, remote, func(string, BlobRef, BlobRef, BlobRef) (BlobRef, bool, error) { return BlobRef{}, false, expected })
	if !errors.Is(err, expected) {
		t.Fatalf("storage failure was swallowed: %v", err)
	}
}

func TestMergeJSONFieldsAndConflicts(t *testing.T) {
	for _, test := range []struct {
		name, base, local, remote, expected string
		ok                                  bool
	}{
		{"independent", `{"limits":{"time":1,"memory":2}}`, `{"limits":{"time":3,"memory":2}}`, `{"limits":{"time":1,"memory":4}}`, `{"limits":{"memory":4,"time":3}}`, true},
		{"null-delete", `{"a":1}`, `{}`, `{"a":null}`, "", false},
		{"same-add", `{}`, `{"a":1}`, `{"a":1}`, `{"a":1}`, true},
		{"both-add", `{}`, `{"a":1}`, `{"b":2}`, `{"a":1,"b":2}`, true},
		{"large-integer", `{"a":9007199254740993,"b":1}`, `{"a":9007199254740993,"b":2}`, `{"a":9007199254740995,"b":1}`, `{"a":9007199254740995,"b":2}`, true},
		{"order-conflict", `{"order":[1,2,3]}`, `{"order":[2,1,3]}`, `{"order":[1,3,2]}`, "", false},
		{"invalid", `{}`, `{} {}`, `{}`, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := MergeJSON([]byte(test.base), []byte(test.local), []byte(test.remote))
			if ok != test.ok || ok && string(got) != test.expected {
				t.Fatalf("got %s %v, want %s %v", got, ok, test.expected, test.ok)
			}
		})
	}
}
