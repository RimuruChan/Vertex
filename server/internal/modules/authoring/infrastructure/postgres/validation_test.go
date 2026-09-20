package postgres

import (
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"testing"
)

func TestValidationResultsRequireEveryExpectedDecision(t *testing.T) {
	cases := []domain.SnapshotValidation{{ID: "negative", Definition: domain.ValidationMaterial{Mode: "invalid_input"}}, {ID: "positive", Definition: domain.ValidationMaterial{Mode: "valid_output"}}}
	good := []domain.ValidationOutcome{{ID: "negative", Mode: "invalid_input", Actual: "rejected", Status: "ok"}, {ID: "positive", Mode: "valid_output", Actual: "accepted", Status: "ok"}}
	if !validationResultsMatch(cases, good) {
		t.Fatal("complete results rejected")
	}
	if validationResultsMatch(cases, good[:1]) {
		t.Fatal("missing result accepted")
	}
	for _, field := range []string{"id", "mode", "actual", "status"} {
		bad := append([]domain.ValidationOutcome{}, good...)
		switch field {
		case "id":
			bad[1].ID = "negative"
		case "mode":
			bad[1].Mode = "invalid_output"
		case "actual":
			bad[1].Actual = "rejected"
		case "status":
			bad[1].Status = "failed"
		}
		if validationResultsMatch(cases, bad) {
			t.Fatalf("changed %s accepted", field)
		}
	}
}
