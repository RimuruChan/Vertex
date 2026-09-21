package domain

import (
	"strings"
	"testing"
)

func treeEntry(id, name, content string) TreeEntry {
	return TreeEntry{ID: id, Path: name, Kind: EntrySource, Blob: Reference([]byte(content)), Attributes: map[string]string{}}
}

func TestContentTreeCanonicalIdentity(t *testing.T) {
	a := treeEntry("a", "sources/a.cpp", "a\n")
	a.Attributes = map[string]string{"language": "cpp", "role": "solution"}
	b := treeEntry("b", "sources/b.cpp", "b\n")
	tree := ContentTree{Entries: []TreeEntry{b, a}}
	first, err := tree.Hash()
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := tree.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	second, err := canonical.Hash()
	if err != nil || first != second {
		t.Fatalf("unstable tree hash %s %s: %v", first, second, err)
	}
	if tree.Entries[0].ID != "b" {
		t.Fatal("hash changed caller order")
	}
	a.Attributes["role"] = "generator"
	if canonical.Entries[0].Attributes["role"] != "solution" {
		t.Fatal("canonical manifest aliases caller map")
	}
	third, _ := tree.Hash()
	if first == third {
		t.Fatal("role changes must invalidate content identity")
	}
	empty, _ := (ContentTree{}).Hash()
	emptySlice, _ := (ContentTree{Entries: []TreeEntry{}}).Hash()
	if empty != emptySlice {
		t.Fatal("nil and empty trees differ")
	}
}

func TestContentTreeRejectsAmbiguousPaths(t *testing.T) {
	for _, name := range []string{"../secret", "/tmp/a", "C:/a", `a\b`, "a/../b", "a//b", "a/./b", "a\x00b", "a/CON.txt", "a/com1", "a/LPT².cpp", "a/file.", "a/file ", "a/?", ".", ".."} {
		t.Run(name, func(t *testing.T) {
			if ValidatePackagePath(name) == nil {
				t.Fatalf("accepted %q", name)
			}
		})
	}
	for _, names := range [][2]string{{"a.cpp", "A.cpp"}, {"test", "test/1.in"}, {"A", "a/b"}} {
		_, err := (ContentTree{Entries: []TreeEntry{treeEntry("a", names[0], ""), treeEntry("b", names[1], "")}}).Hash()
		if err == nil {
			t.Fatalf("accepted colliding paths %v", names)
		}
	}
	if err := ValidatePackagePath("statement/中文.md"); err != nil {
		t.Fatal(err)
	}
	entry := treeEntry("same", "a.cpp", "")
	if _, err := (ContentTree{Entries: []TreeEntry{entry, entry}}).Hash(); err == nil {
		t.Fatal("accepted duplicate IDs")
	}
	entry.Blob.SHA256 = strings.Repeat("z", 64)
	if _, err := (ContentTree{Entries: []TreeEntry{entry}}).Hash(); err == nil {
		t.Fatal("accepted invalid digest")
	}
}

func TestBlobReferenceVerifiesBytes(t *testing.T) {
	ref := Reference([]byte("abc"))
	if ValidateBlob(ref, []byte("abc")) != nil {
		t.Fatal("valid blob rejected")
	}
	if ValidateBlob(ref, []byte("abd")) == nil {
		t.Fatal("incorrect content accepted")
	}
	ref.Bytes++
	if ValidateBlob(ref, []byte("abc")) == nil {
		t.Fatal("incorrect length accepted")
	}
}
