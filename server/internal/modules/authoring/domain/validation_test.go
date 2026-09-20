package domain

import "testing"

func TestValidationSnapshotBindsFilesAndRejectsMissingReferences(t *testing.T) {
	f := newInspectionFixture(t)
	item := ValidationMaterial{SchemaVersion: 1, Name: "Reject wrong sum", Mode: "invalid_output", Input: "sample-input", Answer: "sample-answer", Output: "candidate"}
	f.put("candidate", "validation/wrong.out", EntryAnswer, []byte("4\n"))
	f.document(t, "negative", "vertex/validation/negative.json", EntryValidation, item)
	report, before := f.inspect(t)
	if !report.CanBuild || report.ValidationCount != 1 || len(before.Validation) != 1 || before.Validation[0].Output == nil {
		t.Fatalf("self-test omitted: %+v", report)
	}
	f.put("candidate", "validation/wrong.out", EntryAnswer, []byte("3\n"))
	_, after := f.inspect(t)
	if before.DataHash == after.DataHash {
		t.Fatal("changed candidate reused successful check")
	}
	item.Output = "missing"
	f.document(t, "negative", "vertex/validation/negative.json", EntryValidation, item)
	report, _ = f.inspect(t)
	if report.CanBuild {
		t.Fatal("missing candidate accepted")
	}
	item.Mode = "invalid_input"
	item.Output = ""
	item.Answer = ""
	f.document(t, "negative", "vertex/validation/negative.json", EntryValidation, item)
	report, _ = f.inspect(t)
	if report.CanBuild {
		t.Fatal("negative input without input validator accepted")
	}
}

func TestValidationMaterialRejectsContradictoryExpectation(t *testing.T) {
	for _, item := range []ValidationMaterial{
		{SchemaVersion: 1, Name: "bad", Mode: "invalid_input", Answer: "answer"},
		{SchemaVersion: 1, Name: "bad", Mode: "sometimes"},
		{SchemaVersion: 1, Name: "bad", Mode: "valid_output", Input: "../secret"},
	} {
		if item.Validate() == nil {
			t.Fatalf("invalid self-test accepted: %+v", item)
		}
	}
}
