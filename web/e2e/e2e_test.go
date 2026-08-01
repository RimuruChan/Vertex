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
	"mime/multipart"
	"net/http"
	"os"
	"testing"
	"time"
)

// ---- API 响应结构(与后端 model 对齐)----

type authResp struct {
	Token string `json:"token"`
	User  struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	} `json:"user"`
}

type problemResp struct {
	ID         string `json:"id"`
	Visibility string `json:"visibility"`
}

type caseResult struct {
	CaseIndex int    `json:"caseIndex"`
	Verdict   string `json:"verdict"`
	TimeMs    int    `json:"timeMs"`
	MemoryKb  int    `json:"memoryKb"`
}

type submission struct {
	ID           string       `json:"id"`
	ProblemID    string       `json:"problemId"`
	Language     string       `json:"language"`
	Status       string       `json:"status"`
	Score        int          `json:"score"`
	TotalTimeMs  int          `json:"totalTimeMs"`
	PeakMemoryKb int          `json:"peakMemoryKb"`
	CaseResults  []caseResult `json:"caseResults"`
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
	resp, err := http.DefaultClient.Do(req)
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

func mustLogin(t *testing.T, base, user, pass string) string {
	t.Helper()
	var out authResp
	if err := httpJSON(http.MethodPost, base+"/api/auth/login", "",
		map[string]string{"username": user, "password": pass}, &out, 200); err != nil {
		t.Fatalf("login: %v", err)
	}
	return out.Token
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
	return out.Token, username
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

	req, err := http.NewRequest(http.MethodPost, base+"/api/admin/problems/"+problemID+"/testdata", &body)
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

func submit(t *testing.T, base, token, problemID, lang, code string) string {
	t.Helper()
	body := map[string]string{"problemId": problemID, "language": lang, "sourceCode": code}
	var s submission
	if err := httpJSON(http.MethodPost, base+"/api/submissions", token, body, &s, 202); err != nil {
		t.Fatalf("submit: %v", err)
	}
	return s.ID
}

func isTerminal(status string) bool {
	return status != "Pending" && status != "Judging"
}

// waitForSubmission 轮询提交详情直到终态(判题 worker 轮询 500ms,预留充足超时)。
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
		t.Fatalf("verdict = %q, want %q (caseResults=%+v)", s.Status, want, s.CaseResults)
	}
}

// ---- 测试用例 ----

const acCpp = `#include <bits/stdc++.h>
int main(){ long long a,b; std::cin>>a>>b; std::cout<<a+b; return 0; }`

const acPython = `import sys
a,b = map(int, sys.stdin.read().split())
print(a+b)`

// TestEndToEndCore 覆盖判题核心:AC(多语言)/WA/TLE/CE 与逐测试点结果。
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

	userTok, _ := registerUser(t, base)

	// C++ AC
	sid := submit(t, base, userTok, pid, "cpp", acCpp)
	s := waitForSubmission(t, base, userTok, sid, 2*time.Minute)
	assertVerdict(t, s, "Accepted")
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
}

// TestEndToEndContest 覆盖比赛链路:建赛 → 设题 → 报名 → 赛内提交 → 榜单积分。
func TestEndToEndContest(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	adminTok := mustLogin(t, base, "admin", "admin123")

	pid := createProblem(t, base, adminTok, "Contest A+B", "read a, b, print a+b")
	uploadTestdata(t, base, adminTok, pid, map[string]string{"1.in": "1 2\n", "1.out": "3\n"})

	// 比赛 5 秒后开始(报名必须在开始前)
	begin := time.Now().Add(5 * time.Second).Format(time.RFC3339)
	end := time.Now().Add(2 * time.Hour).Format(time.RFC3339)
	body := map[string]any{
		"title": "E2E Contest", "description": "end-to-end", "rule": "acm",
		"beginAt": begin, "endAt": end, "visibility": "public",
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
	sid := submit(t, base, userTok, pid, "cpp", acCpp)
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
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
