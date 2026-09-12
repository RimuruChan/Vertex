package dto

import (
	"encoding/json"
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	"strings"
	"testing"
	"time"
)

func TestFrozenRankboardWireProjection(t *testing.T) {
	solved := time.Date(2030, 1, 1, 12, 34, 56, 0, time.UTC)
	board := &contestdomain.Rankboard{Format: "icpc", Frozen: true, ProblemCount: 1, ProblemIDs: []string{"p"}, Problems: []contestdomain.Problem{{ProblemID: "p", Label: "A"}}, FirstSolvers: map[string]string{"p": "u"}, Rows: []contestdomain.RankRow{{Rank: 1, UserID: "u", Username: "team", HasPending: true, Cells: []contestdomain.Cell{{Attempts: 8, PenaltySec: 9876, Score: 100, SolvedAt: &solved, PublicAttempts: 1, PendingCount: 3}}}}}
	encode := func() map[string]any {
		t.Helper()
		raw, err := json.Marshal(FromRankboard(board))
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"fullResults", "firstSolvers"} {
			if _, exists := value[field]; exists {
				t.Fatalf("internal board field serialized: %s", field)
			}
		}
		if !board.FullResults && strings.Contains(string(raw), solved.Format(time.RFC3339)) {
			t.Fatal("private solve time leaked")
		}
		return value
	}
	value := encode()
	row := value["rows"].([]any)[0].(map[string]any)
	cell := row["cells"].([]any)[0].(map[string]any)
	allowed := map[string]bool{"attempts": true, "penaltySec": true, "score": true, "pendingCount": true, "firstSolver": true}
	for field := range cell {
		if !allowed[field] {
			t.Errorf("private/unknown frozen cell field: %s", field)
		}
	}
	if cell["attempts"] != float64(1) || cell["pendingCount"] != float64(3) || cell["score"] != float64(0) || cell["penaltySec"] != float64(0) || cell["firstSolver"] != false {
		t.Fatalf("bad public projection: %v", cell)
	}
	board.FullResults = true
	board.Frozen = false
	value = encode()
	cell = value["rows"].([]any)[0].(map[string]any)["cells"].([]any)[0].(map[string]any)
	if cell["score"] != float64(100) || cell["attempts"] != float64(8) || cell["penaltySec"] != float64(9876) || cell["pendingCount"] != float64(0) || cell["firstSolver"] != true || cell["solvedAt"] != solved.Format(time.RFC3339) {
		t.Fatalf("full projection positive control missing private fixture values: %v", cell)
	}
}
