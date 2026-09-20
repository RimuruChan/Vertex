package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// Requires a real worker with the native sandbox. The separate API-only suite
// tests authorization and transactions without claiming this execution proof.
func TestEndToEndFrozenAuthoringCheck(t *testing.T) {
	if os.Getenv("VERTEX_FROZEN_CHECK_E2E") != "1" {
		t.Skip("requires a worker running the frozen-check protocol")
	}
	base := apiBase(t)
	owner, _ := registerUser(t, base)
	reader, readerName := registerUser(t, base)
	space := newDomain(t, base, owner)
	prefix := scopedPrefix(base, space)
	apiCall(t, http.MethodPut, prefix+"/members/"+readerName, owner, map[string]any{"roleKey": "member", "status": "active"}, nil, 200)
	var problem numberedResource
	apiCall(t, http.MethodPost, prefix+"/admin/problems", owner, map[string]any{"title": "Frozen sum", "statementMd": "Read a and b and print their sum.", "visibility": "private"}, &problem, 201)
	apiCall(t, http.MethodPut, prefix+"/admin/problems/"+problem.ID+"/access", owner, map[string]string{"username": readerName, "role": "reader"}, nil, 200)
	endpoint := prefix + "/authoring/problems/" + problem.ID
	var copy domain.WorkingCopy
	apiCall(t, http.MethodPost, endpoint+"/working-copy", owner, nil, &copy, 200)
	save := func(id, path, kind, text string) {
		t.Helper()
		apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/"+id, owner, map[string]any{"etag": copy.ETag, "entry": map[string]any{"id": id, "path": path, "kind": kind, "attributes": map[string]string{}}, "text": text}, &copy, 200)
	}
	doc := func(id, path, kind string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		save(id, path, kind, string(data))
	}
	save("main", "programs/ref/main.cpp", domain.EntrySource, "#include <iostream>\n#include \"include/add.h\"\nint main(){long long a,b;std::cin>>a>>b;std::cout<<add(a,b)<<'\\n';}\n")
	save("header", "programs/ref/include/add.h", domain.EntrySource, "long long add(long long,long long);\n")
	save("helper", "programs/ref/lib/add.cpp", domain.EntrySource, "#include \"../include/add.h\"\nlong long add(long long a,long long b){return a+b;}\n")
	save("validator-source", "programs/validator/main.py", domain.EntrySource, "import sys\nvalues=list(map(int,sys.stdin.read().split()))\nsys.exit(42 if len(values)==2 else 43)\n")
	save("checker-source", "programs/checker/main.py", domain.EntrySource, "import sys\nfrom pathlib import Path\na,b=map(int,Path(sys.argv[1]).read_text().split())\nassert Path(sys.argv[2]).read_text()=='03\\n'\ntry: ok=int(sys.stdin.read())==a+b\nexcept ValueError: ok=False\nPath(sys.argv[3]+'judgemessage.txt').write_text('private expected sum '+str(a+b))\nsys.exit(42 if ok else 43)\n")
	doc("reference", "vertex/programs/reference.json", domain.EntryProgram, domain.ProgramMaterial{SchemaVersion: 1, Name: "reference", Role: "solution", Language: "cpp", Protocol: "stdio", Directory: "programs/ref", Files: []string{"main", "header", "helper"}, EntryPoint: "main", ExpectedVerdicts: []string{"Accepted"}})
	doc("validator", "vertex/programs/validator.json", domain.EntryProgram, domain.ProgramMaterial{SchemaVersion: 1, Name: "validator", Role: "input-validator", Language: "python", Protocol: "kattis", Directory: "programs/validator", Files: []string{"validator-source"}, EntryPoint: "validator-source"})
	doc("checker", "vertex/programs/checker.json", domain.EntryProgram, domain.ProgramMaterial{SchemaVersion: 1, Name: "checker", Role: "output-validator", Language: "python", Protocol: "kattis", Directory: "programs/checker", Files: []string{"checker-source"}, EntryPoint: "checker-source"})
	save("input", "data/1.in", domain.EntryInput, "1 2\n")
	save("answer", "data/1.ans", domain.EntryAnswer, "03\n")
	doc("test1", "vertex/tests/test1.json", domain.EntryTest, domain.TestMaterial{SchemaVersion: 1, Name: "test1", IsSample: true, Input: domain.TestInput{Kind: "file", Entry: "input"}, Answer: domain.TestAnswer{Kind: "file", Entry: "answer"}})
	save("invalid-input", "validation/private-negative.in", domain.EntryInput, "1 2 3\n")
	save("wrong-output", "validation/private-wrong.out", domain.EntryAnswer, "4\n")
	for _, item := range []domain.ValidationMaterial{
		{SchemaVersion: 1, Name: "negative-input", Mode: "invalid_input", Input: "invalid-input"},
		{SchemaVersion: 1, Name: "negative-output", Mode: "invalid_output", Input: "input", Answer: "answer", Output: "wrong-output"},
		{SchemaVersion: 1, Name: "positive-output", Mode: "valid_output", Input: "input", Answer: "answer", Output: "answer"},
	} {
		doc(item.Name, "vertex/validation/"+item.Name+".json", domain.EntryValidation, item)
	}
	var metadata domain.MaterialView
	apiCall(t, http.MethodGet, endpoint+"/materials/problem", owner, nil, &metadata, 200)
	metadata.Metadata.MainSolution = "reference"
	metadata.Metadata.InputValidators = []string{"validator"}
	metadata.Metadata.OutputValidator = "checker"
	metadata.Metadata.Comparison = domain.OutputComparison{Kind: "kattis"}
	doc("problem", "vertex/problem.json", domain.EntryMetadata, metadata.Metadata)
	var inspection domain.MaterialInspection
	apiCall(t, http.MethodGet, endpoint+"/inspection", owner, nil, &inspection, 200)
	if !inspection.CanBuild {
		t.Fatalf("fixture blocked: %+v", inspection.Issues)
	}
	var check domain.CheckRun
	apiCall(t, http.MethodPost, endpoint+"/checks", owner, domain.CheckSelection{ETag: copy.ETag}, &check, 200)
	apiCall(t, http.MethodGet, endpoint+"/checks/"+check.ID, reader, nil, nil, 404)
	var firstCommit domain.CommitOutcome
	apiCall(t, http.MethodPost, endpoint+"/commits", owner, domain.CommitInput{ETag: copy.ETag, RequestID: "checked-first", Message: "Publish reviewed sum"}, &firstCommit, 200)
	copy = firstCommit.Copy
	// The queued snapshot must retain the original input even after this save.
	save("input", "data/1.in", domain.EntryInput, "2 5\n")
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		apiCall(t, http.MethodGet, endpoint+"/checks/"+check.ID, owner, nil, &check, 200)
		if check.State == "succeeded" || check.State == "failed" || check.State == "dead" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if check.State != "succeeded" {
		t.Fatalf("check failed: state=%s stage=%s error=%s log=%s", check.State, check.Stage, check.ErrorMessage, check.Log)
	}
	if len(check.ToolchainKey) != 64 || check.PackageCases != 1 || len(check.Solutions) != 1 || !check.Solutions[0].Matched || len(check.Solutions[0].Cases) != 1 || check.Tests[0].Points != 0 {
		t.Fatalf("incomplete execution report: %+v", check)
	}
	if len(check.Validation) != 3 {
		t.Fatalf("missing validator self-tests: %+v", check)
	}
	for _, result := range check.Validation {
		if result.Status != "ok" {
			t.Fatalf("failed validator self-test: %+v", result)
		}
	}
	if check.Tests[0].InputHead != "1 2\n" || check.Tests[0].AnswerHead != "03\n" {
		t.Fatalf("check did not preserve frozen bytes: %+v", check.Tests)
	}
	var releases struct {
		Items []json.RawMessage `json:"items"`
	}
	apiCall(t, http.MethodGet, endpoint+"/releases", owner, nil, &releases, 200)
	if len(releases.Items) != 0 {
		t.Fatal("successful check published automatically")
	}
	publication := domain.CommitPublication{Revision: firstCommit.Commit.Revision, CheckID: check.ID, ExpectedVersion: 0}
	apiCall(t, http.MethodPost, endpoint+"/releases", reader, publication, nil, 403)
	apiCall(t, http.MethodPut, prefix+"/admin/problems/"+problem.ID+"/access", owner, map[string]string{"username": readerName, "role": "editor"}, nil, 200)
	apiCall(t, http.MethodPost, endpoint+"/releases", reader, publication, nil, 403)
	var released domain.CommitRelease
	apiCall(t, http.MethodPost, endpoint+"/releases", owner, publication, &released, 200)
	apiCall(t, http.MethodPost, endpoint+"/releases", owner, publication, &released, 200)
	if released.Version != 1 || released.Revision != firstCommit.Commit.Revision {
		t.Fatalf("publication was not idempotent: %+v", released)
	}
	var view struct {
		StatementMD      string `json:"statementMd"`
		PublishedVersion int    `json:"publishedVersion"`
	}
	apiCall(t, http.MethodGet, prefix+"/problems/"+problem.ID, owner, nil, &view, 200)
	if !strings.Contains(view.StatementMD, "03") || strings.Contains(view.StatementMD, "2 5") {
		t.Fatal("publication used current draft or truncated check preview")
	}
	var publicJSON json.RawMessage
	apiCall(t, http.MethodGet, prefix+"/problems/"+problem.ID, owner, nil, &publicJSON, 200)
	for _, secret := range []string{"negative-input", "wrong-output", "positive-output", "validation/private", `"validation"`} {
		if strings.Contains(string(publicJSON), secret) {
			t.Fatalf("public DTO leaked self-test %s", secret)
		}
	}
	student, studentName := registerUser(t, base)
	apiCall(t, http.MethodPut, prefix+"/members/"+studentName, owner, map[string]any{"roleKey": "member", "status": "active"}, nil, 200)
	var competition numberedResource
	apiCall(t, http.MethodPost, prefix+"/admin/contests", owner, map[string]any{"title": "Pinned artifact contest", "format": "icpc", "beginAt": time.Now().Add(2 * time.Second).Format(time.RFC3339), "endAt": time.Now().Add(time.Hour).Format(time.RFC3339), "visibility": "public", "allowSelfRegistration": true, "allowLateRegistration": true, "feedback": "full"}, &competition, 201)
	apiCall(t, http.MethodPut, prefix+"/admin/contests/"+competition.ID+"/problems", owner, map[string]any{"problemIds": []string{problem.ID}}, nil, 200)
	apiCall(t, http.MethodPost, prefix+"/contests/"+competition.ID+"/register", student, map[string]any{}, nil, 200)
	// Release v2 uses exact comparison and a different answer representation.
	// v1's Kattis checker accepts both 3 and 03; v2 must reject 03.
	save("input", "data/1.in", domain.EntryInput, "1 2\n")
	save("answer", "data/1.ans", domain.EntryAnswer, "3\n")
	metadata.Metadata.Title = "Sum with exact output"
	metadata.Metadata.OutputValidator = ""
	metadata.Metadata.Comparison = domain.OutputComparison{Kind: "exact"}
	doc("problem", "vertex/problem.json", domain.EntryMetadata, metadata.Metadata)
	var secondCommit domain.CommitOutcome
	apiCall(t, http.MethodPost, endpoint+"/commits", owner, domain.CommitInput{ETag: copy.ETag, RequestID: "checked-second", Message: "Require exact answer formatting"}, &secondCommit, 200)
	copy = secondCommit.Copy
	apiCall(t, http.MethodPost, endpoint+"/releases", owner, domain.CommitPublication{Revision: secondCommit.Commit.Revision, CheckID: check.ID, ExpectedVersion: 1}, nil, 409)
	var secondCheck domain.CheckRun
	apiCall(t, http.MethodPost, endpoint+"/checks", owner, domain.CheckSelection{Revision: secondCommit.Commit.Revision}, &secondCheck, 200)
	deadline = time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		apiCall(t, http.MethodGet, endpoint+"/checks/"+secondCheck.ID, owner, nil, &secondCheck, 200)
		if secondCheck.State == "succeeded" || secondCheck.State == "failed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if secondCheck.State != "succeeded" {
		t.Fatalf("second check failed: %+v", secondCheck)
	}
	apiCall(t, http.MethodPost, endpoint+"/releases", owner, domain.CommitPublication{Revision: secondCommit.Commit.Revision, CheckID: secondCheck.ID, ExpectedVersion: 0}, nil, 409)
	apiCall(t, http.MethodPost, endpoint+"/releases", owner, domain.CommitPublication{Revision: secondCommit.Commit.Revision, CheckID: secondCheck.ID, ExpectedVersion: 1}, &released, 200)
	if released.Version != 2 {
		t.Fatalf("second publication: %+v", released)
	}
	runSubmission := func(token, contest, code, want string, version int) {
		t.Helper()
		body := map[string]any{"problemId": problem.ID, "language": "cpp", "sourceCode": code}
		if contest != "" {
			body["contestId"] = contest
		}
		var sub submission
		apiCall(t, http.MethodPost, prefix+"/submissions", token, body, &sub, 202)
		limit := time.Now().Add(45 * time.Second)
		for time.Now().Before(limit) {
			apiCall(t, http.MethodGet, prefix+"/submissions/"+sub.ID, token, nil, &sub, 200)
			if isTerminal(sub.Status) {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if sub.Status != want || sub.ProblemVersion != version {
			t.Fatalf("submission = %+v, want %s version %d", sub, want, version)
		}
		request, _ := http.NewRequest(http.MethodGet, prefix+"/submissions/"+sub.ID, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"private expected sum", "judgemessage.txt", "toolchainKey", "package_manifest"} {
			if strings.Contains(string(raw), secret) {
				t.Fatalf("submission response leaked %s", secret)
			}
		}
	}
	// Contest participants receive the published problem through their contest,
	// not authoring access; their submission must still bind v1 after v2 exists.
	time.Sleep(2 * time.Second)
	runSubmission(student, competition.ID, "#include <iostream>\nint main(){std::cout<<\"03\\n\";}\n", "Accepted", 1)
	runSubmission(owner, "", "#include <iostream>\nint main(){std::cout<<\"03\\n\";}\n", "Wrong Answer", 2)
	runSubmission(student, competition.ID, "#include <iostream>\nint main(){std::cout<<\"0\\n\";}\n", "Wrong Answer", 1)

	// Copy v1 after v2 exists; no current private resource or newer checker
	// policy may enter the destination's independent workbench.
	save("unshared-note", "notes/private.cpp", domain.EntrySource, "// uncommitted copy secret\n")
	targetSpace := newDomain(t, base, owner)
	targetPrefix := scopedPrefix(base, targetSpace)
	apiCall(t, http.MethodPut, targetPrefix+"/members/"+studentName, owner, map[string]any{"roleKey": "member", "status": "active"}, nil, 200)
	copyBody := map[string]any{"sourceDomain": space.Slug, "sourceProblem": problem.ID, "sourceVersion": 1, "attribution": "Native artifact copy regression"}
	apiCall(t, http.MethodPost, targetPrefix+"/authoring/problem-copies", student, copyBody, nil, 403)
	var copied struct {
		ProblemID  string `json:"problemId"`
		DomainSlug string `json:"domainSlug"`
	}
	apiCall(t, http.MethodPost, targetPrefix+"/authoring/problem-copies", owner, copyBody, &copied, 201)
	copiedEndpoint := targetPrefix + "/authoring/problems/" + copied.ProblemID
	var copiedDraft domain.WorkingCopy
	apiCall(t, http.MethodGet, copiedEndpoint+"/working-copy", owner, nil, &copiedDraft, 200)
	if copiedDraft.HeadRevision != nil || copiedDraft.BaseRevision != nil {
		t.Fatal("copy implicitly created a shared revision")
	}
	for _, entry := range copiedDraft.Tree.Entries {
		if entry.ID == "unshared-note" {
			t.Fatal("uncommitted source material crossed into copy")
		}
	}
	var copiedChecks struct {
		Items []domain.CheckRun `json:"items"`
	}
	apiCall(t, http.MethodGet, copiedEndpoint+"/checks", owner, nil, &copiedChecks, 200)
	if len(copiedChecks.Items) != 1 || copiedChecks.Items[0].State != "succeeded" || copiedChecks.Items[0].Stage != "copied" {
		t.Fatalf("copied validation not recorded: %+v", copiedChecks)
	}
	deniedOrigin := authoringRawResponse(t, copiedEndpoint+"/origin", student, 403)
	if strings.Contains(string(deniedOrigin), "Native artifact copy regression") || strings.Contains(string(deniedOrigin), space.Slug) {
		t.Fatal("denied origin response leaked provenance")
	}
	var adopted domain.CommitOutcome
	apiCall(t, http.MethodPost, copiedEndpoint+"/commits", owner, domain.CommitInput{ETag: copiedDraft.ETag, RequestID: "adopt-copy", Message: "Adopt pinned source release"}, &adopted, 200)
	apiCall(t, http.MethodPost, copiedEndpoint+"/releases", owner, domain.CommitPublication{Revision: adopted.Commit.Revision, CheckID: copiedChecks.Items[0].ID, ExpectedVersion: 0}, &released, 200)
	prefix, problem.ID = targetPrefix, copied.ProblemID
	runSubmission(owner, "", "#include <iostream>\nint main(){std::cout<<\"03\\n\";}\n", "Accepted", 1)
}
