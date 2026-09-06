package e2e

// 出题端到端测试:题面 → testlib checker/validator/generator → 标程 → 构建 → 判题。
// 与其它 E2E 一样,只在设置 E2E_BASE_URL 时运行,并且需要一个启用了构建循环
// (BUILD_WORKER_ENABLED)的 worker。

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// ---- 出题 API 响应结构 ----

type packageMeta struct {
	PackageRevision int    `json:"packageRevision"`
	BuiltRevision   int    `json:"builtRevision"`
	TestdataCases   int    `json:"testdataCases"`
	TestdataChecker string `json:"testdataChecker"`
	Stale           bool   `json:"stale"`
	Title           string `json:"title"`
}

type buildTestOutcome struct {
	Index      int    `json:"index"`
	Status     string `json:"status"`
	IsSample   bool   `json:"isSample"`
	InputHead  string `json:"inputHead"`
	AnswerHead string `json:"answerHead"`
	Message    string `json:"message"`
}

type buildSolutionOutcome struct {
	Name            string `json:"name"`
	ExpectedVerdict string `json:"expectedVerdict"`
	ActualVerdict   string `json:"actualVerdict"`
	Matched         bool   `json:"matched"`
}

type buildResponse struct {
	ID           string                 `json:"id"`
	State        string                 `json:"state"`
	Stage        string                 `json:"stage"`
	Log          string                 `json:"log"`
	ErrorMessage string                 `json:"errorMessage"`
	PackageCases int                    `json:"packageCases"`
	Tests        []buildTestOutcome     `json:"tests"`
	Solutions    []buildSolutionOutcome `json:"solutions"`
}

type workspaceResponse struct {
	Meta        packageMeta    `json:"meta"`
	Issues      []string       `json:"issues"`
	LatestBuild *buildResponse `json:"latestBuild"`
	Tests       []struct {
		ID    int64 `json:"id"`
		Index int   `json:"index"`
	} `json:"tests"`
}

// ---- 题目包源文件 ----

const sumSolution = `#include <bits/stdc++.h>
int main(){int n; if(!(std::cin>>n)) return 0; long long s=0; for(int i=0;i<n;i++){long long v; std::cin>>v; s+=v;} std::cout<<s<<'\n';}
`

// 故意漏掉最后一个数,用来验证构建期对拍能抓到 Wrong Answer。
const wrongSolution = `#include <bits/stdc++.h>
int main(){int n; if(!(std::cin>>n)) return 0; long long s=0; for(int i=0;i<n;i++){long long v; std::cin>>v; if(i+1<n) s+=v;} std::cout<<s<<'\n';}
`

const sumChecker = `#include "testlib.h"
int main(int argc, char* argv[]) {
    registerTestlibCmd(argc, argv);
    long long jury = ans.readLong();
    long long participant = ouf.readLong();
    if (jury != participant) quitf(_wa, "expected %lld, found %lld", jury, participant);
    ouf.readEof();
    quitf(_ok, "sum = %lld", jury);
}
`

const sumValidator = `#include "testlib.h"
int main(int argc, char* argv[]) {
    registerValidation(argc, argv);
    int n = inf.readInt(1, 1000, "n");
    inf.readEoln();
    for (int i = 0; i < n; i++) {
        inf.readInt(-1000000, 1000000, "a");
        if (i + 1 < n) inf.readSpace();
    }
    inf.readEoln();
    inf.readEof();
}
`

const sumGenerator = `#include "testlib.h"
int main(int argc, char* argv[]) {
    registerGen(argc, argv, 1);
    int n = atoi(argv[1]);
    println(n);
    for (int i = 0; i < n; i++) std::cout << rnd.next(-1000, 1000) << " \n"[i + 1 == n];
    return 0;
}
`

func savePackageFile(t *testing.T, base, token, problemID string, body map[string]any) {
	t.Helper()
	if err := httpJSON(http.MethodPut, base+"/api/admin/problems/"+problemID+"/files",
		token, body, nil, http.StatusOK); err != nil {
		t.Fatalf("save package file %v: %v", body["name"], err)
	}
}

func addPackageTest(t *testing.T, base, token, problemID string, body map[string]any) {
	t.Helper()
	if err := httpJSON(http.MethodPost, base+"/api/admin/problems/"+problemID+"/tests",
		token, body, nil, http.StatusCreated); err != nil {
		t.Fatalf("add package test: %v", err)
	}
}

func loadWorkspace(t *testing.T, base, token, problemID string) workspaceResponse {
	t.Helper()
	var workspace workspaceResponse
	if err := httpJSON(http.MethodGet, base+"/api/admin/problems/"+problemID+"/package",
		token, nil, &workspace, http.StatusOK); err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	return workspace
}

// waitForBuild 轮询构建直到进入终态。构建要编译四个程序并跑完所有测试点,
// 因此超时比判题宽松得多。
func waitForBuild(t *testing.T, base, token, problemID, buildID string, timeout time.Duration) buildResponse {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var build buildResponse
		err := httpJSON(http.MethodGet,
			base+"/api/admin/problems/"+problemID+"/builds/"+buildID, token, nil, &build, http.StatusOK)
		if err == nil && build.State != "queued" && build.State != "running" {
			return build
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("build %s did not finish within %s", buildID, timeout)
	return buildResponse{}
}

func startBuild(t *testing.T, base, token, problemID string) buildResponse {
	t.Helper()
	var build buildResponse
	if err := httpJSON(http.MethodPost, base+"/api/admin/problems/"+problemID+"/builds",
		token, nil, &build, http.StatusAccepted); err != nil {
		t.Fatalf("start build: %v", err)
	}
	return build
}

// TestEndToEndProblemAuthoring 覆盖完整出题链路:结构化题面渲染、testlib
// 三件套编译、数据生成与校验、标程产出答案、构建期对拍,以及构建产物被真实
// 判题使用。
func TestEndToEndProblemAuthoring(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	adminUser := os.Getenv("E2E_ADMIN_USER")
	adminPass := os.Getenv("E2E_ADMIN_PASS")
	if adminUser == "" || adminPass == "" {
		t.Skip("E2E_ADMIN_USER/E2E_ADMIN_PASS not set")
	}
	admin := mustLogin(t, base, adminUser, adminPass)
	problemID := createProblem(t, base, admin, fmt.Sprintf("authoring-%d", time.Now().UnixNano()%1000000), "")

	// 结构化题面。样例区块由构建产生,这里只写正文。
	statement := map[string]any{
		"name":         "数列求和",
		"legend":       "给定 $n$ 个整数,输出它们的和。",
		"inputFormat":  "第一行一个整数 $n$,第二行 $n$ 个整数。",
		"outputFormat": "一行一个整数表示答案。",
		"notes":        "$1 \\le n \\le 1000$",
	}
	if err := httpJSON(http.MethodPut, base+"/api/admin/problems/"+problemID+"/statements/zh",
		admin, statement, nil, http.StatusOK); err != nil {
		t.Fatalf("save statement: %v", err)
	}

	// 未准备测试点和标程时不允许构建。
	workspace := loadWorkspace(t, base, admin, problemID)
	if len(workspace.Issues) == 0 {
		t.Fatal("empty package reported no build blockers")
	}
	if err := httpJSON(http.MethodPost, base+"/api/admin/problems/"+problemID+"/builds",
		admin, nil, nil, http.StatusBadRequest); err != nil {
		t.Fatalf("build of an incomplete package was not rejected: %v", err)
	}

	savePackageFile(t, base, admin, problemID, map[string]any{
		"kind": "solution", "name": "std", "language": "cpp",
		"sourceCode": sumSolution, "isActive": true,
	})
	savePackageFile(t, base, admin, problemID, map[string]any{
		"kind": "solution", "name": "drop_last", "language": "cpp",
		"sourceCode": wrongSolution, "expectedVerdict": "Wrong Answer",
	})
	savePackageFile(t, base, admin, problemID, map[string]any{
		"kind": "checker", "name": "check", "language": "cpp", "sourceCode": sumChecker,
	})
	savePackageFile(t, base, admin, problemID, map[string]any{
		"kind": "validator", "name": "validate", "language": "cpp", "sourceCode": sumValidator,
	})
	savePackageFile(t, base, admin, problemID, map[string]any{
		"kind": "generator", "name": "gen", "language": "cpp", "sourceCode": sumGenerator,
	})

	// 一个手工样例 + 两个生成器测试点。
	addPackageTest(t, base, admin, problemID, map[string]any{
		"source": "manual", "inputData": "3\n1 2 3", "isSample": true,
	})
	addPackageTest(t, base, admin, problemID, map[string]any{
		"source": "generator", "generateCmd": "gen 50",
	})
	addPackageTest(t, base, admin, problemID, map[string]any{
		"source": "generator", "generateCmd": "gen 1000",
	})

	// checker 只能用 C++:testlib 是 C++ 头文件。
	if err := httpJSON(http.MethodPut, base+"/api/admin/problems/"+problemID+"/files", admin,
		map[string]any{"kind": "checker", "name": "bad", "language": "python", "sourceCode": "print(1)"},
		nil, http.StatusBadRequest); err != nil {
		t.Fatalf("non-C++ checker was not rejected: %v", err)
	}
	// 生成命令不经过 shell,含元字符必须被拒绝。
	if err := httpJSON(http.MethodPost, base+"/api/admin/problems/"+problemID+"/tests", admin,
		map[string]any{"source": "generator", "generateCmd": "gen 5; id"},
		nil, http.StatusBadRequest); err != nil {
		t.Fatalf("unsafe generate command was not rejected: %v", err)
	}

	workspace = loadWorkspace(t, base, admin, problemID)
	if len(workspace.Issues) != 0 {
		t.Fatalf("package is still not buildable: %v", workspace.Issues)
	}
	if !workspace.Meta.Stale {
		t.Fatal("edited package was not reported as stale")
	}

	queued := startBuild(t, base, admin, problemID)
	build := waitForBuild(t, base, admin, problemID, queued.ID, 10*time.Minute)
	if build.State != "succeeded" {
		t.Fatalf("build state = %q (stage %s): %s\n%s",
			build.State, build.Stage, build.ErrorMessage, build.Log)
	}
	if build.PackageCases != 3 {
		t.Fatalf("build produced %d cases, want 3", build.PackageCases)
	}
	// 对拍必须发现故意写错的解确实是 Wrong Answer。
	if len(build.Solutions) != 1 {
		t.Fatalf("build reported %d alternate solutions, want 1", len(build.Solutions))
	}
	if !build.Solutions[0].Matched || build.Solutions[0].ActualVerdict != "Wrong Answer" {
		t.Fatalf("alternate solution outcome = %+v", build.Solutions[0])
	}
	// 样例测试点的答案由标程产生,应当是 1+2+3。
	var sample *buildTestOutcome
	for i := range build.Tests {
		if build.Tests[i].IsSample {
			sample = &build.Tests[i]
		}
	}
	if sample == nil {
		t.Fatal("build report contains no sample test")
	}
	if got := strings.TrimSpace(sample.AnswerHead); got != "6" {
		t.Fatalf("sample answer = %q, want 6", got)
	}

	workspace = loadWorkspace(t, base, admin, problemID)
	if workspace.Meta.Stale {
		t.Fatal("package is still stale after a successful build")
	}
	if workspace.Meta.TestdataCases != 3 || workspace.Meta.TestdataChecker != "testlib" {
		t.Fatalf("published testdata = %d cases / %s checker",
			workspace.Meta.TestdataCases, workspace.Meta.TestdataChecker)
	}
	if workspace.Meta.Title != "数列求和" {
		t.Fatalf("problem title = %q, want the statement name", workspace.Meta.Title)
	}

	// 公开题面必须包含渲染出的样例。
	var published struct {
		StatementMD string `json:"statementMd"`
	}
	if err := httpJSON(http.MethodGet, base+"/api/problems/"+problemID, admin, nil, &published, 200); err != nil {
		t.Fatalf("read published problem: %v", err)
	}
	for _, want := range []string{"## 题目描述", "## 样例", "1 2 3", "6"} {
		if !strings.Contains(published.StatementMD, want) {
			t.Fatalf("published statement is missing %q:\n%s", want, published.StatementMD)
		}
	}

	// 构建产物必须能被真实判题使用,包括 testlib checker。
	userToken, _ := registerUser(t, base)
	accepted := waitForSubmission(t, base, userToken,
		submit(t, base, userToken, problemID, "cpp", sumSolution), 3*time.Minute)
	assertVerdict(t, accepted, "Accepted")

	rejected := waitForSubmission(t, base, userToken,
		submit(t, base, userToken, problemID, "cpp", wrongSolution), 3*time.Minute)
	assertVerdict(t, rejected, "Wrong Answer")
}
