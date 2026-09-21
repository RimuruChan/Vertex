package e2e

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEndToEndAuthoringProductWorkflows(t *testing.T) {
	base := apiBase(t)
	owner, _ := registerUser(t, base)
	reader, readerName := registerUser(t, base)
	space := newDomain(t, base, owner)
	prefix := scopedPrefix(base, space)
	apiCall(t, http.MethodPut, prefix+"/members/"+readerName, owner, map[string]any{"roleKey": "member", "status": "active"}, nil, 200)
	var problem numberedResource
	apiCall(t, http.MethodPost, prefix+"/admin/problems", owner, map[string]any{"title": "Generation workflow verification", "visibility": "private"}, &problem, 201)
	apiCall(t, http.MethodPut, prefix+"/admin/problems/"+problem.ID+"/access", owner, map[string]string{"username": readerName, "role": "reader"}, nil, 200)
	endpoint := prefix + "/authoring/problems/" + problem.ID
	var copy domain.WorkingCopy
	apiCall(t, http.MethodPost, endpoint+"/working-copy", owner, nil, &copy, 200)
	program := func(id, role string, files map[string]string) {
		t.Helper()
		definition := domain.ProgramMaterial{SchemaVersion: 1, Name: id, Role: role, Language: "python", Protocol: "stdio", Files: []string{}, Arguments: []string{}, ExpectedVerdicts: []string{}}
		if role == "solution" {
			definition.ExpectedVerdicts = []string{"Accepted"}
		}
		sources := []domain.ProgramSourceEdit{}
		for name, content := range files {
			sourceID := id + "-" + strings.ReplaceAll(name, ".", "-")
			definition.Files = append(definition.Files, sourceID)
			if name == "main.py" {
				definition.EntryPoint = sourceID
			}
			sources = append(sources, domain.ProgramSourceEdit{ID: sourceID, Name: name, RelativeName: name, Blob: uploadAuthoringBlob(t, endpoint, owner, content)})
		}
		input := domain.ProgramSaveInput{ETag: copy.ETag, ID: id, Program: definition, Sources: sources}
		var saved domain.ProgramSaveResult
		apiCall(t, http.MethodPut, endpoint+"/programs", reader, input, nil, 403)
		apiCall(t, http.MethodPut, endpoint+"/programs", owner, input, &saved, 200)
		copy = saved.Copy
		if len(saved.Sources) != len(files) || saved.Program.EntryPoint == "" {
			t.Fatal("program aggregate missing sources")
		}
	}
	program("reference", "solution", map[string]string{"main.py": "import sys\nfrom add import add\na,b=map(int,sys.stdin.read().split())\nprint(add(a,b))\n", "add.py": "def add(a,b):\n    return a+b\n"})
	program("generator", "generator", map[string]string{"main.py": "import sys\nseed,index=map(int,sys.argv[1:])\nprint(seed,index)\n"})
	save := func(id, kind, path string, value any, attributes map[string]string) {
		t.Helper()
		text, ok := value.(string)
		if !ok {
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			text = string(data)
		}
		apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/"+id, owner, map[string]any{"etag": copy.ETag, "entry": map[string]any{"id": id, "kind": kind, "path": path, "attributes": attributes}, "text": text}, &copy, 200)
	}
	for _, item := range []struct {
		id, input, answer string
		sample            bool
	}{{"sample", "1 2\n", "3\n", true}, {"secret", "987654321 123456789\n", "1111111110\n", false}} {
		save(item.id+"-input", "input", "data/"+item.id+".in", item.input, map[string]string{})
		save(item.id+"-answer", "answer", "data/"+item.id+".ans", item.answer, map[string]string{})
		save(item.id, "test", "vertex/tests/"+item.id+".json", domain.TestMaterial{SchemaVersion: 1, Name: item.id, IsSample: item.sample, Input: domain.TestInput{Kind: "file", Entry: item.id + "-input", Arguments: []string{}}, Answer: domain.TestAnswer{Kind: "file", Entry: item.id + "-answer"}}, map[string]string{"format": "json"})
	}
	var metadata domain.MaterialView
	apiCall(t, http.MethodGet, endpoint+"/materials/problem", owner, nil, &metadata, 200)
	metadata.Metadata.MainSolution = "reference"
	save("problem", "metadata", "vertex/problem.json", metadata.Metadata, map[string]string{"format": "json"})
	var preview domain.DraftStatementPreview
	request := domain.StatementPreviewInput{ETag: copy.ETag, EntryID: "statement-zh", Content: domain.StandardStatement("Sum", "zh")}
	apiCall(t, http.MethodPost, endpoint+"/statement-preview", reader, request, nil, 404)
	apiCall(t, http.MethodPost, endpoint+"/statement-preview", owner, request, &preview, 200)
	raw, _ := json.Marshal(preview)
	if !strings.Contains(preview.Content, "1 2") || strings.Contains(string(raw), "987654321") || strings.Contains(string(raw), "1111111110") {
		t.Fatalf("preview leaked or omitted samples: %s", raw)
	}
	input := domain.GenerationInput{ETag: copy.ETag, ID: "coverage", Preview: true, Plan: domain.GenerationPlan{SchemaVersion: 1, Name: "Seed coverage", Generator: "generator", Solution: "reference", Rules: []domain.GenerationRule{{ID: "small", Name: "Small", Count: 3, SeedStart: 10, Parameters: "{seed} {index}"}}}}
	var expanded domain.GenerationResult
	apiCall(t, http.MethodPost, endpoint+"/generation", reader, input, nil, 403)
	apiCall(t, http.MethodPost, endpoint+"/generation", owner, input, &expanded, 200)
	var untouched domain.WorkingCopy
	apiCall(t, http.MethodGet, endpoint+"/working-copy", owner, nil, &untouched, 200)
	if untouched.ETag != copy.ETag || expanded.Copy != nil || len(expanded.Cases) != 3 {
		t.Fatal("preview changed working copy")
	}
	input.Preview = false
	apiCall(t, http.MethodPost, endpoint+"/generation", owner, input, &expanded, 200)
	copy = *expanded.Copy
	apiCall(t, http.MethodPost, endpoint+"/generation", owner, input, nil, 409)
	var review domain.ContentComparison
	apiCall(t, http.MethodGet, endpoint+"/changes", owner, nil, &review, 200)
	found := false
	for _, item := range review.Review {
		if item.Kind == domain.EntryProgram && item.Label == "reference" && len(item.EntryIDs) >= 3 {
			found = true
		}
	}
	if !found {
		t.Fatalf("source changes were not grouped by program: %+v", review.Review)
	}
	var committed domain.CommitOutcome
	apiCall(t, http.MethodPost, endpoint+"/commits", owner, domain.CommitInput{ETag: copy.ETag, RequestID: "workflow-reviewed", Message: "Review typed program and generation plan"}, &committed, 200)
	copy = committed.Copy
	request.Revision = committed.Commit.Revision
	request.ETag = ""
	apiCall(t, http.MethodPost, endpoint+"/statement-preview", reader, request, &preview, 200)
	if os.Getenv("VERTEX_FROZEN_CHECK_E2E") != "1" {
		t.Log("API verified; real Worker execution not requested")
		return
	}
	var check domain.CheckRun
	apiCall(t, http.MethodPost, endpoint+"/checks", owner, domain.CheckSelection{Revision: committed.Commit.Revision}, &check, 200)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		apiCall(t, http.MethodGet, endpoint+"/checks/"+check.ID, owner, nil, &check, 200)
		if check.State == "succeeded" || check.State == "failed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if check.State != "succeeded" || check.PackageCases != 5 {
		t.Fatalf("real generation failed: %+v", check)
	}
	var exported domain.PackageExport
	apiCall(t, http.MethodPost, endpoint+"/exports", owner, map[string]any{"revision": committed.Commit.Revision, "format": "luogu-data"}, &exported, 200)
	archive := authoringRawResponse(t, endpoint+"/blobs/"+exported.File.SHA256, owner, 200)
	zipped, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string]string{}
	for _, file := range zipped.File {
		if file.FileInfo().IsDir() {
			continue
		}
		body, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(io.LimitReader(body, 4096))
		body.Close()
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(file.Name, "/")
		contents[parts[len(parts)-1]] = string(data)
	}
	for index, seed := range []int{10, 11, 12} {
		number := index + 3
		if contents[fmt.Sprintf("%04d.in", number)] != fmt.Sprintf("%d %d\n", seed, index+1) || contents[fmt.Sprintf("%04d.out", number)] != fmt.Sprintf("%d\n", seed+index+1) {
			t.Fatalf("wrong generated artifact at %d: %+v", number, contents)
		}
	}
	t.Logf("real Worker generated 3 tests plus 2 manual tests; domain=%s problem=%s", space, problem.ID)
}
