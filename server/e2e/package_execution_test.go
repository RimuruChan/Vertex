package e2e

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// The archive is a real standard export independently verified with
// problemtools. Requires a real Worker; API-only fixture completion is forbidden.
func TestEndToEndStandardPackageExecution(t *testing.T) {
	filename := os.Getenv("VERTEX_AUTHORING_PACKAGE")
	if filename == "" {
		t.Skip("explicit standard package fixture required")
	}
	if fixtureChecksWithoutExecution {
		t.Fatal("requires real package execution")
	}
	archive, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	base := apiBase(t)
	owner, name := registerUser(t, base)
	space := newDomain(t, base, owner)
	prefix := scopedPrefix(base, space)
	var problem numberedResource
	apiCall(t, http.MethodPost, prefix+"/admin/problems", owner, map[string]any{"title": "Standard import execution", "statementMd": "Before import", "visibility": "private"}, &problem, 201)
	endpoint := prefix + "/authoring/problems/" + problem.ID
	var copy domain.WorkingCopy
	apiCall(t, http.MethodPost, endpoint+"/working-copy", owner, nil, &copy, 200)
	var upload bytes.Buffer
	form := multipart.NewWriter(&upload)
	if err := form.WriteField("etag", copy.ETag); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("timeLimitMs", "1500"); err != nil {
		t.Fatal(err)
	}
	part, err := form.CreateFormFile("file", "standard.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(archive); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint+"/imports", &upload)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+owner)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("import preview: %d %s %v", response.StatusCode, data, err)
	}
	var receipt domain.ImportReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.Plan.CanApply {
		t.Fatalf("unusable imported package: %+v", receipt.Plan.Issues)
	}
	apiCall(t, http.MethodPost, endpoint+"/imports/"+receipt.ID+"/apply", owner, map[string]string{"etag": copy.ETag}, &copy, 200)
	if copy.HeadRevision != nil {
		t.Fatal("import automatically committed")
	}
	if version := publishFixtureAt(t, endpoint, owner); version != 1 {
		t.Fatal("unexpected first publication")
	}
	pdf := authoringRawResponse(t, prefix+"/problems/"+problem.ID+"/files/statement?version=1", owner, 200)
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("imported TeX was not rendered")
	}
	var checks struct {
		Items []domain.CheckRun `json:"items"`
	}
	apiCall(t, http.MethodGet, endpoint+"/checks", owner, nil, &checks, 200)
	if len(checks.Items) != 1 {
		t.Fatalf("unexpected check count %d", len(checks.Items))
	}
	var check domain.CheckRun
	apiCall(t, http.MethodGet, endpoint+"/checks/"+checks.Items[0].ID, owner, nil, &check, 200)
	if check.State != "succeeded" || len(check.Solutions) != 2 || !check.Solutions[0].Matched || !check.Solutions[1].Matched {
		t.Fatalf("original reference verdicts not checked: %+v", check)
	}
	var exported domain.PackageExport
	apiCall(t, http.MethodPost, endpoint+"/exports", owner, map[string]string{"format": "kattis-legacy"}, &exported, 200)
	result := authoringRawResponse(t, endpoint+"/blobs/"+exported.File.SHA256, owner, 200)
	reader, err := zip.NewReader(bytes.NewReader(result), int64(len(result)))
	if err != nil {
		t.Fatal(err)
	}
	wrappers := 0
	root := strings.TrimSuffix(exported.Filename, ".zip") + "/"
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, root) {
			t.Fatalf("download filename and archive root disagree: %s / %s", exported.Filename, file.Name)
		}
		if strings.HasSuffix(file.Name, "/run") {
			wrappers++
		}
	}
	if wrappers != 2 {
		t.Fatal("re-export lost validator adapters")
	}
	t.Logf("Package execution QA: domain=%s problem=%s owner=%s", space.Slug, problem.ID, name)
}
