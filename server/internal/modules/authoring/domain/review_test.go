package domain

import (
	"encoding/json"
	"testing"
)

func TestReviewUsesProgramOwnership(t *testing.T) {
	blobs := map[string][]byte{}
	entry := func(id, kind, path string, value any) TreeEntry {
		data, _ := json.Marshal(value)
		e := TreeEntry{ID: id, Kind: kind, Path: path, Blob: Reference(data), Attributes: map[string]string{}}
		blobs[e.Blob.SHA256] = data
		return e
	}
	program := entry("p", EntryProgram, "vertex/programs/p.json", map[string]any{"name": "正确参考解", "files": []string{"s"}})
	old := entry("s", EntrySource, "secret/tree/main.cpp", "old")
	next := entry("s", EntrySource, "secret/tree/main.cpp", "new")
	items, err := StructuredReview(ContentTree{Entries: []TreeEntry{program, old}}, ContentTree{Entries: []TreeEntry{program, next}}, func(e TreeEntry) ([]byte, error) { return blobs[e.Blob.SHA256], nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Kind != EntryProgram || items[0].Label != "正确参考解" || items[0].EntryIDs[0] != "s" {
		t.Fatalf("not semantic: %+v", items)
	}
}
func TestStatementReviewIgnoresCodeHeadings(t *testing.T) {
	sections := StatementSections("## 输入格式\n整数。\n```cpp\n# not heading\n```\n## 输出格式\n总和。")
	if _, exists := sections["not heading"]; exists {
		t.Fatal("code interpreted as structure")
	}
	if sections["输出格式"] != "总和。" {
		t.Fatal(sections)
	}
}

func TestStatementReviewIncludesTitle(t *testing.T) {
	sections := StatementSections("# New title\n\n## 输入格式\n整数。")
	if sections["题目标题"] != "New title" {
		t.Fatal(sections)
	}
}
