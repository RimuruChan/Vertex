package packages

import (
	"bytes"
	"io"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func validationFixture() map[string]string {
	files := kattisFixture("2025-09")
	files["data/invalid_input/extra.in"] = "1 2 3\n"
	for _, mode := range []string{"valid_output", "invalid_output"} {
		base := "data/" + mode + "/candidate"
		files[base+".in"] = "1 2\n"
		files[base+".ans"] = "3\n"
		files[base+".out"] = "3\n"
		files[base+".yaml"] = "description: Unicode 中文 output test\n"
	}
	files["data/invalid_output/candidate.out"] = "4\n"
	return files
}

func TestValidationPackageRoundTripPreservesExpectationAndBytes(t *testing.T) {
	store, blobs := testStore(t)
	first, err := Import(packageZIP(t, validationFixture()), domain.ImportOptions{}, store)
	if err != nil {
		t.Fatal(err)
	}
	read := func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	}
	exported, err := Export(first.Tree, ExportOptions{Format: "kattis-2025-09"}, read)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Import(exported.Data, domain.ImportOptions{}, store)
	if err != nil {
		t.Fatal(err)
	}
	for _, plan := range []*domain.ImportPlan{first, second} {
		report, snapshot, err := domain.InspectMaterials(plan.Tree, func(ref domain.BlobRef) ([]byte, error) { return blobs[ref.SHA256], nil })
		if err != nil || !report.CanBuild || report.ValidationCount != 3 {
			t.Fatalf("self-tests not executable: %+v %v", report, err)
		}
		for _, item := range snapshot.Validation {
			if item.Definition.Mode == "invalid_input" {
				if item.Answer != nil || item.Output != nil {
					t.Fatal("invalid input gained an answer")
				}
				continue
			}
			expected := "3\n"
			if item.Definition.Mode == "invalid_output" {
				expected = "4\n"
			}
			if string(blobs[item.Output.SHA256]) != expected || item.Definition.Description != "Unicode 中文 output test" {
				t.Fatalf("lost self-test: %+v", item)
			}
		}
	}
	if _, err := Export(first.Tree, ExportOptions{Format: "kattis-legacy"}, read); err == nil {
		t.Fatal("legacy silently dropped validation tests")
	}
}

func TestValidationPackageRejectsMissingFilesAndBlocksUnknownArguments(t *testing.T) {
	for _, file := range []string{"data/invalid_output/candidate.ans", "data/valid_output/candidate.out", "data/invalid_output/candidate.in"} {
		files := validationFixture()
		delete(files, file)
		store, _ := testStore(t)
		if _, err := Import(packageZIP(t, files), domain.ImportOptions{}, store); err == nil {
			t.Fatalf("missing %s accepted", file)
		}
	}
	files := validationFixture()
	files["data/invalid_input/test_group.yaml"] = "input_validator_args: [strict]\n"
	store, blobs := testStore(t)
	plan, err := Import(packageZIP(t, files), domain.ImportOptions{}, store)
	if err != nil {
		t.Fatal(err)
	}
	report, _, err := domain.InspectMaterials(plan.Tree, func(ref domain.BlobRef) ([]byte, error) { return blobs[ref.SHA256], nil })
	if err != nil || report.CanBuild {
		t.Fatalf("unknown arguments silently ignored: %+v %v", report, err)
	}
}
