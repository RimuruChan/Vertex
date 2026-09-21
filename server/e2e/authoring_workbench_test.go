package e2e

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// This uses actual HTTP middleware, public-number resolution, PostgreSQL and
// blob storage. Assertions inspect returned bytes, not frontend visibility.
func TestEndToEndAuthoringWorkbench(t *testing.T) {
	base := apiBase(t)
	owner, _ := registerUser(t, base)
	editor, editorName := registerUser(t, base)
	reader, readerName := registerUser(t, base)
	outsider, _ := registerUser(t, base)
	space := newDomain(t, base, owner)
	prefix := scopedPrefix(base, space)
	for _, name := range []string{editorName, readerName} {
		apiCall(t, http.MethodPut, prefix+"/members/"+name, owner, map[string]any{"roleKey": "member", "status": "active"}, nil, 200)
	}
	var p numberedResource
	apiCall(t, http.MethodPost, prefix+"/admin/problems", owner, map[string]any{"title": "Versioned authoring", "visibility": "private"}, &p, 201)
	for name, role := range map[string]string{editorName: "editor", readerName: "reader"} {
		apiCall(t, http.MethodPut, prefix+"/admin/problems/"+p.ID+"/access", owner, map[string]string{"username": name, "role": role}, nil, 200)
	}
	endpoint := prefix + "/authoring/problems/" + p.ID
	for _, legacy := range []struct{ method, path string }{{http.MethodPut, "/files"}, {http.MethodPost, "/tests"}, {http.MethodPut, "/statements/zh"}, {http.MethodPost, "/builds"}, {http.MethodPost, "/publish"}, {http.MethodPost, "/testdata"}, {http.MethodPut, ""}} {
		apiCall(t, legacy.method, prefix+"/admin/problems/"+p.ID+legacy.path, owner, map[string]any{}, nil, 404)
	}
	apiCall(t, http.MethodPost, prefix+"/problem-copies", owner, map[string]any{}, nil, 404)
	var copy domain.WorkingCopy
	apiCall(t, http.MethodGet, endpoint+"/working-copy", owner, nil, nil, 404)
	apiCall(t, http.MethodPost, endpoint+"/working-copy", reader, nil, nil, 403)
	apiCall(t, http.MethodPost, endpoint+"/working-copy", owner, nil, &copy, 200)
	var metadata domain.MaterialView
	apiCall(t, http.MethodGet, endpoint+"/materials/problem", owner, nil, &metadata, 200)
	if metadata.Metadata == nil || metadata.Metadata.Title != "Versioned authoring" {
		t.Fatalf("new problem has no editable metadata: %+v", metadata)
	}
	var inspection domain.MaterialInspection
	apiCall(t, http.MethodGet, endpoint+"/inspection?etag="+copy.ETag, owner, nil, &inspection, 200)
	if inspection.CanBuild || len(inspection.Issues) == 0 || inspection.ETag != copy.ETag {
		t.Fatalf("empty materials were marked ready: %+v", inspection)
	}
	apiCall(t, http.MethodPost, endpoint+"/checks", owner, domain.CheckSelection{ETag: copy.ETag}, nil, 400)
	apiCall(t, http.MethodGet, endpoint+"/inspection", reader, nil, nil, 404)
	var changes domain.ContentComparison
	apiCall(t, http.MethodGet, endpoint+"/changes", owner, nil, &changes, 200)
	if len(changes.Changes) != 2 || changes.ETag != copy.ETag {
		t.Fatalf("initial material diff: %+v", changes)
	}
	program := domain.ProgramMaterial{SchemaVersion: 1, Name: "Reference solution", Role: "solution", Language: "cpp", Protocol: "stdio", Files: []string{}, Arguments: []string{}, ExpectedVerdicts: []string{"Accepted"}}
	programBytes, _ := json.Marshal(program)
	programEntry := domain.TreeEntry{ID: "reference", Path: "vertex/programs/reference.json", Kind: domain.EntryProgram, Attributes: map[string]string{"format": "json"}}
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/reference", owner, map[string]any{"etag": copy.ETag, "entry": programEntry, "text": string(programBytes)}, &copy, 200)
	var material domain.MaterialView
	apiCall(t, http.MethodGet, endpoint+"/materials/reference", owner, nil, &material, 200)
	if material.Program == nil || material.Program.Name != "Reference solution" {
		t.Fatal("program document was not round-tripped")
	}
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/reference", owner, map[string]any{"etag": copy.ETag, "entry": programEntry, "text": `{"schemaVersion":2}`}, nil, 400)
	apiCall(t, http.MethodDelete, endpoint+"/working-copy/entries/problem", owner, map[string]string{"etag": copy.ETag}, nil, 400)
	secret := "private draft marker 28f87"
	ref := uploadAuthoringBlob(t, endpoint, owner, secret)
	for i := range copy.Tree.Entries {
		if copy.Tree.Entries[i].ID == "statement-zh" {
			copy.Tree.Entries[i].Blob = ref
		}
	}
	apiCall(t, http.MethodPut, endpoint+"/working-copy", owner, map[string]any{"etag": copy.ETag, "tree": copy.Tree}, &copy, 200)
	savedToken := copy.ETag
	apiCall(t, http.MethodPut, endpoint+"/working-copy", owner, map[string]any{"etag": copy.ETag, "tree": copy.Tree}, &copy, 200)
	if copy.ETag != savedToken {
		t.Fatal("saving identical content changed the working-copy token")
	}
	var history struct {
		Items []domain.ContentCommit `json:"items"`
	}
	apiCall(t, http.MethodGet, endpoint+"/commits", reader, nil, &history, 200)
	if len(history.Items) != 0 {
		t.Fatal("save created a history revision")
	}
	for _, token := range []string{reader, editor} {
		body := authoringRawResponse(t, endpoint+"/blobs/"+ref.SHA256, token, 404)
		if strings.Contains(string(body), secret) || strings.Contains(string(body), ref.SHA256) {
			t.Fatal("private blob information in denied response")
		}
		apiCall(t, http.MethodGet, endpoint+"/working-copy", token, nil, nil, 404)
	}
	authoringRawResponse(t, endpoint+"/commits", outsider, 404)
	apiCall(t, http.MethodPut, endpoint+"/working-copy", owner, map[string]any{"etag": "stale-token", "tree": copy.Tree}, nil, 409)
	var result domain.CommitOutcome
	apiCall(t, http.MethodPost, endpoint+"/commits", owner, domain.CommitInput{ETag: copy.ETag, RequestID: "first", Message: "Reviewed initial statement"}, &result, 200)
	if result.Commit == nil || result.Commit.Revision != 1 {
		t.Fatalf("missing explicit commit: %+v", result)
	}
	apiCall(t, http.MethodGet, endpoint+"/changes", owner, nil, &changes, 200)
	if len(changes.Changes) != 0 {
		t.Fatal("committed copy still has pending changes")
	}
	apiCall(t, http.MethodGet, endpoint+"/materials/reference?revision=1", reader, nil, &material, 200)
	if material.Program == nil {
		t.Fatal("reader cannot inspect committed program material")
	}
	if got := string(authoringRawResponse(t, endpoint+"/blobs/"+ref.SHA256, reader, 200)); got != secret {
		t.Fatalf("committed material not reviewable: %q", got)
	}
	// Being committed does not make authoring files readable to unrelated users.
	authoringRawResponse(t, endpoint+"/blobs/"+ref.SHA256, outsider, 404)
	apiCall(t, http.MethodPost, endpoint+"/working-copy", editor, nil, &copy, 200)
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/statement-zh/text", editor, map[string]string{"etag": copy.ETag, "text": "editor secret draft\r\n"}, &copy, 200)
	if body := authoringRawResponse(t, endpoint+"/commits/1", reader, 200); strings.Contains(string(body), "editor secret draft") {
		t.Fatal("historical response leaked the working draft")
	}
	apiCall(t, http.MethodGet, prefix+"/problems/"+p.ID, outsider, nil, nil, 404)
	var releases struct {
		Items []json.RawMessage `json:"items"`
	}
	apiCall(t, http.MethodGet, endpoint+"/releases", owner, nil, &releases, 200)
	if len(releases.Items) != 0 {
		t.Fatal("working copy commit published the problem")
	}
	// Another domain's number space must not resolve this problem, even with
	// a valid owner token and a known content digest.
	other := newDomain(t, base, owner)
	authoringRawResponse(t, scopedPrefix(base, other)+"/authoring/problems/"+p.ID+"/blobs/"+ref.SHA256, owner, 404)

	// Data import is staged privately and merges into the existing draft. Test
	// the raw previews and exported archive, not only UI visibility.
	apiCall(t, http.MethodGet, endpoint+"/working-copy", owner, nil, &copy, 200)
	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	for name, data := range map[string]string{"case1.in": "private input 8ab21\n", "case1.out": "private answer 8ab21\n"} {
		part, err := zipWriter.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	var upload bytes.Buffer
	form := multipart.NewWriter(&upload)
	if err := form.WriteField("etag", copy.ETag); err != nil {
		t.Fatal(err)
	}
	part, err := form.CreateFormFile("file", "data.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(archive.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint+"/imports", &upload)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+owner)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	previewBytes, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("preview failed: %d %s", response.StatusCode, previewBytes)
	}
	var preview domain.ImportReceipt
	if err := json.Unmarshal(previewBytes, &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Plan.Scope != "data" || !preview.Plan.CanApply {
		t.Fatalf("wrong preview: %+v", preview)
	}
	var unchanged domain.WorkingCopy
	apiCall(t, http.MethodGet, endpoint+"/working-copy", owner, nil, &unchanged, 200)
	if unchanged.ETag != copy.ETag {
		t.Fatal("preview changed current copy")
	}
	for _, actor := range []string{editor, reader} {
		body := authoringRawResponse(t, endpoint+"/imports/"+preview.ID, actor, 404)
		if bytes.Contains(body, []byte("case1")) || bytes.Contains(body, []byte(preview.Plan.ArchiveHash)) {
			t.Fatal("private import response leaked metadata")
		}
		for _, entry := range preview.Plan.Tree.Entries {
			if entry.Path == "case1.out" {
				authoringRawResponse(t, endpoint+"/blobs/"+entry.Blob.SHA256, actor, 404)
			}
		}
	}
	authoringRawResponse(t, scopedPrefix(base, other)+"/authoring/problems/"+p.ID+"/imports/"+preview.ID, owner, 404)
	apiCall(t, http.MethodPost, endpoint+"/imports/"+preview.ID+"/apply", owner, map[string]string{"etag": copy.ETag}, &copy, 200)
	apiCall(t, http.MethodGet, endpoint+"/materials/problem", owner, nil, &metadata, 200)
	if metadata.Metadata.Title != "Versioned authoring" || len(metadata.Metadata.TestOrder) != 1 {
		t.Fatal("data import replaced problem metadata")
	}
	var exported domain.PackageExport
	apiCall(t, http.MethodPost, endpoint+"/exports", owner, map[string]string{"format": "vertex"}, &exported, 200)
	resultBytes := authoringRawResponse(t, endpoint+"/blobs/"+exported.File.SHA256, owner, 200)
	if _, err := zip.NewReader(bytes.NewReader(resultBytes), int64(len(resultBytes))); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []string{editor, reader} {
		authoringRawResponse(t, endpoint+"/blobs/"+exported.File.SHA256, actor, 404)
	}

	var page domain.MaterialPage
	apiCall(t, http.MethodGet, endpoint+"/materials?kind=test&limit=1&etag="+copy.ETag, owner, nil, &page, 200)
	if page.Total != 1 || len(page.Items) != 1 || page.ETag != copy.ETag || page.Items[0].Test == nil {
		t.Fatalf("material catalog: %+v", page)
	}
	testID := page.Items[0].Entry.ID
	for _, actor := range []string{editor, reader} {
		expected := 404
		if actor == editor {
			expected = 200
		}
		denied := authoringRawResponse(t, endpoint+"/materials?kind=test", actor, expected)
		if bytes.Contains(denied, []byte(testID)) {
			t.Fatal("private catalog leaked entry ID")
		}
	}
	apiCall(t, http.MethodGet, endpoint+"/materials?kind=program&revision=1", reader, nil, &page, 200)
	if len(page.Items) != 1 || page.Items[0].Program == nil {
		t.Fatal("reader cannot review committed program catalog")
	}
	apiCall(t, http.MethodGet, endpoint+"/materials?kind=test&limit=0", owner, nil, nil, 400)
	apiCall(t, http.MethodGet, endpoint+"/materials?kind=test&etag=stale", owner, nil, nil, 409)
	second := domain.TestMaterial{SchemaVersion: 1, Name: "Second", Input: domain.TestInput{Kind: "file", Entry: ""}, Answer: domain.TestAnswer{Kind: "file", Entry: ""}}
	secondBytes, _ := json.Marshal(second)
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/second-test", owner, map[string]any{"etag": copy.ETag, "entry": domain.TreeEntry{ID: "second-test", Path: "vertex/tests/second.json", Kind: domain.EntryTest}, "text": string(secondBytes)}, &copy, 200)
	apiCall(t, http.MethodGet, endpoint+"/materials?kind=test&limit=1&etag="+copy.ETag, owner, nil, &page, 200)
	if page.Total != 2 || page.Next != testID {
		t.Fatalf("first page order: %+v", page)
	}
	cursor := page.Next
	page = domain.MaterialPage{}
	apiCall(t, http.MethodGet, endpoint+"/materials?kind=test&limit=1&after="+cursor+"&etag="+copy.ETag, owner, nil, &page, 200)
	if len(page.Items) != 1 || page.Items[0].Entry.ID != "second-test" || page.Next != "" {
		t.Fatalf("second page order: %+v", page)
	}
	sample := true
	points := 12.5
	timeLimit, memoryLimit := 1500, 131072
	batch := domain.MaterialBatch{ETag: copy.ETag, TestIDs: []string{testID, "missing-test"}, Patch: &domain.TestPatch{IsSample: &sample, Points: &points, TimeLimitMs: &timeLimit, MemoryLimitKB: &memoryLimit}}
	apiCall(t, http.MethodPost, endpoint+"/working-copy/batch", owner, batch, nil, 400)
	apiCall(t, http.MethodGet, endpoint+"/working-copy", owner, nil, &unchanged, 200)
	if unchanged.ETag != copy.ETag {
		t.Fatal("failed batch partially changed the copy")
	}
	batch.TestIDs = []string{testID, "second-test"}
	order := []string{"second-test", testID}
	batch.TestOrder = &order
	apiCall(t, http.MethodPost, endpoint+"/working-copy/batch", reader, batch, nil, 404)
	apiCall(t, http.MethodPost, endpoint+"/working-copy/batch", owner, batch, &copy, 200)
	apiCall(t, http.MethodGet, endpoint+"/materials?kind=test&limit=2&etag="+copy.ETag, owner, nil, &page, 200)
	if len(page.Items) != 2 || page.Items[0].Entry.ID != "second-test" || !page.Items[1].Test.IsSample || page.Items[1].Test.Points != 12.5 {
		t.Fatalf("batch changes missing: %+v", page)
	}
	for _, item := range page.Items {
		if item.Test.TimeLimitMs != timeLimit || item.Test.MemoryLimitKB != memoryLimit {
			t.Fatal("batch limits not applied atomically")
		}
	}
	apiCall(t, http.MethodPost, endpoint+"/working-copy/batch", owner, batch, nil, 409)
	apiCall(t, http.MethodPost, endpoint+"/working-copy/batch", owner, domain.MaterialBatch{ETag: copy.ETag, DeleteIDs: []string{testID, "second-test"}}, &copy, 200)
	apiCall(t, http.MethodGet, endpoint+"/materials/problem", owner, nil, &metadata, 200)
	if len(metadata.Metadata.TestOrder) != 0 {
		t.Fatal("batch delete left deleted tests in ordering")
	}
	binary := "\x00\xff\x01\x02binary fixture"
	binaryRef := uploadAuthoringBlob(t, endpoint, owner, binary)
	binaryEntry := domain.TreeEntry{ID: "binary-asset", Path: "attachments/original.bin", Kind: domain.EntryAsset, Blob: binaryRef}
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/binary-asset", owner, map[string]any{"etag": copy.ETag, "entry": binaryEntry}, &copy, 200)
	binaryEntry.Path = "attachments/renamed.bin"
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/binary-asset", owner, map[string]any{"etag": copy.ETag, "entry": binaryEntry}, &copy, 200)
	for _, entry := range copy.Tree.Entries {
		if entry.ID == binaryEntry.ID && (entry.Blob != binaryRef || entry.Path != binaryEntry.Path) {
			t.Fatal("binary rename rewrote file bytes")
		}
	}
	if string(authoringRawResponse(t, endpoint+"/blobs/"+binaryRef.SHA256, owner, 200)) != binary {
		t.Fatal("binary bytes changed after rename")
	}
	apiCall(t, http.MethodGet, endpoint+"/materials/problem", owner, nil, &metadata, 200)
	metadata.Metadata.Title = "Private draft title 83ef"
	privateMeta, _ := json.Marshal(metadata.Metadata)
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/problem/text", owner, map[string]any{"etag": copy.ETag, "text": string(privateMeta)}, &copy, 200)
	var library domain.LibraryPage
	libraryURL := prefix + "/authoring/problems"
	apiCall(t, http.MethodGet, libraryURL+"?keyword=83ef&status=changes", owner, nil, &library, 200)
	if len(library.Items) != 1 || library.Items[0].ID != p.ID || library.Items[0].Title != metadata.Metadata.Title || !library.Items[0].HasChanges || !library.Items[0].CanEdit || library.Items[0].HeadRevision != 1 {
		t.Fatalf("authoring summary does not reflect private draft: %+v", library)
	}
	for _, actor := range []string{editor, reader} {
		apiCall(t, http.MethodGet, libraryURL+"?keyword=83ef", actor, nil, &library, 200)
		if len(library.Items) != 0 || library.Total != 0 {
			t.Fatal("private title was searchable by a collaborator")
		}
		body := authoringRawResponse(t, libraryURL, actor, 200)
		if bytes.Contains(body, []byte("Private draft title")) || bytes.Contains(body, []byte(binaryRef.SHA256)) || bytes.Contains(body, []byte(copy.ETag)) {
			t.Fatal("list leaked private draft material")
		}
	}
	apiCall(t, http.MethodGet, libraryURL+"?keyword=83ef&page=2&size=1", owner, nil, &library, 200)
	if len(library.Items) != 0 || library.Total != 1 {
		t.Fatalf("empty page lost its total: %+v", library)
	}
	apiCall(t, http.MethodGet, scopedPrefix(base, other)+"/authoring/problems?keyword=83ef", owner, nil, &library, 200)
	if len(library.Items) != 0 {
		t.Fatal("authoring list crossed domain boundary")
	}
	policy := domain.VisibilityChange{Visibility: "public", ExpectedVisibility: "private"}
	apiCall(t, http.MethodPut, endpoint+"/visibility", editor, policy, nil, 403)
	apiCall(t, http.MethodPut, endpoint+"/visibility", owner, policy, nil, 200)
	apiCall(t, http.MethodPut, endpoint+"/visibility", owner, domain.VisibilityChange{Visibility: "private", ExpectedVisibility: "private"}, nil, 409)
	apiCall(t, http.MethodPut, endpoint+"/visibility", owner, domain.VisibilityChange{Visibility: "private", ExpectedVisibility: "public"}, nil, 200)
	apiCall(t, http.MethodGet, endpoint+"/working-copy", owner, nil, &unchanged, 200)
	if unchanged.ETag != copy.ETag {
		t.Fatal("resource visibility changed the personal content tree")
	}
}

func uploadAuthoringBlob(t *testing.T, endpoint, token, content string) domain.BlobRef {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "material.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint+"/blobs", &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("upload: %d %s", response.StatusCode, data)
	}
	var ref domain.BlobRef
	if err := json.NewDecoder(response.Body).Decode(&ref); err != nil {
		t.Fatal(err)
	}
	return ref
}

func authoringRawResponse(t *testing.T, url, token string, status int) []byte {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("read %s: %d want %d: %s", url, response.StatusCode, status, body)
	}
	if status == 200 && response.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatal("private material response was cacheable")
	}
	return body
}
