package e2e

import (
	"encoding/json"
	"fmt"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

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
    ouf.skipBlanks(); // The model solution prints a trailing newline.
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

func publishProblem(t *testing.T, base, token, problemID string) int {
	t.Helper()
	return publishFixtureAt(t, base+"/api/domains/official/authoring/problems/"+problemID, token)
}

func TestEndToEndProblemAuthoring(t *testing.T) {
	base := apiBase(t)
	adminUser, adminPass := os.Getenv("E2E_ADMIN_USER"), os.Getenv("E2E_ADMIN_PASS")
	if adminUser == "" || adminPass == "" {
		t.Skip("E2E administrator credentials not set")
	}
	admin := mustLogin(t, base, adminUser, adminPass)
	problemID := createProblem(t, base, admin, fmt.Sprintf("authoring-%d", time.Now().UnixNano()%1000000), "")
	endpoint := base + "/api/domains/official/authoring/problems/" + problemID
	var copy domain.WorkingCopy
	apiCall(t, http.MethodPost, endpoint+"/working-copy", admin, nil, &copy, 200)
	save := func(id, path, kind, text string) {
		t.Helper()
		attributes := map[string]string{}
		if kind == domain.EntryStatement {
			attributes = map[string]string{"format": "markdown", "language": "zh"}
		}
		apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/"+id, admin, map[string]any{"etag": copy.ETag, "entry": domain.TreeEntry{ID: id, Path: path, Kind: kind, Attributes: attributes}, "text": text}, &copy, 200)
	}
	doc := func(id, path, kind string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		save(id, path, kind, string(data))
	}
	save("statement-zh", "statement/problem.zh.md", domain.EntryStatement, "# 数列求和\n\n## 题目描述\n\n给定 n 个整数，输出它们的和。\n\n## 输入格式\n\n第一行 n，第二行 n 个整数。\n\n## 输出格式\n\n输出和。\n")
	var inspection domain.MaterialInspection
	apiCall(t, http.MethodGet, endpoint+"/inspection", admin, nil, &inspection, 200)
	if inspection.CanBuild {
		t.Fatal("empty package was buildable")
	}
	apiCall(t, http.MethodPost, endpoint+"/checks", admin, domain.CheckSelection{ETag: copy.ETag}, nil, 400)
	program := func(id, role, protocol, source string, expected []string) {
		t.Helper()
		save(id+"-source", "programs/"+id+"/main.cpp", domain.EntrySource, source)
		doc(id, "vertex/programs/"+id+".json", domain.EntryProgram, domain.ProgramMaterial{SchemaVersion: 1, Name: id, Directory: "programs/" + id, Role: role, Language: "cpp", Protocol: protocol, Files: []string{id + "-source"}, EntryPoint: id + "-source", ExpectedVerdicts: expected})
	}
	program("std", "solution", "stdio", sumSolution, []string{"Accepted"})
	program("drop_last", "solution", "stdio", wrongSolution, []string{"Wrong Answer"})
	program("check", "output-validator", "testlib", sumChecker, nil)
	program("validate", "input-validator", "testlib", sumValidator, nil)
	program("gen", "generator", "stdio", sumGenerator, nil)
	save("sample-input", "data/sample.in", domain.EntryInput, "3\n1 2 3\n")
	doc("sample", "vertex/tests/sample.json", domain.EntryTest, domain.TestMaterial{SchemaVersion: 1, Name: "Sample", IsSample: true, Input: domain.TestInput{Kind: "file", Entry: "sample-input"}, Answer: domain.TestAnswer{Kind: "solution", Solution: "std"}})
	for _, count := range []int{50, 1000} {
		id := fmt.Sprintf("generated-%d", count)
		doc(id, "vertex/tests/"+id+".json", domain.EntryTest, domain.TestMaterial{SchemaVersion: 1, Name: id, Input: domain.TestInput{Kind: "generator", Generator: "gen", Arguments: []string{fmt.Sprint(count)}}, Answer: domain.TestAnswer{Kind: "solution", Solution: "std"}})
	}
	var meta domain.MaterialView
	apiCall(t, http.MethodGet, endpoint+"/materials/problem", admin, nil, &meta, 200)
	meta.Metadata.Title = "数列求和"
	meta.Metadata.MainSolution = "std"
	meta.Metadata.InputValidators = []string{"validate"}
	meta.Metadata.OutputValidator = "check"
	meta.Metadata.Comparison = domain.OutputComparison{Kind: "testlib"}
	doc("problem", "vertex/problem.json", domain.EntryMetadata, meta.Metadata)
	// Shell command strings are not a second execution path: only argument arrays.
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/unsafe", admin, map[string]any{"etag": copy.ETag, "entry": domain.TreeEntry{ID: "unsafe", Path: "vertex/tests/unsafe.json", Kind: domain.EntryTest}, "text": `{"schemaVersion":1,"name":"unsafe","generateCmd":"gen 5; id"}`}, nil, 400)
	var check domain.CheckRun
	apiCall(t, http.MethodPost, endpoint+"/checks", admin, domain.CheckSelection{ETag: copy.ETag}, &check, 200)
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		apiCall(t, http.MethodGet, endpoint+"/checks/"+check.ID, admin, nil, &check, 200)
		if check.State != "queued" && check.State != "running" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if check.State != "succeeded" || check.PackageCases != 3 || len(check.Solutions) != 2 {
		t.Fatalf("native testlib check failed: %+v", check)
	}
	foundWrong, foundSample := false, false
	for _, solution := range check.Solutions {
		if solution.Name == "drop_last" {
			foundWrong = solution.Matched && solution.ActualVerdict == "Wrong Answer"
		}
	}
	for _, test := range check.Tests {
		if test.IsSample {
			foundSample = strings.TrimSpace(test.AnswerHead) == "6"
		}
	}
	if !foundWrong || !foundSample {
		t.Fatal("check lost wrong-reference or generated-answer validation")
	}
	apiCall(t, http.MethodGet, base+"/api/domains/official/problems/"+problemID, "", nil, nil, 404)
	if publishProblem(t, base, admin, problemID) != 1 {
		t.Fatal("first release is not v1")
	}
	var published struct {
		StatementMD string `json:"statementMd"`
	}
	apiCall(t, http.MethodGet, base+"/api/domains/official/problems/"+problemID, "", nil, &published, 200)
	for _, value := range []string{"## 题目描述", "## 样例", "1 2 3", "6"} {
		if !strings.Contains(published.StatementMD, value) {
			t.Fatalf("published statement missing %s", value)
		}
	}
	user, _ := registerUser(t, base)
	accepted := waitForSubmission(t, base, user, submit(t, base, user, problemID, "cpp", sumSolution), 3*time.Minute)
	assertVerdict(t, accepted, "Accepted")
	rejected := waitForSubmission(t, base, user, submit(t, base, user, problemID, "cpp", wrongSolution), 3*time.Minute)
	assertVerdict(t, rejected, "Wrong Answer")
	apiCall(t, http.MethodGet, endpoint+"/working-copy", admin, nil, &copy, 200)
	save("statement-zh", "statement/problem.zh.md", domain.EntryStatement, "# 数列求和\n\n尚未发布的新描述。\n")
	apiCall(t, http.MethodGet, base+"/api/domains/official/problems/"+problemID, "", nil, &published, 200)
	if strings.Contains(published.StatementMD, "尚未发布") {
		t.Fatal("working copy changed the public statement")
	}
	if publishProblem(t, base, admin, problemID) != 2 {
		t.Fatal("second release is not v2")
	}
	if retained := waitForSubmission(t, base, user, accepted.ID, time.Minute); retained.ProblemVersion != 1 {
		t.Fatal("new publication changed an existing evaluation")
	}
}
