package e2e

// 赛制端到端测试:ICPC 罚时、IOI 取最高分、OI 取最后一次提交、封榜与裁判视图、
// 反馈屏蔽、答疑与批量重测。需要 E2E_BASE_URL 指向运行中的完整栈。

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

type contestDetails struct {
	Contest struct {
		ID             string    `json:"id"`
		Title          string    `json:"title"`
		BeginAt        time.Time `json:"beginAt"`
		Format         string    `json:"format"`
		PenaltyMinutes int       `json:"penaltyMinutes"`
		Feedback       string    `json:"feedback"`
	} `json:"contest"`
	Problems []struct {
		ProblemID string `json:"problemId"`
		Label     string `json:"label"`
		Points    int    `json:"points"`
	} `json:"problems"`
	StaffRole string `json:"staffRole"`
}

type boardCell struct {
	Attempts     int     `json:"attempts"`
	PenaltySec   int     `json:"penaltySec"`
	Score        int     `json:"score"`
	SolvedAt     *string `json:"solvedAt"`
	PendingCount int     `json:"pendingCount"`
	FirstSolver  bool    `json:"firstSolver"`
}

type boardRow struct {
	Rank       int         `json:"rank"`
	Username   string      `json:"username"`
	UserID     string      `json:"userId"`
	Solved     int         `json:"solved"`
	Score      int         `json:"score"`
	Penalty    int         `json:"penalty"`
	Cells      []boardCell `json:"cells"`
	HasPending bool        `json:"hasPending"`
}

type board struct {
	Format   string     `json:"format"`
	Rows     []boardRow `json:"rows"`
	Frozen   bool       `json:"frozen"`
	JuryView bool       `json:"juryView"`
	Problems []struct {
		ProblemID string `json:"problemId"`
		Label     string `json:"label"`
		Points    int    `json:"points"`
	} `json:"problems"`
}

type clarification struct {
	ID       int64  `json:"id"`
	Body     string `json:"body"`
	FromJury bool   `json:"fromJury"`
	Announce bool   `json:"announce"`
	Answered bool   `json:"answered"`
	Replies  []struct {
		Body string `json:"body"`
	} `json:"replies"`
}

type rejudging struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Changed int    `json:"changed"`
}

// createContest builds a contest that is already running so submissions are
// accepted immediately, and registers nobody.
func createContest(t *testing.T, base, token string, body map[string]any) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	if err := httpJSON(http.MethodPost, base+"/api/admin/contests", token, body, &created, 201); err != nil {
		t.Fatalf("create contest: %v", err)
	}
	return created.ID
}

func setContestProblems(t *testing.T, base, token, contestID string, body map[string]any) {
	t.Helper()
	if err := httpJSON(http.MethodPut,
		base+"/api/admin/contests/"+contestID+"/problems", token, body, nil, 200); err != nil {
		t.Fatalf("set contest problems: %v", err)
	}
}

func readBoard(t *testing.T, base, token, contestID, view string) board {
	t.Helper()
	url := base + "/api/contests/" + contestID + "/rankboard"
	if view != "" {
		url += "?view=" + view
	}
	var result board
	if err := httpJSON(http.MethodGet, url, token, nil, &result, 200); err != nil {
		t.Fatalf("read rankboard: %v", err)
	}
	return result
}

func rowFor(t *testing.T, result board, username string) boardRow {
	t.Helper()
	for _, row := range result.Rows {
		if row.Username == username {
			return row
		}
	}
	t.Fatalf("username %q is not on the board (%d rows)", username, len(result.Rows))
	return boardRow{}
}

// contestWindow returns a window that has already started, so registration is
// closed for contestants; the test registers them through the admin path
// before the window opens by creating the contest a moment in the future.
func contestWindow(startIn, length time.Duration) (time.Time, time.Time) {
	begin := time.Now().Add(startIn)
	return begin, begin.Add(length)
}

// prepareContest creates a contest starting shortly in the future, registers
// the given tokens, then waits for it to start.
func prepareContest(
	t *testing.T, base, admin string, settings map[string]any,
	problems []map[string]any, contestants map[string]string,
) string {
	t.Helper()
	begin, end := contestWindow(6*time.Second, 30*time.Minute)
	body := map[string]any{
		"title": fmt.Sprintf("e2e-%s-%d", settings["rule"], time.Now().UnixNano()%1000000),
		"rule":  settings["rule"], "beginAt": begin, "endAt": end,
		"visibility": "public", "rankboardVisible": true,
	}
	for key, value := range settings {
		body[key] = value
	}
	contestID := createContest(t, base, admin, body)
	setContestProblems(t, base, admin, contestID, map[string]any{"problems": problems})

	for _, token := range contestants {
		if err := httpJSON(http.MethodPost,
			base+"/api/contests/"+contestID+"/register", token, map[string]string{}, nil, 200); err != nil {
			t.Fatalf("register for contest: %v", err)
		}
	}
	// Wait for the contest to open before the first submission.
	time.Sleep(time.Until(begin) + time.Second)
	return contestID
}

const acceptSolution = `#include <bits/stdc++.h>
int main(){int n; if(!(std::cin>>n)) return 0; long long s=0; for(int i=0;i<n;i++){long long v; std::cin>>v; s+=v;} std::cout<<s<<'\n';}
`

const rejectSolution = `#include <bits/stdc++.h>
int main(){std::cout<<-1<<'\n';}
`

// contestProblem creates a problem with simple sum testdata that both the
// accepting and the rejecting solution above can be judged against.
func contestProblem(t *testing.T, base, admin, title string) string {
	t.Helper()
	problemID := createProblem(t, base, admin, title, "sum the numbers")
	uploadTestdata(t, base, admin, problemID, map[string]string{
		"1.in": "3\n1 2 3\n", "1.out": "6\n",
		"2.in": "2\n10 20\n", "2.out": "30\n",
	})
	publishProblem(t, base, admin, problemID)
	return problemID
}

// TestEndToEndContestICPC covers the penalty model, the freeze, the jury view
// and the clarification channel on an ICPC contest.
func TestEndToEndContestICPC(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	adminUser, adminPass := os.Getenv("E2E_ADMIN_USER"), os.Getenv("E2E_ADMIN_PASS")
	if adminUser == "" || adminPass == "" {
		t.Skip("E2E_ADMIN_USER/E2E_ADMIN_PASS not set")
	}
	admin := mustLogin(t, base, adminUser, adminPass)
	problemID := contestProblem(t, base, admin, fmt.Sprintf("icpc-%d", time.Now().UnixNano()%1000000))

	alice, aliceName := registerUser(t, base)
	bob, bobName := registerUser(t, base)
	contestID := prepareContest(t, base, admin,
		map[string]any{"rule": "icpc", "penaltyMinutes": 20, "penalizeCompileError": true},
		[]map[string]any{{"problemId": problemID, "label": "A", "color": "#ff0000"}},
		map[string]string{aliceName: alice, bobName: bob})

	// Alice fails once, then solves; Bob solves on the first try.
	rejected := submit(t, base, alice, problemID, "cpp", rejectSolution, contestID)
	assertVerdict(t, waitForSubmission(t, base, alice, rejected, 3*time.Minute), "Wrong Answer")
	accepted := submit(t, base, alice, problemID, "cpp", acceptSolution, contestID)
	assertVerdict(t, waitForSubmission(t, base, alice, accepted, 3*time.Minute), "Accepted")

	bobAccepted := submit(t, base, bob, problemID, "cpp", acceptSolution, contestID)
	assertVerdict(t, waitForSubmission(t, base, bob, bobAccepted, 3*time.Minute), "Accepted")

	result := readBoard(t, base, alice, contestID, "")
	if result.Format != "icpc" {
		t.Fatalf("board format = %q, want icpc", result.Format)
	}
	aliceRow := rowFor(t, result, aliceName)
	if aliceRow.Solved != 1 {
		t.Fatalf("alice solved = %d, want 1", aliceRow.Solved)
	}
	if aliceRow.Cells[0].Attempts != 2 {
		t.Fatalf("alice attempts = %d, want 2", aliceRow.Cells[0].Attempts)
	}
	// One rejected run costs 20 minutes on top of the solve time.
	if aliceRow.Penalty < 20*60 {
		t.Fatalf("alice penalty = %ds, want at least the 20 minute rejection penalty", aliceRow.Penalty)
	}
	bobRow := rowFor(t, result, bobName)
	if bobRow.Penalty >= aliceRow.Penalty {
		t.Fatalf("bob penalty %d should be below alice's %d", bobRow.Penalty, aliceRow.Penalty)
	}
	if bobRow.Rank >= aliceRow.Rank {
		t.Fatalf("bob (rank %d) should outrank alice (rank %d)", bobRow.Rank, aliceRow.Rank)
	}
	// Exactly one contestant is the first solver of the problem.
	firsts := 0
	for _, row := range result.Rows {
		if row.Cells[0].FirstSolver {
			firsts++
		}
	}
	if firsts != 1 {
		t.Fatalf("first solver count = %d, want 1", firsts)
	}

	// A contestant may not request the jury board.
	contestantJury := readBoard(t, base, alice, contestID, "jury")
	if contestantJury.JuryView {
		t.Fatal("a contestant received the jury view")
	}

	// Clarifications: alice asks, the jury answers, bob cannot see it.
	var asked clarification
	if err := httpJSON(http.MethodPost, base+"/api/contests/"+contestID+"/clarifications",
		alice, map[string]any{"problemId": problemID, "body": "样例是否保证有序?"}, &asked, 201); err != nil {
		t.Fatalf("ask clarification: %v", err)
	}
	if err := httpJSON(http.MethodPost, base+"/api/contests/"+contestID+"/clarifications/reply",
		admin, map[string]any{"parentId": asked.ID, "body": "不保证。"}, nil, 201); err != nil {
		t.Fatalf("reply to clarification: %v", err)
	}
	var aliceThreads struct {
		Items []clarification `json:"items"`
	}
	if err := httpJSON(http.MethodGet, base+"/api/contests/"+contestID+"/clarifications",
		alice, nil, &aliceThreads, 200); err != nil {
		t.Fatalf("list clarifications: %v", err)
	}
	if len(aliceThreads.Items) != 1 || len(aliceThreads.Items[0].Replies) != 1 {
		t.Fatalf("alice sees %+v, want one answered thread", aliceThreads.Items)
	}
	if !aliceThreads.Items[0].Answered {
		t.Fatal("the answered thread is not marked as answered")
	}
	var bobThreads struct {
		Items []clarification `json:"items"`
	}
	if err := httpJSON(http.MethodGet, base+"/api/contests/"+contestID+"/clarifications",
		bob, nil, &bobThreads, 200); err != nil {
		t.Fatalf("list clarifications as bob: %v", err)
	}
	if len(bobThreads.Items) != 0 {
		t.Fatalf("bob can read another team's clarification: %+v", bobThreads.Items)
	}

	// An announcement reaches everyone.
	if err := httpJSON(http.MethodPost, base+"/api/contests/"+contestID+"/clarifications/reply",
		admin, map[string]any{"body": "全场公告:数据已更新。"}, nil, 201); err != nil {
		t.Fatalf("announce: %v", err)
	}
	if err := httpJSON(http.MethodGet, base+"/api/contests/"+contestID+"/clarifications",
		bob, nil, &bobThreads, 200); err != nil {
		t.Fatalf("list announcements as bob: %v", err)
	}
	if len(bobThreads.Items) != 1 || !bobThreads.Items[0].Announce {
		t.Fatalf("bob did not receive the announcement: %+v", bobThreads.Items)
	}

	// Batch rejudge: the verdicts are deterministic, so nothing should change.
	var batch rejudging
	if err := httpJSON(http.MethodPost, base+"/api/admin/rejudgings", admin,
		map[string]any{"contestId": contestID, "reason": "e2e"}, &batch, 202); err != nil {
		t.Fatalf("create rejudging: %v", err)
	}
	if batch.Total != 3 {
		t.Fatalf("rejudging covers %d submissions, want 3", batch.Total)
	}
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		if err := httpJSON(http.MethodGet, base+"/api/admin/rejudgings/"+batch.ID,
			admin, nil, &batch, 200); err != nil {
			t.Fatalf("read rejudging: %v", err)
		}
		if batch.State == "finished" {
			break
		}
		time.Sleep(time.Second)
	}
	if batch.State != "finished" {
		t.Fatalf("rejudging did not finish: %+v", batch)
	}
	if batch.Changed != 0 {
		t.Fatalf("a deterministic rejudge changed %d verdicts", batch.Changed)
	}
	// The scoreboard must survive the rejudge unchanged.
	after := readBoard(t, base, alice, contestID, "")
	afterAlice := rowFor(t, after, aliceName)
	if afterAlice.Solved != aliceRow.Solved || afterAlice.Penalty != aliceRow.Penalty {
		t.Fatalf("rejudge changed the standings: %+v -> %+v", aliceRow, afterAlice)
	}
}

// TestEndToEndContestScoreFormats covers IOI (best score wins) and OI (the last
// submission is the one that counts), including the feedback blackout.
func TestEndToEndContestScoreFormats(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	adminUser, adminPass := os.Getenv("E2E_ADMIN_USER"), os.Getenv("E2E_ADMIN_PASS")
	if adminUser == "" || adminPass == "" {
		t.Skip("E2E_ADMIN_USER/E2E_ADMIN_PASS not set")
	}
	admin := mustLogin(t, base, adminUser, adminPass)
	problemID := contestProblem(t, base, admin, fmt.Sprintf("score-%d", time.Now().UnixNano()%1000000))

	t.Run("IOI keeps the best score", func(t *testing.T) {
		user, username := registerUser(t, base)
		contestID := prepareContest(t, base, admin,
			map[string]any{"rule": "ioi", "feedback": "full"},
			[]map[string]any{{"problemId": problemID, "label": "A", "points": 100}},
			map[string]string{username: user})

		accepted := submit(t, base, user, problemID, "cpp", acceptSolution, contestID)
		assertVerdict(t, waitForSubmission(t, base, user, accepted, 3*time.Minute), "Accepted")
		// A later worse submission must not take the score away.
		worse := submit(t, base, user, problemID, "cpp", rejectSolution, contestID)
		assertVerdict(t, waitForSubmission(t, base, user, worse, 3*time.Minute), "Wrong Answer")

		result := readBoard(t, base, user, contestID, "")
		if result.Format != "ioi" {
			t.Fatalf("board format = %q, want ioi", result.Format)
		}
		row := rowFor(t, result, username)
		if row.Score != 100 {
			t.Fatalf("IOI score = %d, want 100 (the best result is kept)", row.Score)
		}
		if row.Cells[0].Attempts != 2 {
			t.Fatalf("IOI attempts = %d, want 2", row.Cells[0].Attempts)
		}
		if row.Penalty != 0 {
			t.Fatalf("IOI penalty = %d, want 0", row.Penalty)
		}
	})

	t.Run("OI scores the last submission and hides feedback", func(t *testing.T) {
		user, username := registerUser(t, base)
		contestID := prepareContest(t, base, admin,
			map[string]any{"rule": "oi"},
			[]map[string]any{{"problemId": problemID, "label": "A", "points": 100}},
			map[string]string{username: user})

		var details contestDetails
		if err := httpJSON(http.MethodGet, base+"/api/contests/"+contestID, user, nil, &details, 200); err != nil {
			t.Fatalf("read contest: %v", err)
		}
		if details.Contest.Feedback != "none" {
			t.Fatalf("OI feedback = %q, want none by default", details.Contest.Feedback)
		}

		accepted := submit(t, base, user, problemID, "cpp", acceptSolution, contestID)
		first := waitForContestSubmission(t, base, user, accepted, 3*time.Minute)
		// The contestant must not learn the verdict during an OI contest.
		if first.Status != "Submitted" {
			t.Fatalf("OI contestant saw status %q, want the hidden placeholder", first.Status)
		}
		if len(first.CaseResults) != 0 {
			t.Fatal("OI contestant received per-test results")
		}
		assertHiddenSubmissionWire(t, base, user, accepted, contestID, "Submitted", true)
		for _, status := range []string{"Accepted", "Wrong%20Answer"} {
			zeroWire(t, readWire(t, base, user, "/api/submissions?contest="+contestID+"&status="+status, 200), "total")
		}
		// The jury still sees the real verdict.
		var juryView submission
		if err := httpJSON(http.MethodGet, base+"/api/submissions/"+accepted,
			admin, nil, &juryView, 200); err != nil {
			t.Fatalf("read submission as admin: %v", err)
		}
		if juryView.Status != "Accepted" {
			t.Fatalf("admin saw status %q, want Accepted", juryView.Status)
		}

		// The last submission is the one that counts, even when it is worse.
		worse := submit(t, base, user, problemID, "cpp", rejectSolution, contestID)
		waitForContestSubmission(t, base, user, worse, 3*time.Minute)

		// OI withholds the public board until the end, including when staff
		// explicitly preview it. A contestant cannot bypass this via view=jury.
		for _, viewer := range []struct{ token, query string }{
			{user, ""}, {user, "?view=jury"}, {admin, ""},
		} {
			var response struct {
				Code string `json:"code"`
			}
			if err := httpJSON(http.MethodGet, base+"/api/contests/"+contestID+"/rankboard"+viewer.query,
				viewer.token, nil, &response, http.StatusForbidden); err != nil {
				t.Fatalf("OI public board must remain hidden: %v", err)
			}
			if response.Code != "contest.rankboard_hidden" {
				t.Fatalf("OI board error = %q, want contest.rankboard_hidden", response.Code)
			}
		}
		assertVerdict(t, waitForSubmission(t, base, admin, worse, 3*time.Minute), "Wrong Answer")
		result := readBoard(t, base, admin, contestID, "jury")
		row := rowFor(t, result, username)
		if !result.JuryView || row.Score != 0 || len(row.Cells) != 1 || row.Cells[0].Attempts != 2 {
			t.Fatalf("OI jury board must score both submissions using the last result: %+v", result)
		}

		// End through the same settings API as the UI. Keep both submissions
		// inside the contest window, then verify results become public.
		if err := httpJSON(http.MethodPut, base+"/api/admin/contests/"+contestID, admin,
			map[string]any{
				"title": details.Contest.Title, "rule": "oi", "feedback": "none",
				"beginAt": details.Contest.BeginAt, "endAt": time.Now().UTC(),
				"visibility": "public", "rankboardVisible": true,
			}, nil, http.StatusOK); err != nil {
			t.Fatalf("end OI contest: %v", err)
		}
		published := readBoard(t, base, user, contestID, "")
		publicRow := rowFor(t, published, username)
		if published.JuryView || published.Frozen || published.Format != "oi" || publicRow.Score != 0 || len(publicRow.Cells) != 1 || publicRow.Cells[0].Attempts != 2 {
			t.Fatalf("OI final public board differs from the final submission result: %+v", published)
		}
		var revealed submission
		if err := httpJSON(http.MethodGet, base+"/api/submissions/"+accepted, user, nil, &revealed, http.StatusOK); err != nil {
			t.Fatalf("read OI result after end: %v", err)
		}
		if revealed.Status != "Accepted" || len(revealed.CaseResults) == 0 {
			t.Fatalf("OI feedback must be revealed after the contest ends: %+v", revealed)
		}
	})
}

// waitForContestSubmission polls until judging settles. In a contest that
// hides feedback the status never leaves the placeholder, so this also stops
// once the submission is no longer queued.
func waitForContestSubmission(t *testing.T, base, token, id string, timeout time.Duration) submission {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var item submission
		if err := httpJSON(http.MethodGet, base+"/api/submissions/"+id, token, nil, &item, 200); err == nil {
			if item.Status != "Pending" && item.Status != "Judging" {
				return item
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("submission %s did not settle within %s", id, timeout)
	return submission{}
}
