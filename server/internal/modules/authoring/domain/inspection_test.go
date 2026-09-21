package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

type inspectionFixture struct {
	tree  ContentTree
	blobs map[string][]byte
	meta  PackageMetadata
}

func TestPublicationBlocksUnimplementedScoringAndStages(t *testing.T) {
	snapshot := CheckSnapshot{Tests: []SnapshotTest{{ID: "case", Definition: TestMaterial{Points: 0.5, IsPretest: true}}}, Groups: []SnapshotGroup{{ID: "group"}}}
	issues := snapshot.PublicationIssues()
	if len(issues) != 3 {
		t.Fatalf("unimplemented semantics would publish: %+v", issues)
	}
	snapshot.Tests[0].Definition.Points = 0
	snapshot.Tests[0].Definition.IsPretest = false
	snapshot.Groups = nil
	if len(snapshot.PublicationIssues()) != 0 {
		t.Fatal("ordinary pass-fail problem was blocked")
	}
}

func newInspectionFixture(t *testing.T) *inspectionFixture {
	t.Helper()
	f := &inspectionFixture{blobs: map[string][]byte{}}
	initial, err := InitialMaterials("Sum", "Find the sum.\n", "en", "", "normal", 1000, 262144)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range initial {
		f.tree.Entries = append(f.tree.Entries, item.Entry)
		f.blobs[item.Entry.Blob.SHA256] = item.Data
	}
	if err := json.Unmarshal(initial[0].Data, &f.meta); err != nil {
		t.Fatal(err)
	}
	f.meta.MainSolution = "reference"
	f.meta.TestOrder = []string{"sample", "secret"}
	f.document(t, "problem", "vertex/problem.json", EntryMetadata, f.meta)
	f.put("main", "programs/reference/main.cpp", EntrySource, []byte("#include \"helper.h\"\nint main() { return 0; }\n"))
	f.put("header", "programs/reference/helper.h", EntrySource, []byte("#pragma once\n"))
	f.document(t, "reference", "vertex/programs/reference.json", EntryProgram, ProgramMaterial{SchemaVersion: 1, Name: "Reference", Directory: "programs/reference", Role: "solution", Language: "cpp", Protocol: "stdio", Files: []string{"main", "header"}, EntryPoint: "main", ExpectedVerdicts: []string{"Accepted"}})
	for _, id := range f.meta.TestOrder {
		f.put(id+"-input", "data/"+id+".in", EntryInput, []byte("1 2\n"))
		f.put(id+"-answer", "data/"+id+".ans", EntryAnswer, []byte("3\n"))
		f.document(t, id, "vertex/tests/"+id+".json", EntryTest, TestMaterial{SchemaVersion: 1, Name: id, IsSample: id == "sample", Input: TestInput{Kind: "file", Entry: id + "-input"}, Answer: TestAnswer{Kind: "file", Entry: id + "-answer"}, Points: 0.5})
	}
	return f
}
func (f *inspectionFixture) put(id, name, kind string, data []byte) {
	ref := Reference(data)
	f.blobs[ref.SHA256] = data
	for i := range f.tree.Entries {
		if f.tree.Entries[i].ID == id {
			f.tree.Entries[i].Blob = ref
			return
		}
	}
	f.tree.Entries = append(f.tree.Entries, TreeEntry{ID: id, Path: name, Kind: kind, Blob: ref, Attributes: map[string]string{}})
}
func (f *inspectionFixture) document(t *testing.T, id, name, kind string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	f.put(id, name, kind, data)
}
func (f *inspectionFixture) inspect(t *testing.T) (MaterialInspection, *CheckSnapshot) {
	t.Helper()
	report, snapshot, err := InspectMaterials(f.tree, func(ref BlobRef) ([]byte, error) {
		// The inspector must not load large test files or source programs.
		for _, entry := range f.tree.Entries {
			if entry.Blob == ref && (entry.Kind == EntryInput || entry.Kind == EntryAnswer || entry.Kind == EntrySource) {
				t.Fatalf("inspector read raw material %s", entry.ID)
			}
		}
		return f.blobs[ref.SHA256], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return report, snapshot
}

func TestInspectionSealsDataAndProgramReferences(t *testing.T) {
	f := newInspectionFixture(t)
	report, snapshot := f.inspect(t)
	if !report.CanBuild || report.TestCount != 2 || report.SampleCount != 1 || report.ProgramCount != 1 {
		t.Fatalf("unexpected readiness %+v", report)
	}
	if snapshot.Programs[0].EntryPoint != "main.cpp" || len(snapshot.Programs[0].Files) != 2 {
		t.Fatalf("lost program layout: %+v", snapshot.Programs)
	}
	if snapshot.Tests[0].ID != "sample" || snapshot.Tests[0].Input == nil || snapshot.Tests[0].Answer == nil || snapshot.Tests[0].Definition.Points != 0.5 {
		t.Fatalf("lost imported answer or order: %+v", snapshot.Tests)
	}
	oldHash := report.DataHash
	f.put("statement-en", "statement/problem.en.md", EntryStatement, []byte("New explanation only.\n"))
	f.meta.Title = "A better title"
	f.meta.Source = "A source credit"
	f.document(t, "problem", "vertex/problem.json", EntryMetadata, f.meta)
	wording, _ := f.inspect(t)
	if wording.DataHash != oldHash || wording.TreeHash == report.TreeHash {
		t.Fatal("editorial edit invalidated data or failed to identify full tree")
	}
	f.put("secret-answer", "data/secret.ans", EntryAnswer, []byte("4\n"))
	changed, _ := f.inspect(t)
	if changed.DataHash == oldHash {
		t.Fatal("changed answer reused data fingerprint")
	}
	if snapshot.Tests[1].Answer.SHA256 != Reference([]byte("3\n")).SHA256 {
		t.Fatal("later edit changed frozen snapshot")
	}
}

func TestIdenticalDataProducesNonBlockingDuplicateHint(t *testing.T) {
	f := newInspectionFixture(t)
	report, _ := f.inspect(t)
	duplicates := 0
	for _, issue := range report.Issues {
		if issue.Code == "test.duplicate" {
			duplicates++
			if issue.Severity != "warning" {
				t.Fatal("duplicate blocked deliberate repeated data")
			}
		}
	}
	if !report.CanBuild || duplicates != 1 {
		t.Fatalf("missing duplicate hint: %+v", report)
	}
	f.put("secret-answer", "data/secret.ans", EntryAnswer, []byte("different answer"))
	report, _ = f.inspect(t)
	for _, issue := range report.Issues {
		if issue.Code == "test.duplicate" {
			t.Fatal("different answer considered duplicate")
		}
	}
}

func TestInspectionBlocksBrokenReferencesAndCycles(t *testing.T) {
	f := newInspectionFixture(t)
	f.meta.MainSolution = "missing"
	f.meta.TestOrder = []string{"sample", "absent"}
	f.document(t, "problem", "vertex/problem.json", EntryMetadata, f.meta)
	f.document(t, "g1", "vertex/groups/g1.json", EntryGroup, GroupMaterial{SchemaVersion: 1, Name: "G1", Aggregation: "sum", Prerequisites: []string{"g2"}})
	f.document(t, "g2", "vertex/groups/g2.json", EntryGroup, GroupMaterial{SchemaVersion: 1, Name: "G2", Aggregation: "sum", Prerequisites: []string{"g1"}})
	report, _ := f.inspect(t)
	if report.CanBuild {
		t.Fatal("invalid graph was buildable")
	}
	codes := map[string]bool{}
	for _, issue := range report.Issues {
		codes[issue.Code] = true
	}
	for _, code := range []string{"program.role", "test.unordered", "test.order_reference", "group.cycle"} {
		if !codes[code] {
			t.Errorf("missing %s in %+v", code, report.Issues)
		}
	}
}

func TestInspectionFingerprintIncludesExecutionSemantics(t *testing.T) {
	for _, change := range []string{"limits", "program", "comparison", "seed", "answer-source"} {
		t.Run(change, func(t *testing.T) {
			f := newInspectionFixture(t)
			before, _ := f.inspect(t)
			switch change {
			case "limits":
				f.meta.TimeLimitMs++
				f.document(t, "problem", "vertex/problem.json", EntryMetadata, f.meta)
			case "program":
				f.put("header", "programs/reference/helper.h", EntrySource, []byte("#define VALUE 42\n"))
			case "comparison":
				f.meta.Comparison.FloatingPoint = true
				f.document(t, "problem", "vertex/problem.json", EntryMetadata, f.meta)
			case "seed":
				f.document(t, "secret", "vertex/tests/secret.json", EntryTest, TestMaterial{SchemaVersion: 1, Name: "secret", Input: TestInput{Kind: "generator", Generator: "missing", Arguments: []string{"--seed", "42"}}, Answer: TestAnswer{Kind: "file", Entry: "secret-answer"}})
			case "answer-source":
				f.document(t, "secret", "vertex/tests/secret.json", EntryTest, TestMaterial{SchemaVersion: 1, Name: "secret", Input: TestInput{Kind: "file", Entry: "secret-input"}, Answer: TestAnswer{Kind: "solution"}})
			}
			after, _ := f.inspect(t)
			if before.DataHash == after.DataHash {
				t.Fatal("execution change did not invalidate fingerprint")
			}
		})
	}
}

func TestInspectionDoesNotHideStorageFailure(t *testing.T) {
	f := newInspectionFixture(t)
	expected := errors.New("storage unavailable")
	_, snapshot, err := InspectMaterials(f.tree, func(BlobRef) ([]byte, error) { return nil, expected })
	if !errors.Is(err, expected) || snapshot != nil {
		t.Fatalf("storage failure became a valid snapshot: %v", err)
	}
}
