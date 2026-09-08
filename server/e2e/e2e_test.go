package e2e

// 端到端测试:驱动完整判题管线(注册 → 建题 → 传测试数据 → 提交 → 判定)。
// 需设置 E2E_BASE_URL(指向运行中的 web API);未设置时跳过(本地 go test ./... 不受影响)。
// 配套 GitHub Actions 工作流: .github/workflows/e2e.yml

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sort"
	"sync"
	"testing"
	"time"
)

// ---- API 响应结构(与公开 DTO 对齐)----

type authResp struct {
	AccessToken string `json:"accessToken"`
	Token       string `json:"token"`
	User        struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	} `json:"user"`
}

type judgeJob struct {
	JobID        string `json:"jobId"`
	SubmissionID string `json:"submissionId"`
	Generation   int    `json:"generation"`
	LeaseToken   string `json:"leaseToken"`
}

type problemResp struct {
	ID              string `json:"id"`
	Visibility      string `json:"visibility"`
	SubmissionCount int    `json:"submissionCount"`
	AcceptedCount   int    `json:"acceptedCount"`
	SolvedUserCount int    `json:"solvedUserCount"`
}

type caseResult struct {
	CaseIndex  int    `json:"caseIndex"`
	Verdict    string `json:"verdict"`
	TimeMs     int    `json:"timeMs"`
	MemoryKb   int    `json:"memoryKb"`
	ExitStatus string `json:"exitStatus"`
}

type submission struct {
	ID             string       `json:"id"`
	ProblemID      string       `json:"problemId"`
	ProblemVersion int          `json:"problemVersion"`
	Language       string       `json:"language"`
	Status         string       `json:"status"`
	Score          int          `json:"score"`
	TotalTimeMs    int          `json:"totalTimeMs"`
	PeakMemoryKb   int          `json:"peakMemoryKb"`
	CompileResult  string       `json:"compileResult"`
	CaseResults    []caseResult `json:"caseResults"`
}

type contest struct {
	ID      string `json:"id"`
	BeginAt string `json:"beginAt"`
	EndAt   string `json:"endAt"`
}

type acmCell struct {
	Attempts     int     `json:"attempts"`
	PenaltySec   int     `json:"penaltySec"`
	SolvedAt     *string `json:"solvedAt"`
	PendingCount int     `json:"pendingCount"`
}

type rankRow struct {
	Rank     int       `json:"rank"`
	Username string    `json:"username"`
	UserID   string    `json:"userId"`
	Solved   int       `json:"solved"`
	Penalty  int       `json:"penalty"`
	Cells    []acmCell `json:"cells"`
}

type rankboard struct {
	ProblemCount int       `json:"problemCount"`
	ProblemIDs   []string  `json:"problemIds"`
	Rows         []rankRow `json:"rows"`
	Frozen       bool      `json:"frozen"`
}

// ---- HTTP 工具 ----

func httpJSON(method, url, token string, body, out any, wantStatus int) error {
	return httpJSONWithClient(http.DefaultClient, method, url, token, body, out, wantStatus)
}

func httpJSONWithClient(client *http.Client, method, url, token string, body, out any, wantStatus int) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("status %d (want %d): %s", resp.StatusCode, wantStatus, string(b))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func mustLogin(t *testing.T, base, user, pass string) string {
	t.Helper()
	var out authResp
	if err := httpJSON(http.MethodPost, base+"/api/auth/login", "",
		map[string]string{"username": user, "password": pass}, &out, 200); err != nil {
		t.Fatalf("login: %v", err)
	}
	return out.AccessToken
}

func registerUser(t *testing.T, base string) (token, username string) {
	t.Helper()
	username = fmt.Sprintf("user_%d", time.Now().UnixNano()%1000000000)
	var out authResp
	if err := httpJSON(http.MethodPost, base+"/api/auth/register", "",
		map[string]string{"username": username, "email": username + "@test.local", "password": "testpass123"},
		&out, 201); err != nil {
		t.Fatalf("register: %v", err)
	}
	return out.AccessToken, username
}

func createProblem(t *testing.T, base, token, title, statement string) string {
	t.Helper()
	body := map[string]any{
		"title": title, "statementMd": statement, "difficulty": 1,
		"source": "e2e", "timeLimitMs": 1000, "memoryLimitKb": 262144, "visibility": "public",
	}
	var p problemResp
	if err := httpJSON(http.MethodPost, base+"/api/admin/problems", token, body, &p, 201); err != nil {
		t.Fatalf("create problem: %v", err)
	}
	return p.ID
}

// uploadTestdata 构造内存 zip 并以 multipart 上传。
func uploadTestdata(t *testing.T, base, token, problemID string, files map[string]string) {
	uploadTestdataAt(t, base+"/api/admin/problems/"+problemID+"/testdata", token, files)
}

func uploadTestdataAt(t *testing.T, endpoint, token string, files map[string]string) {
	t.Helper()

	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "testdata.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(zbuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	_ = mw.WriteField("checker", "diff")
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		t.Fatalf("upload testdata: status %d: %s", resp.StatusCode, string(b))
	}
}

func submit(t *testing.T, base, token, problemID, lang, code string, contestID ...string) string {
	t.Helper()
	body := map[string]string{"problemId": problemID, "language": lang, "sourceCode": code}
	if len(contestID) > 0 {
		body["contestId"] = contestID[0]
	}
	var s submission
	if err := httpJSON(http.MethodPost, base+"/api/submissions", token, body, &s, 202); err != nil {
		t.Fatalf("submit: %v", err)
	}
	return s.ID
}

func isTerminal(status string) bool {
	return status != "Pending" && status != "Judging"
}

// waitForSubmission 以 500ms 间隔轮询公开提交状态，并为实际判题留出充足超时。
func waitForSubmission(t *testing.T, base, token, id string, timeout time.Duration) submission {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var s submission
		if err := httpJSON(http.MethodGet, base+"/api/submissions/"+id, token, nil, &s, 200); err == nil && isTerminal(s.Status) {
			return s
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("submission %s did not finish within %s", id, timeout)
	return submission{}
}

func assertVerdict(t *testing.T, s submission, want string) {
	t.Helper()
	if s.Status != want {
		t.Fatalf("verdict = %q, want %q (compileResult=%q, caseResults=%+v)",
			s.Status, want, s.CompileResult, s.CaseResults)
	}
}

func TestEndToEndAuthLifecycle(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	username := fmt.Sprintf("auth_%d", time.Now().UnixNano()%1000000000)
	registration := map[string]string{
		"username": username, "email": username + "@test.local", "password": "testpass123",
	}
	var registered authResp
	if err := httpJSONWithClient(client, http.MethodPost, base+"/api/auth/register", "", registration, &registered, http.StatusCreated); err != nil {
		t.Fatalf("register: %v", err)
	}
	if registered.AccessToken == "" || registered.AccessToken != registered.Token {
		t.Fatalf("access token compatibility fields are inconsistent")
	}
	cookies := jar.Cookies(mustURL(t, base+"/api/auth/refresh"))
	if len(cookies) != 1 {
		t.Fatalf("refresh cookie count = %d, want 1", len(cookies))
	}
	originalCookie := cookies[0]

	type refreshAttempt struct {
		status int
		body   authResp
		cookie *http.Cookie
		err    error
	}
	results := make(chan refreshAttempt, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			request, requestErr := http.NewRequest(http.MethodPost, base+"/api/auth/refresh", nil)
			if requestErr != nil {
				results <- refreshAttempt{err: requestErr}
				return
			}
			request.AddCookie(originalCookie)
			response, requestErr := http.DefaultClient.Do(request)
			if requestErr != nil {
				results <- refreshAttempt{err: requestErr}
				return
			}
			defer response.Body.Close()
			attempt := refreshAttempt{status: response.StatusCode}
			if response.StatusCode == http.StatusOK {
				attempt.err = json.NewDecoder(response.Body).Decode(&attempt.body)
				for _, cookie := range response.Cookies() {
					if cookie.Name == "vertex_refresh" {
						attempt.cookie = cookie
					}
				}
			}
			results <- attempt
		}()
	}
	wait.Wait()
	close(results)
	statuses := make([]int, 0, 2)
	var rotated refreshAttempt
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		statuses = append(statuses, result.status)
		if result.status == http.StatusOK {
			rotated = result
		}
	}
	sort.Ints(statuses)
	if statuses[0] != http.StatusOK || statuses[1] != http.StatusUnauthorized {
		t.Fatalf("concurrent refresh statuses = %v, want [200 401]", statuses)
	}
	if rotated.cookie == nil || rotated.body.AccessToken == "" {
		t.Fatal("successful refresh did not rotate both credentials")
	}

	logoutRequest, err := http.NewRequest(http.MethodPost, base+"/api/auth/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	logoutRequest.Header.Set("Authorization", "Bearer "+rotated.body.AccessToken)
	logoutRequest.AddCookie(rotated.cookie)
	logoutResponse, err := http.DefaultClient.Do(logoutRequest)
	if err != nil {
		t.Fatal(err)
	}
	logoutResponse.Body.Close()
	if logoutResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", logoutResponse.StatusCode)
	}
	if err := httpJSON(http.MethodGet, base+"/api/auth/me", rotated.body.AccessToken, nil, nil, http.StatusUnauthorized); err != nil {
		t.Fatalf("revoked access token: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, base+"/api/auth/refresh", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(rotated.cookie)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("refresh after logout status = %d, want 401", response.StatusCode)
	}
}

func TestEndToEndJudgeFencing(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	judgeToken := envOr("E2E_JUDGE_API_TOKEN", "vertex-dev-judge-token")
	adminToken := mustLogin(t, base, envOr("E2E_ADMIN_USER", "admin"), envOr("E2E_ADMIN_PASS", "admin123"))
	problemID := createProblem(t, base, adminToken, "Judge fencing", "print 1")
	uploadTestdata(t, base, adminToken, problemID, map[string]string{"1.in": "\n", "1.out": "1\n"})
	publishProblem(t, base, adminToken, problemID)
	userToken, _ := registerUser(t, base)
	submissionID := submit(t, base, userToken, problemID, "cpp", "int main(){}")

	var job judgeJob
	if err := httpJSON(http.MethodPost, base+"/internal/judge/v1/jobs/claim", judgeToken,
		map[string]any{"workerId": "e2e-fencing", "waitSeconds": 1, "capabilities": []string{"cpp"}},
		&job, http.StatusOK); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if job.SubmissionID != submissionID {
		t.Fatalf("claimed submission = %q, want %q", job.SubmissionID, submissionID)
	}
	result := map[string]any{
		"workerId": "e2e-fencing", "submissionId": submissionID, "generation": job.Generation,
		"leaseToken": job.LeaseToken, "status": "Accepted", "score": 100,
		"totalTimeMs": 1, "peakMemoryKb": 1024,
		"cases": []map[string]any{{"caseIndex": 1, "verdict": "Accepted", "timeMs": 1, "memoryKb": 1024}},
	}
	stale := maps.Clone(result)
	stale["leaseToken"] = "00000000-0000-0000-0000-000000000000"
	if err := httpJSON(http.MethodPut, base+"/internal/judge/v1/jobs/"+job.JobID+"/result",
		judgeToken, stale, nil, http.StatusConflict); err != nil {
		t.Fatalf("stale result: %v", err)
	}
	if err := httpJSON(http.MethodPut, base+"/internal/judge/v1/jobs/"+job.JobID+"/result",
		judgeToken, result, nil, http.StatusNoContent); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if err := httpJSON(http.MethodPut, base+"/internal/judge/v1/jobs/"+job.JobID+"/result",
		judgeToken, result, nil, http.StatusNoContent); err != nil {
		t.Fatalf("idempotent complete: %v", err)
	}
	staleSubmissionID := submit(t, base, userToken, problemID, "cpp", "int main(){}")
	var staleJob judgeJob
	if err := httpJSON(http.MethodPost, base+"/internal/judge/v1/jobs/claim", judgeToken,
		map[string]any{"workerId": "e2e-fencing", "waitSeconds": 1, "capabilities": []string{"cpp"}},
		&staleJob, http.StatusOK); err != nil {
		t.Fatalf("claim stale-generation fixture: %v", err)
	}
	if staleJob.SubmissionID != staleSubmissionID {
		t.Fatalf("claimed submission = %q, want %q", staleJob.SubmissionID, staleSubmissionID)
	}
	staleGenerationResult := map[string]any{
		"workerId": "e2e-fencing", "submissionId": staleSubmissionID, "generation": staleJob.Generation,
		"leaseToken": staleJob.LeaseToken, "status": "Accepted", "score": 100,
		"totalTimeMs": 1, "peakMemoryKb": 1024,
		"cases": []map[string]any{{"caseIndex": 1, "verdict": "Accepted", "timeMs": 1, "memoryKb": 1024}},
	}
	if err := httpJSON(http.MethodPost, base+"/api/admin/submissions/"+staleSubmissionID+"/rejudge",
		adminToken, nil, nil, http.StatusOK); err != nil {
		t.Fatalf("rejudge: %v", err)
	}
	if err := httpJSON(http.MethodPut, base+"/internal/judge/v1/jobs/"+staleJob.JobID+"/result",
		judgeToken, staleGenerationResult, nil, http.StatusConflict); err != nil {
		t.Fatalf("old generation result: %v", err)
	}
}

// ---- 测试用例 ----

const acCpp = `#include <bits/stdc++.h>
int main(){ long long a,b; std::cin>>a>>b; std::cout<<a+b; return 0; }`

const acPython = `import sys
a,b = map(int, sys.stdin.read().split())
print(a+b)`

// TestEndToEndCore 覆盖判题核心:AC(多语言)/WA/TLE/CE/OLE 与逐测试点结果。
func TestEndToEndCore(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	admin := envOr("E2E_ADMIN_USER", "admin")
	adminPass := envOr("E2E_ADMIN_PASS", "admin123")

	adminTok := mustLogin(t, base, admin, adminPass)
	pid := createProblem(t, base, adminTok, "A+B",
		"给定两个整数,输出它们的和。\n\n输入格式:一行两个整数 $a, b$。\n\n输出格式:一行一个整数 $a+b$。")
	uploadTestdata(t, base, adminTok, pid, map[string]string{
		"1.in": "1 2\n", "1.out": "3\n",
		"2.in": "1000000000 2000000000\n", "2.out": "3000000000\n",
	})
	publishProblem(t, base, adminTok, pid)

	userTok, _ := registerUser(t, base)

	// 两份相同 C++ AC 背靠背入队，覆盖并发 Worker 的独立 isolate box 与编译缓存竞争。
	sid := submit(t, base, userTok, pid, "cpp", acCpp)
	concurrentSID := submit(t, base, userTok, pid, "cpp", acCpp)
	s := waitForSubmission(t, base, userTok, sid, 2*time.Minute)
	assertVerdict(t, s, "Accepted")
	concurrentSubmission := waitForSubmission(t, base, userTok, concurrentSID, 2*time.Minute)
	assertVerdict(t, concurrentSubmission, "Accepted")
	if len(s.CaseResults) != 2 {
		t.Errorf("expected 2 case results, got %d", len(s.CaseResults))
	}
	if s.CaseResults[0].Verdict != "Accepted" {
		t.Errorf("case 1 verdict = %q, want Accepted", s.CaseResults[0].Verdict)
	}

	// Python AC(解释型语言路径)
	sid = submit(t, base, userTok, pid, "python", acPython)
	s = waitForSubmission(t, base, userTok, sid, 2*time.Minute)
	assertVerdict(t, s, "Accepted")

	// C++ WA
	sid = submit(t, base, userTok, pid, "cpp",
		`#include <bits/stdc++.h>
int main(){ long long a,b; std::cin>>a>>b; std::cout<<a*b; return 0; }`)
	s = waitForSubmission(t, base, userTok, sid, 2*time.Minute)
	assertVerdict(t, s, "Wrong Answer")

	// C++ TLE(空转 CPU,撞 1s 时间上限)
	sid = submit(t, base, userTok, pid, "cpp",
		`int main(){ volatile long long x=0; while(true) x++; return 0; }`)
	s = waitForSubmission(t, base, userTok, sid, 2*time.Minute)
	assertVerdict(t, s, "Time Limit Exceeded")

	// C++ CE(编译错误)
	sid = submit(t, base, userTok, pid, "cpp", `int main() { this is not valid c++`)
	s = waitForSubmission(t, base, userTok, sid, 2*time.Minute)
	assertVerdict(t, s, "Compile Error")

	// C++ OLE(SIGXFSZ,撞 32MB 文件大小上限)
	sid = submit(t, base, userTok, pid, "cpp",
		`#include <iostream>
int main(){ while(true) std::cout << "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n"; }`)
	s = waitForSubmission(t, base, userTok, sid, 2*time.Minute)
	assertVerdict(t, s, "Output Limit Exceeded")
}

// TestEndToEndJudgeRecovery 在 Web 重启后验证 Judge 的退避重连与继续领任务。
func TestEndToEndJudgeRecovery(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	adminTok := mustLogin(t, base, envOr("E2E_ADMIN_USER", "admin"), envOr("E2E_ADMIN_PASS", "admin123"))
	pid := createProblem(t, base, adminTok, "Judge reconnect", "Web 重启后的 Judge 恢复测试。")
	uploadTestdata(t, base, adminTok, pid, map[string]string{"1.in": "20 22\n", "1.out": "42\n"})
	publishProblem(t, base, adminTok, pid)
	userTok, _ := registerUser(t, base)
	sid := submit(t, base, userTok, pid, "cpp", acCpp)
	assertVerdict(t, waitForSubmission(t, base, userTok, sid, 2*time.Minute), "Accepted")
}

// TestEndToEndContest 覆盖比赛链路:建赛 → 设题 → 报名 → 赛内提交 → 榜单积分。
func TestEndToEndContest(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	adminTok := mustLogin(t, base, envOr("E2E_ADMIN_USER", "admin"), envOr("E2E_ADMIN_PASS", "admin123"))

	pid := createProblem(t, base, adminTok, "Contest A+B", "read a, b, print a+b")
	uploadTestdata(t, base, adminTok, pid, map[string]string{"1.in": "1 2\n", "1.out": "3\n"})
	publishProblem(t, base, adminTok, pid)

	// 比赛 5 秒后开始(报名必须在开始前)
	begin := time.Now().Add(5 * time.Second).Format(time.RFC3339)
	end := time.Now().Add(2 * time.Hour).Format(time.RFC3339)
	body := map[string]any{
		"title": "E2E Contest", "description": "end-to-end", "rule": "acm",
		"beginAt": begin, "endAt": end, "visibility": "public", "rankboardVisible": true,
	}
	var c contest
	if err := httpJSON(http.MethodPost, base+"/api/admin/contests", adminTok, body, &c, 201); err != nil {
		t.Fatalf("create contest: %v", err)
	}
	if err := httpJSON(http.MethodPut, base+"/api/admin/contests/"+c.ID+"/problems",
		adminTok, map[string]any{"problemIds": []string{pid}}, nil, 200); err != nil {
		t.Fatalf("set contest problems: %v", err)
	}

	userTok, username := registerUser(t, base)
	if err := httpJSON(http.MethodPost, base+"/api/contests/"+c.ID+"/register", userTok, nil, nil, 200); err != nil {
		t.Fatalf("register contest: %v", err)
	}

	// 等待比赛开始
	beginTime, err := time.Parse(time.RFC3339, c.BeginAt)
	if err != nil {
		t.Fatalf("parse beginAt %q: %v", c.BeginAt, err)
	}
	if wait := time.Until(beginTime); wait > 0 {
		time.Sleep(wait)
	}
	time.Sleep(time.Second)

	// 比赛内提交 AC
	sid := submit(t, base, userTok, pid, "cpp", acCpp, c.ID)
	s := waitForSubmission(t, base, userTok, sid, 2*time.Minute)
	assertVerdict(t, s, "Accepted")

	// 榜单应显示该用户 1 题通过
	var board rankboard
	if err := httpJSON(http.MethodGet, base+"/api/contests/"+c.ID+"/rankboard", userTok, nil, &board, 200); err != nil {
		t.Fatalf("rankboard: %v", err)
	}
	found := false
	for _, row := range board.Rows {
		if row.Username == username {
			found = true
			if row.Solved != 1 {
				t.Errorf("row.Solved = %d, want 1", row.Solved)
			}
			if len(row.Cells) != 1 {
				t.Errorf("len(row.Cells) = %d, want 1", len(row.Cells))
			}
			if row.Cells[0].SolvedAt == nil {
				t.Error("cell.SolvedAt should be set after AC")
			}
		}
	}
	if !found {
		t.Errorf("user %q not found in rankboard rows", username)
	}

	// AC 重判后比赛积分格必须保持幂等；公开题目统计仅包含练习提交，
	// 不能通过它泄露封榜或隐藏反馈的比赛结果。
	if err := httpJSON(http.MethodPost, base+"/api/admin/submissions/"+sid+"/rejudge",
		adminTok, nil, nil, 200); err != nil {
		t.Fatalf("rejudge: %v", err)
	}
	s = waitForSubmission(t, base, userTok, sid, 2*time.Minute)
	assertVerdict(t, s, "Accepted")
	if err := httpJSON(http.MethodGet, base+"/api/contests/"+c.ID+"/rankboard", userTok, nil, &board, 200); err != nil {
		t.Fatalf("rankboard after rejudge: %v", err)
	}
	for _, row := range board.Rows {
		if row.Username == username && (row.Solved != 1 || row.Cells[0].Attempts != 1) {
			t.Errorf("rankboard changed after rejudge: solved=%d attempts=%d", row.Solved, row.Cells[0].Attempts)
		}
	}
	var p problemResp
	if err := httpJSON(http.MethodGet, base+"/api/problems/"+pid, userTok, nil, &p, 200); err != nil {
		t.Fatalf("problem after rejudge: %v", err)
	}
	if p.SubmissionCount != 0 || p.AcceptedCount != 0 || p.SolvedUserCount != 0 {
		t.Errorf("contest rejudge leaked into practice counters: submissions:%d accepted:%d solvedUsers:%d, want 0/0/0",
			p.SubmissionCount, p.AcceptedCount, p.SolvedUserCount)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
