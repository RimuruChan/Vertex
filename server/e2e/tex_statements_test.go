package e2e

import (
	"bytes"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func TestEndToEndTeXStatements(t *testing.T) {
	base := apiBase(t)
	owner, ownerName := registerUser(t, base)
	editor, editorName := registerUser(t, base)
	space := newDomain(t, base, owner)
	prefix := scopedPrefix(base, space)
	apiCall(t, http.MethodPut, prefix+"/members/"+editorName, owner, map[string]any{"roleKey": "member", "status": "active"}, nil, 200)
	var p numberedResource
	apiCall(t, http.MethodPost, prefix+"/admin/problems", owner, map[string]any{"title": "TeX 排版验收", "statementMd": "Initial wording", "visibility": "private"}, &p, 201)
	endpoint := prefix + "/authoring/problems/" + p.ID
	apiCall(t, http.MethodPut, prefix+"/admin/problems/"+p.ID+"/access", owner, map[string]string{"username": editorName, "role": "editor"}, nil, 200)
	saveFixtureDataAt(t, endpoint, owner, map[string]string{"1.in": "1 2\n", "1.out": "3\n", "2.in": "SECRET-INPUT-99999", "2.out": "SECRET-ANSWER-99999"})
	var copy domain.WorkingCopy
	apiCall(t, http.MethodGet, endpoint+"/working-copy", owner, nil, &copy, 200)
	isSample := true
	apiCall(t, http.MethodPost, endpoint+"/working-copy/batch", owner, domain.MaterialBatch{ETag: copy.ETag, TestIDs: []string{"fixture-test-1"}, Patch: &domain.TestPatch{IsSample: &isSample}}, &copy, 200)
	var statement domain.TreeEntry
	for _, entry := range copy.Tree.Entries {
		if entry.Kind == domain.EntryStatement {
			statement = entry
			break
		}
	}
	statement.Path = "statement/problem.zh.tex"
	statement.Attributes["format"] = "tex"
	source := `\problemname
\section*{题目描述}给定整数 $a$ 和 $b$，输出 $a+b$。
\begin{Input}两个整数。\end{Input}
\begin{Output}输出和。\end{Output}
Before sample.
\nextsample
After sample.
`
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/"+statement.ID, owner, map[string]any{"etag": copy.ETag, "entry": statement, "text": source}, &copy, 200)
	var check domain.CheckRun
	apiCall(t, http.MethodPost, endpoint+"/checks", owner, domain.CheckSelection{ETag: copy.ETag}, &check, 200)
	if fixtureChecksWithoutExecution {
		completeFixtureCheck(t, base, check.ID)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		apiCall(t, http.MethodGet, endpoint+"/checks/"+check.ID, owner, nil, &check, 200)
		if check.State != "queued" && check.State != "running" {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if check.State != "succeeded" || len(check.Statements) != 1 || check.Statements[0].ID != statement.ID {
		t.Fatalf("statement check failed: %+v", check)
	}
	preview := endpoint + "/checks/" + check.ID + "/statements/" + statement.ID
	pdf := authoringRawResponse(t, preview, owner, 200)
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("check preview is not a PDF")
	}
	authoringRawResponse(t, preview, editor, 404)
	authoringRawResponse(t, endpoint+"/checks/"+check.ID+"/statements/missing", owner, 404)
	var commit domain.CommitOutcome
	apiCall(t, http.MethodPost, endpoint+"/commits", owner, domain.CommitInput{ETag: copy.ETag, Message: "Review rendered TeX", RequestID: "tex-first"}, &commit, 200)
	if got := authoringRawResponse(t, preview, editor, 200); !bytes.Equal(got, pdf) {
		t.Fatal("shared checked PDF changed")
	}
	var published domain.CommitRelease
	apiCall(t, http.MethodPost, endpoint+"/releases", owner, domain.CommitPublication{Revision: commit.Commit.Revision, CheckID: check.ID, ExpectedVersion: 0}, &published, 200)
	var presentation struct {
		Files []struct {
			Purpose  string `json:"purpose"`
			Embedded bool   `json:"embedded"`
		} `json:"files"`
	}
	apiCall(t, http.MethodGet, prefix+"/problems/"+p.ID, owner, nil, &presentation, 200)
	embedded := 0
	for _, file := range presentation.Files {
		if file.Purpose == "sample-input" || file.Purpose == "sample-answer" {
			if !file.Embedded {
				t.Fatal("rendered PDF samples repeated in external preview")
			}
			embedded++
		}
	}
	if embedded != 2 {
		t.Fatalf("public samples missing: %+v", presentation)
	}
	file := prefix + "/problems/" + p.ID + "/files/statement?version=1"
	if got := authoringRawResponse(t, file, owner, 200); !bytes.Equal(got, pdf) {
		t.Fatal("release PDF differs from reviewed check")
	}
	// A new TeX commit cannot publish using the earlier compiled artifact.
	copy = commit.Copy
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/"+statement.ID, owner, map[string]any{"etag": copy.ETag, "entry": statement, "text": source + "\nChanged wording."}, &copy, 200)
	apiCall(t, http.MethodPost, endpoint+"/commits", owner, domain.CommitInput{ETag: copy.ETag, Message: "Change TeX wording", RequestID: "tex-second"}, &commit, 200)
	apiCall(t, http.MethodPost, endpoint+"/releases", owner, domain.CommitPublication{Revision: commit.Commit.Revision, CheckID: check.ID, ExpectedVersion: 1}, nil, 409)
	if got := authoringRawResponse(t, file, owner, 200); !bytes.Equal(got, pdf) {
		t.Fatal("failed publication changed live statement")
	}
	if !fixtureChecksWithoutExecution {
		t.Logf("TeX QA: domain=%s problem=%s owner=%s check=%s", space.Slug, p.ID, ownerName, check.ID)
		if target := os.Getenv("E2E_TEX_PDF"); target != "" {
			if err := os.WriteFile(target, pdf, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
