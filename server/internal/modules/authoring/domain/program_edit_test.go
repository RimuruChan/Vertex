package domain

import (
	"strings"
	"testing"
)

func TestProgramAggregateIsolatesSharedSources(t *testing.T) {
	code := TreeEntry{ID: "source", Kind: EntrySource, Path: "original/main.cpp", Blob: Reference([]byte("old")), Attributes: map[string]string{}}
	entry := TreeEntry{ID: "program", Kind: EntryProgram, Path: "vertex/programs/p.json", Blob: Reference([]byte("{}")), Attributes: map[string]string{}}
	definition := ProgramMaterial{SchemaVersion: 1, Name: "正确解", Role: "solution", Language: "cpp", Protocol: "stdio", Files: []string{code.ID}, EntryPoint: code.ID, Directory: "original", ExpectedVerdicts: []string{"Accepted"}}
	request := ProgramSaveInput{ID: entry.ID, BaseHash: entry.Blob.SHA256, Program: definition, Sources: []ProgramSourceEdit{{ID: code.ID, RelativeName: "main.cpp", BaseHash: code.Blob.SHA256, Blob: Reference([]byte("new"))}}}
	next, sources, remap, err := ArrangeProgram(ContentTree{Entries: []TreeEntry{entry, code}}, request, map[string]ProgramMaterial{"program": definition, "other": definition})
	if err != nil {
		t.Fatal(err)
	}
	if next.EntryPoint == code.ID || remap[code.ID] != next.EntryPoint || !strings.HasSuffix(sources[0].Path, "/main.cpp") {
		t.Fatalf("shared source was not isolated: %+v %+v", next, sources)
	}
}
func TestProgramAggregateRejectsUnlistedAndStaleSources(t *testing.T) {
	program := ProgramMaterial{SchemaVersion: 1, Name: "解答", Role: "solution", Language: "cpp", Protocol: "stdio", Files: []string{"source"}, EntryPoint: "source"}
	source := ProgramSourceEdit{ID: "other", RelativeName: "main.cpp", Blob: Reference([]byte("code"))}
	_, _, _, err := ArrangeProgram(ContentTree{}, ProgramSaveInput{ID: "p", Program: program, Sources: []ProgramSourceEdit{source}}, nil)
	if err == nil {
		t.Fatal("unlisted membership accepted")
	}
	source.ID = "source"
	source.RelativeName = "../private.cpp"
	if _, _, _, err := ArrangeProgram(ContentTree{}, ProgramSaveInput{ID: "p", Program: program, Sources: []ProgramSourceEdit{source}}, nil); err == nil {
		t.Fatal("traversal accepted")
	}
}
