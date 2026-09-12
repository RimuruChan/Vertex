package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

type wireObject map[string]json.RawMessage

func readWire(t *testing.T, base, token, path string, status int) wireObject {
	t.Helper()
	var body wireObject
	if err := httpJSON(http.MethodGet, base+path, token, nil, &body, status); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return body
}

func omittedWire(t *testing.T, body wireObject, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, exists := body[field]; exists {
			t.Errorf("response contains forbidden field %q", field)
		}
	}
}

func zeroWire(t *testing.T, body wireObject, fields ...string) {
	t.Helper()
	for _, field := range fields {
		var number int
		if raw, exists := body[field]; !exists {
			t.Errorf("missing sanitized field %s", field)
		} else if err := json.Unmarshal(raw, &number); err != nil || number != 0 {
			t.Errorf("%s must be numeric zero, got %s", field, raw)
		}
	}
}

func stringWire(t *testing.T, body wireObject, field string) string {
	t.Helper()
	var value string
	if err := json.Unmarshal(body[field], &value); err != nil {
		t.Fatalf("decode %s: %v", field, err)
	}
	return value
}

func listWireItem(t *testing.T, body wireObject, id string) wireObject {
	t.Helper()
	var items []wireObject
	if err := json.Unmarshal(body["items"], &items); err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if stringWire(t, item, "id") == id {
			return item
		}
	}
	t.Fatalf("submission %s not present in list", id)
	return nil
}

func pollingWire(t *testing.T, body wireObject) {
	t.Helper()
	allowed := map[string]bool{"id": true, "status": true, "score": true, "totalTimeMs": true, "peakMemoryKb": true, "compileResult": true, "caseResults": true, "judgedCases": true, "totalCases": true}
	for field := range body {
		if !allowed[field] {
			t.Errorf("polling response contains unexpected field %q", field)
		}
	}
}

func redactedWire(t *testing.T, body wireObject, status string, hideScore bool) {
	t.Helper()
	if got := stringWire(t, body, "status"); got != status {
		t.Errorf("status = %s, want %s", got, status)
	}
	omittedWire(t, body, "caseResults", "compileResult", "judgedAt")
	zeroWire(t, body, "totalTimeMs", "peakMemoryKb", "judgedCases", "totalCases")
	if hideScore {
		zeroWire(t, body, "score")
	}
}

func assertHiddenSubmissionWire(t *testing.T, base, token, id, contestID, status string, hideScore bool) {
	t.Helper()
	detail := readWire(t, base, token, "/api/submissions/"+id, 200)
	redactedWire(t, detail, status, hideScore)
	progress := readWire(t, base, token, "/api/submissions/"+id+"/progress", 200)
	pollingWire(t, progress)
	redactedWire(t, progress, status, hideScore)
	listed := listWireItem(t, readWire(t, base, token, "/api/submissions?contest="+contestID, 200), id)
	omittedWire(t, listed, "sourceCode")
	redactedWire(t, listed, status, hideScore)
}

// This tests bytes crossing HTTP, not whether the UI happens to render them.
// The owner/jury positive controls prove the protected data really exists.
func TestEndToEndContestResponsePrivacy(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" || os.Getenv("E2E_ADMIN_USER") == "" || os.Getenv("E2E_ADMIN_PASS") == "" {
		t.Skip("E2E environment not configured")
	}
	admin := mustLogin(t, base, os.Getenv("E2E_ADMIN_USER"), os.Getenv("E2E_ADMIN_PASS"))
	problemID := contestProblem(t, base, admin, fmt.Sprintf("privacy-%d", time.Now().UnixNano()%1000000))
	alice, aliceName := registerUser(t, base)
	bob, bobName := registerUser(t, base)
	contestID := prepareContest(t, base, admin, map[string]any{"rule": "icpc", "feedback": "full", "submissionVisibility": "during", "sourceCodeVisibility": "after_end"}, []map[string]any{{"problemId": problemID, "label": "A"}}, map[string]string{aliceName: alice, bobName: bob})
	accepted := submit(t, base, alice, problemID, "cpp", acceptSolution, contestID)
	assertVerdict(t, waitForSubmission(t, base, admin, accepted, 3*time.Minute), "Accepted")
	wrong := submit(t, base, alice, problemID, "cpp", rejectSolution, contestID)
	assertVerdict(t, waitForSubmission(t, base, admin, wrong, 3*time.Minute), "Wrong Answer")
	const privateSource = "#error PRIVATE_SOURCE_SENTINEL\nint main(){}\n"
	compileError := submit(t, base, alice, problemID, "cpp", privateSource, contestID)
	assertVerdict(t, waitForSubmission(t, base, admin, compileError, 3*time.Minute), "Compile Error")
	own := readWire(t, base, alice, "/api/submissions/"+compileError, 200)
	if stringWire(t, own, "sourceCode") != privateSource || !strings.Contains(stringWire(t, own, "compileResult"), "PRIVATE_SOURCE_SENTINEL") {
		t.Fatal("compiler fixture must expose the owner's source marker to its owner")
	}
	for _, suffix := range []string{"", "/progress"} {
		peer := readWire(t, base, bob, "/api/submissions/"+compileError+suffix, 200)
		omittedWire(t, peer, "sourceCode", "compileResult")
		encoded, _ := json.Marshal(peer)
		if strings.Contains(string(encoded), "PRIVATE_SOURCE_SENTINEL") {
			t.Fatal("peer received private code through compiler diagnostics")
		}
		if suffix != "" {
			pollingWire(t, peer)
		}
	}
	full := readWire(t, base, alice, "/api/submissions/"+accepted, 200)
	var cases []wireObject
	if err := json.Unmarshal(full["caseResults"], &cases); err != nil || len(cases) == 0 {
		t.Fatal("full feedback fixture has no actual cases")
	}
	var details contestDetails
	if err := httpJSON(http.MethodGet, base+"/api/contests/"+contestID, admin, nil, &details, 200); err != nil {
		t.Fatal(err)
	}
	settings := map[string]any{"title": details.Contest.Title, "rule": "icpc", "beginAt": details.Contest.BeginAt, "endAt": time.Now().Add(20 * time.Minute), "visibility": "public", "rankboardVisible": true, "submissionVisibility": "during", "sourceCodeVisibility": "after_end"}
	save := func() {
		t.Helper()
		if err := httpJSON(http.MethodPut, base+"/api/admin/contests/"+contestID, admin, settings, nil, 200); err != nil {
			t.Fatalf("change visibility policy: %v", err)
		}
	}
	settings["feedback"] = "summary"
	save()
	assertHiddenSubmissionWire(t, base, alice, accepted, contestID, "Accepted", false)
	settings["feedback"] = "first_error"
	save()
	for _, suffix := range []string{"", "/progress"} {
		body := readWire(t, base, alice, "/api/submissions/"+wrong+suffix, 200)
		omittedWire(t, body, "compileResult", "judgedAt")
		zeroWire(t, body, "totalTimeMs", "peakMemoryKb", "judgedCases", "totalCases")
		var first []wireObject
		if err := json.Unmarshal(body["caseResults"], &first); err != nil || len(first) != 1 {
			t.Fatalf("first-error feedback must contain exactly one failed case: %s", body["caseResults"])
		}
		if stringWire(t, first[0], "verdict") != "Wrong Answer" || string(first[0]["caseIndex"]) != "1" {
			t.Fatalf("wrong first failure: %v", first[0])
		}
		omittedWire(t, first[0], "checkerOutput", "exitStatus")
		zeroWire(t, first[0], "timeMs", "memoryKb")
		if suffix != "" {
			pollingWire(t, body)
		}
	}
	settings["feedback"] = "full"
	settings["freezeAt"] = details.Contest.BeginAt.Add(time.Millisecond)
	settings["frozenSubmissionVisibility"] = "pending"
	save()
	assertHiddenSubmissionWire(t, base, bob, accepted, contestID, "Pending", true)
	omittedWire(t, readWire(t, base, bob, "/api/submissions/"+accepted, 200), "sourceCode")
	for _, status := range []string{"Accepted", "Wrong Answer", "Compile Error"} {
		body := readWire(t, base, bob, "/api/submissions?contest="+contestID+"&status="+url.QueryEscape(status), 200)
		zeroWire(t, body, "total")
		if string(body["items"]) != "[]" {
			t.Fatal("frozen status filter exposed records")
		}
	}
	public := readBoard(t, base, bob, contestID, "")
	hidden := rowFor(t, public, aliceName)
	if hidden.Solved != 0 || hidden.Score != 0 || hidden.Penalty != 0 || len(hidden.Cells) != 1 || hidden.Cells[0].SolvedAt != nil || hidden.Cells[0].FirstSolver || hidden.Cells[0].Score != 0 {
		t.Fatalf("frozen board exposes private results: %+v", hidden)
	}
	settings["frozenSubmissionVisibility"] = "hidden"
	save()
	for _, suffix := range []string{"", "/progress"} {
		body := readWire(t, base, bob, "/api/submissions/"+accepted+suffix, 404)
		omittedWire(t, body, "sourceCode", "status", "score", "caseResults", "judgedAt")
	}
	zeroWire(t, readWire(t, base, bob, "/api/submissions?contest="+contestID, 200), "total")
	settings["frozenSubmissionVisibility"] = "pending"
	settings["endAt"] = time.Now().UTC()
	save()
	assertHiddenSubmissionWire(t, base, bob, accepted, contestID, "Pending", true)
	settings["unfreezeAt"] = time.Now().UTC()
	save()
	peer := readWire(t, base, bob, "/api/submissions/"+accepted, 200)
	if stringWire(t, peer, "sourceCode") != acceptSolution || stringWire(t, peer, "status") != "Accepted" {
		t.Fatal("source/results were not released after end and unfreeze")
	}
	pollingWire(t, readWire(t, base, bob, "/api/submissions/"+accepted+"/progress", 200))
	if !strings.Contains(stringWire(t, readWire(t, base, bob, "/api/submissions/"+compileError+"/progress", 200), "compileResult"), "PRIVATE_SOURCE_SENTINEL") {
		t.Fatal("released source diagnostics should be readable after end and unfreeze")
	}
}
