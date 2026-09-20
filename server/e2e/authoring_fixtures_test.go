package e2e

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// Only API-only/protocol suites set this. Native suites use the actual Worker.
// A simulated fixture is explicit in its stored check log and has no run matrix.
var fixtureChecksWithoutExecution bool

func saveFixtureDataAt(t *testing.T, endpoint, token string, files map[string]string) {
	t.Helper()
	var copy domain.WorkingCopy
	apiCall(t, http.MethodPost, endpoint+"/working-copy", token, nil, &copy, 200)
	remove := []string{}
	for _, entry := range copy.Tree.Entries {
		if strings.HasPrefix(entry.ID, "fixture-") {
			remove = append(remove, entry.ID)
		}
	}
	if len(remove) > 0 {
		apiCall(t, http.MethodPost, endpoint+"/working-copy/batch", token, domain.MaterialBatch{ETag: copy.ETag, DeleteIDs: remove}, &copy, 200)
	}
	save := func(id, path, kind, text string) {
		t.Helper()
		entry := domain.TreeEntry{ID: id, Path: path, Kind: kind}
		body := map[string]any{"etag": copy.ETag, "entry": entry, "text": text}
		if len(text) > 1<<20 {
			entry.Blob = uploadAuthoringBlob(t, endpoint, token, text)
			body = map[string]any{"etag": copy.ETag, "entry": entry}
		}
		apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/"+id, token, body, &copy, 200)
	}
	doc := func(id, path, kind string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		save(id, path, kind, string(data))
	}
	names := []string{}
	answers := map[string]string{}
	for name := range files {
		if strings.HasSuffix(name, ".in") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for index, name := range names {
		answer, ok := files[strings.TrimSuffix(name, ".in")+".out"]
		if !ok {
			t.Fatalf("fixture answer missing: %s", name)
		}
		answers[base64.StdEncoding.EncodeToString([]byte(files[name]))] = base64.StdEncoding.EncodeToString([]byte(answer))
		inputID, answerID := fmt.Sprintf("fixture-input-%d", index+1), fmt.Sprintf("fixture-answer-%d", index+1)
		save(inputID, fmt.Sprintf("fixtures/%d.in", index+1), domain.EntryInput, files[name])
		save(answerID, fmt.Sprintf("fixtures/%d.ans", index+1), domain.EntryAnswer, answer)
		doc(fmt.Sprintf("fixture-test-%d", index+1), fmt.Sprintf("vertex/tests/fixture-%d.json", index+1), domain.EntryTest, domain.TestMaterial{SchemaVersion: 1, Name: name, Input: domain.TestInput{Kind: "file", Entry: inputID}, Answer: domain.TestAnswer{Kind: "file", Entry: answerID}})
	}
	lookup, _ := json.Marshal(answers)
	source := "import base64,sys\nanswers=" + string(lookup) + "\nkey=base64.b64encode(sys.stdin.buffer.read()).decode()\nsys.stdout.buffer.write(base64.b64decode(answers[key]))\n"
	save("fixture-source", "fixtures/reference/main.py", domain.EntrySource, source)
	doc("fixture-reference", "vertex/programs/fixture-reference.json", domain.EntryProgram, domain.ProgramMaterial{SchemaVersion: 1, Name: "Fixture reference", Role: "solution", Language: "python", Protocol: "stdio", Directory: "fixtures/reference", Files: []string{"fixture-source"}, EntryPoint: "fixture-source", ExpectedVerdicts: []string{"Accepted"}})
	var metadata domain.MaterialView
	apiCall(t, http.MethodGet, endpoint+"/materials/problem", token, nil, &metadata, 200)
	metadata.Metadata.MainSolution = "fixture-reference"
	metadata.Metadata.Comparison = domain.OutputComparison{Kind: "tokens", CaseSensitive: true}
	metadata.Metadata.OutputValidator = ""
	metadata.Metadata.InputValidators = []string{}
	doc("problem", "vertex/problem.json", domain.EntryMetadata, metadata.Metadata)
}

func publishFixtureAt(t *testing.T, endpoint, token string) int {
	t.Helper()
	var copy domain.WorkingCopy
	apiCall(t, http.MethodGet, endpoint+"/working-copy", token, nil, &copy, 200)
	var inspection domain.MaterialInspection
	apiCall(t, http.MethodGet, endpoint+"/inspection?etag="+copy.ETag, token, nil, &inspection, 200)
	var checks struct {
		Items []domain.CheckRun `json:"items"`
	}
	apiCall(t, http.MethodGet, endpoint+"/checks", token, nil, &checks, 200)
	var selected domain.CheckRun
	for _, check := range checks.Items {
		if check.State == "succeeded" && check.DataHash == inspection.DataHash && check.PolicyVersion == inspection.PolicyVersion {
			selected = check
			break
		}
	}
	if selected.ID == "" {
		apiCall(t, http.MethodPost, endpoint+"/checks", token, domain.CheckSelection{ETag: copy.ETag}, &selected, 200)
		if fixtureChecksWithoutExecution {
			base := strings.Split(endpoint, "/api/")[0]
			completeFixtureCheck(t, base, selected.ID)
		}
		deadline := time.Now().Add(2 * time.Minute)
		for time.Now().Before(deadline) {
			apiCall(t, http.MethodGet, endpoint+"/checks/"+selected.ID, token, nil, &selected, 200)
			if selected.State != "queued" && selected.State != "running" {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if selected.State != "succeeded" {
			t.Fatalf("fixture check failed: %+v", selected)
		}
	}
	var commit domain.CommitOutcome
	apiCall(t, http.MethodPost, endpoint+"/commits", token, domain.CommitInput{ETag: copy.ETag, RequestID: fmt.Sprintf("fixture-%d", time.Now().UnixNano()), Message: "Publish reviewed fixture"}, &commit, 200)
	if commit.Copy.HeadRevision == nil {
		t.Fatal("fixture has no committed revision")
	}
	var prior struct {
		PublishedVersion int `json:"publishedVersion"`
	}
	admin := strings.Replace(endpoint, "/authoring/problems/", "/admin/problems/", 1)
	apiCall(t, http.MethodGet, admin, token, nil, &prior, 200)
	var published domain.CommitRelease
	apiCall(t, http.MethodPost, endpoint+"/releases", token, domain.CommitPublication{Revision: *commit.Copy.HeadRevision, CheckID: selected.ID, ExpectedVersion: prior.PublishedVersion}, &published, 200)
	return published.Version
}

func completeFixtureCheck(t *testing.T, base, checkID string) {
	t.Helper()
	serviceToken := os.Getenv("E2E_JUDGE_API_TOKEN")
	if serviceToken == "" {
		t.Fatal("API-only fixture completion requires the test worker credential")
	}
	const worker = "api-only-fixture-builder"
	var job struct {
		BuildID    string                `json:"buildId"`
		ProblemID  string                `json:"problemId"`
		LeaseToken string                `json:"leaseToken"`
		Check      *domain.CheckSnapshot `json:"check"`
	}
	apiCall(t, http.MethodPost, base+"/internal/judge/v1/builds/claim", serviceToken, map[string]any{"workerId": worker, "checkProtocol": domain.CheckPolicyVersion, "waitSeconds": 1}, &job, 200)
	if job.BuildID != checkID || job.Check == nil {
		t.Fatalf("fixture builder claimed another job: %+v", job)
	}
	read := func(ref domain.BlobRef) []byte {
		t.Helper()
		request, _ := http.NewRequest(http.MethodGet, base+"/internal/judge/v1/builds/"+job.BuildID+"/content/"+ref.SHA256, nil)
		request.Header.Set("Authorization", "Bearer "+serviceToken)
		request.Header.Set("X-Vertex-Worker-Id", worker)
		request.Header.Set("X-Vertex-Lease-Token", job.LeaseToken)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 {
			t.Fatalf("read fixture blob: %d %v", response.StatusCode, err)
		}
		return data
	}
	manifest := domain.CheckArtifact{SchemaVersion: 1, ToolchainKey: strings.Repeat("f", 64), Snapshot: *job.Check, Tests: []domain.ArtifactTest{}}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	write := func(name string, data []byte) {
		t.Helper()
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	for index, test := range job.Check.Tests {
		if test.Input == nil || test.Answer == nil {
			t.Fatal("API-only fixtures require pre-materialized files")
		}
		manifest.Tests = append(manifest.Tests, domain.ArtifactTest{ID: test.ID, Input: *test.Input, Answer: *test.Answer})
		write(fmt.Sprintf("%d.in", index+1), read(*test.Input))
		write(fmt.Sprintf("%d.out", index+1), read(*test.Answer))
	}
	for _, program := range job.Check.Programs {
		for _, file := range program.Files {
			write("programs/"+program.ID+"/"+file.Path, read(file.Blob))
		}
	}
	// API-only tests exercise binding/authorization, never claim TeX execution.
	// The native E2E uses a real Worker and does not enter this helper.
	for _, statement := range job.Check.Statements {
		pdf := []byte(publicationPDF())
		manifest.Statements = append(manifest.Statements, domain.ArtifactStatement{ID: statement.ID, PDF: domain.Reference(pdf)})
		write("statements/"+statement.ID+".pdf", pdf)
	}
	data, _ := json.Marshal(manifest)
	write("artifact.json", data)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodPost, base+"/internal/judge/v1/builds/"+job.BuildID+"/package?problemId="+job.ProblemID, &archive)
	request.Header.Set("Authorization", "Bearer "+serviceToken)
	request.Header.Set("X-Vertex-Worker-Id", worker)
	request.Header.Set("X-Vertex-Lease-Token", job.LeaseToken)
	request.Header.Set("Content-Type", "application/zip")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("fixture artifact upload: %d %s", response.StatusCode, body)
	}
	apiCall(t, http.MethodPut, base+"/internal/judge/v1/builds/"+job.BuildID+"/result", serviceToken, map[string]any{"workerId": worker, "leaseToken": job.LeaseToken, "success": true, "toolchainKey": manifest.ToolchainKey, "log": "API-only fixture preparation: no program was executed."}, nil, 204)
}
