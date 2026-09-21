package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	problemDTO "github.com/RimuruChan/Vertex/server/internal/modules/problem/transport/http/dto"
)

func TestEndToEndPublishedFiles(t *testing.T) {
	base := apiBase(t)
	owner, ownerName := registerUser(t, base)
	viewer, viewerName := registerUser(t, base)
	space := newDomain(t, base, owner)
	prefix := scopedPrefix(base, space)
	apiCall(t, http.MethodPut, prefix+"/members/"+viewerName, owner, map[string]any{"roleKey": "member", "status": "active"}, nil, 200)
	var p numberedResource
	wording := "Public wording\n\n![Publication diagram](../attachments/diagram.png)\n\n{{nextsample}}\n\nBetween examples\n\n{{remainingsamples}}\n\n[Read the attachment](../attachments/guide.txt)"
	apiCall(t, http.MethodPost, prefix+"/admin/problems", owner, map[string]any{"title": "Published files", "statementMd": wording, "visibility": "private"}, &p, 201)
	endpoint := prefix + "/authoring/problems/" + p.ID
	large := strings.Repeat("0123456789", 110000)
	binary := "\x00binary sample\n"
	saveFixtureDataAt(t, endpoint, owner, map[string]string{"1.in": large, "1.out": "large answer\n", "2.in": "PRIVATE-SECRET-CASE", "2.out": "private answer", "3.in": binary, "3.out": "binary answer\n"})
	var copy domain.WorkingCopy
	apiCall(t, http.MethodGet, endpoint+"/working-copy", owner, nil, &copy, 200)
	for _, id := range []string{"fixture-test-1", "fixture-test-3"} {
		var item domain.MaterialView
		apiCall(t, http.MethodGet, endpoint+"/materials/"+id, owner, nil, &item, 200)
		item.Test.IsSample = true
		encoded, _ := json.Marshal(item.Test)
		apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/"+id, owner, map[string]any{"etag": copy.ETag, "entry": item.Entry, "text": string(encoded)}, &copy, 200)
	}
	diagram := publicationPNG(t)
	for _, item := range []struct{ id, path, kind, value string }{{"guide", "attachments/guide.txt", domain.EntryAsset, "public attachment"}, {"diagram", "attachments/diagram.png", domain.EntryAsset, string(diagram)}, {"private-notes", "resources/notes.txt", domain.EntryResource, "NEVER-PUBLIC-REFERENCE"}} {
		ref := uploadAuthoringBlob(t, endpoint, owner, item.value)
		apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/"+item.id, owner, map[string]any{"etag": copy.ETag, "entry": domain.TreeEntry{ID: item.id, Path: item.path, Kind: item.kind, Blob: ref}}, &copy, 200)
	}
	if version := publishFixtureAt(t, endpoint, owner); version != 1 {
		t.Fatal("unexpected initial release")
	}
	var view problemDTO.ProblemResponse
	apiCall(t, http.MethodGet, prefix+"/problems/"+p.ID, owner, nil, &view, 200)
	if strings.Contains(view.StatementMD, "{{nextsample}}") || !strings.Contains(view.StatementMD, "(#sample-1)") || !strings.Contains(view.StatementMD, "(#sample-2)") {
		t.Fatalf("sample directives not placed: %s", view.StatementMD)
	}
	encoded, _ := json.Marshal(view)
	for _, secret := range []string{"PRIVATE-SECRET-CASE", "NEVER-PUBLIC-REFERENCE", "fixture-source", "blobSha256"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("public metadata leaked %q", secret)
		}
	}
	if len(view.Files) != 7 || len(encoded) > 20000 || strings.Contains(view.StatementMD, large) {
		t.Fatalf("presentation metadata is unbounded or incomplete: files=%d bytes=%d", len(view.Files), len(encoded))
	}
	foundLarge, foundBinary := false, false
	for _, file := range view.Files {
		if file.ID == "sample-1-input" {
			foundLarge = file.Truncated && !file.Binary && len(file.Preview) <= 4096 && file.Size == int64(len(large))
		}
		if file.ID == "sample-2-input" {
			foundBinary = file.Binary && file.Preview == "" && !file.Embedded
		}
	}
	if !foundLarge || !foundBinary {
		t.Fatal("sample presentation flags are incorrect")
	}
	fileURL := func(id string, version int) string {
		return fmt.Sprintf("%s/problems/%s/files/%s?version=%d", prefix, p.ID, id, version)
	}
	if got := string(authoringRawResponse(t, fileURL("sample-1-input", 1), owner, 200)); got != large {
		t.Fatal("large sample was truncated on download")
	}
	if got := string(authoringRawResponse(t, fileURL("sample-2-input", 1), owner, 200)); got != binary {
		t.Fatal("binary sample changed")
	}
	authoringRawResponse(t, fileURL("asset-guide", 1), viewer, 404)
	authoringRawResponse(t, fileURL("asset-private-notes", 1), owner, 404)
	authoringRawResponse(t, fileURL("fixture-input-2", 1), owner, 404)

	now := time.Now()
	settings := map[string]any{"title": "Pinned presentation", "format": "icpc", "beginAt": now.Add(time.Hour).Format(time.RFC3339), "endAt": now.Add(3 * time.Hour).Format(time.RFC3339), "visibility": "public", "allowSelfRegistration": true, "admission": "members", "feedback": "full"}
	var contest numberedResource
	apiCall(t, http.MethodPost, prefix+"/admin/contests", owner, settings, &contest, 201)
	apiCall(t, http.MethodPut, prefix+"/admin/contests/"+contest.ID+"/problems", owner, map[string]any{"problems": []map[string]any{{"problemId": p.ID, "label": "A", "points": 100}}}, nil, 200)
	apiCall(t, http.MethodPost, prefix+"/contests/"+contest.ID+"/register", viewer, nil, nil, 200)
	contestFile := func(id string, version int) string {
		return fmt.Sprintf("%s/contests/%s/problems/A/files/%s?version=%d", prefix, contest.ID, id, version)
	}
	authoringRawResponse(t, contestFile("sample-1-input", 1), viewer, 404)
	settings["beginAt"] = now.Add(-time.Hour).Format(time.RFC3339)
	apiCall(t, http.MethodPut, prefix+"/admin/contests/"+contest.ID, owner, settings, nil, 200)
	if got := string(authoringRawResponse(t, contestFile("asset-guide", 1), viewer, 200)); got != "public attachment" {
		t.Fatal("contest participant could not read the pinned private problem attachment")
	}

	// A new PDF release cannot change the bytes served through a pinned contest.
	pdf := publicationPDF()
	if filename := os.Getenv("E2E_PRESENTATION_PDF"); filename != "" {
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		pdf = string(data)
	}
	ref := uploadAuthoringBlob(t, endpoint, owner, pdf)
	apiCall(t, http.MethodGet, endpoint+"/working-copy", owner, nil, &copy, 200)
	var statement domain.TreeEntry
	for _, entry := range copy.Tree.Entries {
		if entry.Kind == domain.EntryStatement {
			statement = entry
			break
		}
	}
	statement.Path = "statement/problem.zh.pdf"
	statement.Attributes["format"] = "pdf"
	statement.Blob = ref
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/"+statement.ID, owner, map[string]any{"etag": copy.ETag, "entry": statement}, &copy, 200)
	if version := publishFixtureAt(t, endpoint, owner); version != 2 {
		t.Fatal("PDF release not published")
	}
	apiCall(t, http.MethodPut, endpoint+"/visibility", owner, domain.VisibilityChange{Visibility: "public", ExpectedVisibility: "private"}, nil, 200)
	if got := string(authoringRawResponse(t, fileURL("statement", 2), viewer, 200)); got != pdf {
		t.Fatal("PDF download changed")
	}
	authoringRawResponse(t, fileURL("statement", 1), viewer, 404)
	if got := string(authoringRawResponse(t, contestFile("statement", 1), viewer, 200)); got != wording {
		t.Fatal("contest used the newer practice statement")
	}
	authoringRawResponse(t, contestFile("statement", 2), viewer, 404)
	apiCall(t, http.MethodGet, prefix+"/problems/"+p.ID, viewer, nil, &view, 200)
	if view.StatementMD != "" {
		t.Fatal("PDF source leaked into Markdown body")
	}
	if len(view.Files) == 0 || view.Files[0].MediaType == "" {
		t.Fatal("PDF presentation metadata absent")
	}
	apiCall(t, http.MethodPut, base+"/api/domains/"+space.Slug, owner, map[string]any{"name": "Presentation validation", "visibility": "public", "joinPolicy": "invite"}, nil, 200)
	if got := string(authoringRawResponse(t, fileURL("statement", 2), "", 200)); got != pdf {
		t.Fatal("public statement file was not available anonymously")
	}
	authoringRawResponse(t, fileURL("statement", 1), "", 404)
	request, _ := http.NewRequest(http.MethodGet, fileURL("statement", 2), nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(response.Header.Get("Content-Disposition"), "attachment") || !strings.Contains(response.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatal("presentation download security headers missing")
	}
	if got := authoringRawResponse(t, fileURL("asset-diagram", 2), viewer, 200); !bytes.Equal(got, diagram) {
		t.Fatal("public image bytes changed")
	}
	t.Logf("Presentation QA: domain=%s problem=%s contest=%s owner=%s", space.Slug, p.ID, contest.ID, ownerName)
}

func publicationPNG(t *testing.T) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 240, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 240; x++ {
			picture.Set(x, y, color.RGBA{R: 70, G: 115, B: 200, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, picture); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

// A small valid, non-scripted PDF fixture; no external renderer is needed by the
// API-only suite. Browser QA may supply the separately verified statement PDF.
func publicationPDF() string {
	stream := "BT /F1 18 Tf 30 150 Td (PDF publication fixture) Tj ET"
	second := "BT /F1 18 Tf 30 150 Td (Second page fixture) Tj ET"
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R 6 0 R] /Count 2 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>", fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream), "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Resources << /Font << /F1 4 0 R >> >> /Contents 7 0 R >>", fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(second), second)}
	var output strings.Builder
	output.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for index, object := range objects {
		offsets = append(offsets, output.Len())
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&output, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return output.String()
}
