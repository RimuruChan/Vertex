package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"io"
	"reflect"
	"strings"
	"testing"
)

type workflowRepository struct {
	domain.RevisionRepository
	copy                  domain.WorkingCopy
	blobs                 map[string][]byte
	reads                 []string
	uploads, writes       int
	denied, uploadFailure bool
}

func (r *workflowRepository) AuthorizeEdit(context.Context, string) error {
	if r.denied {
		return errors.New("denied")
	}
	return nil
}
func (r *workflowRepository) WorkingCopy(context.Context, string) (*domain.WorkingCopy, error) {
	return &r.copy, nil
}
func (r *workflowRepository) Revision(context.Context, string, int64) (*domain.ContentCommit, domain.ContentTree, error) {
	return &domain.ContentCommit{Revision: 1}, r.copy.Tree, nil
}
func (r *workflowRepository) Blob(_ context.Context, _ string, digest string) (domain.BlobRef, io.ReadCloser, error) {
	r.reads = append(r.reads, digest)
	data, ok := r.blobs[digest]
	if !ok {
		return domain.BlobRef{}, nil, domain.ErrNotFound
	}
	return domain.Reference(data), io.NopCloser(strings.NewReader(string(data))), nil
}
func (r *workflowRepository) Upload(_ context.Context, _ string, reader io.Reader) (domain.BlobRef, error) {
	if r.uploadFailure {
		return domain.BlobRef{}, errors.New("upload failed")
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return domain.BlobRef{}, err
	}
	ref := domain.Reference(data)
	r.blobs[ref.SHA256] = data
	r.uploads++
	return ref, nil
}
func (r *workflowRepository) Save(_ context.Context, _ string, etag string, tree domain.ContentTree) (*domain.WorkingCopy, error) {
	if r.denied {
		return nil, errors.New("denied")
	}
	if etag != r.copy.ETag {
		return nil, domain.ErrWorkingCopyConflict
	}
	r.writes++
	r.copy.Tree = tree
	r.copy.ETag += "x"
	return &r.copy, nil
}
func (r *workflowRepository) add(id, kind, location string, value any) domain.TreeEntry {
	var data []byte
	if s, ok := value.(string); ok {
		data = []byte(s)
	} else {
		data, _ = json.Marshal(value)
	}
	entry := domain.TreeEntry{ID: id, Kind: kind, Path: location, Blob: domain.Reference(data), Attributes: map[string]string{}}
	r.blobs[entry.Blob.SHA256] = data
	r.copy.Tree.Entries = append(r.copy.Tree.Entries, entry)
	return entry
}
func workflowFixture(t *testing.T) *workflowRepository {
	t.Helper()
	r := &workflowRepository{copy: domain.WorkingCopy{ETag: "v1", Tree: domain.ContentTree{Entries: []domain.TreeEntry{}}}, blobs: map[string][]byte{}}
	initial, err := domain.InitialMaterials("测试题", "## 描述\n内容。", "zh", "", "normal", 1000, 262144)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range initial {
		r.blobs[m.Entry.Blob.SHA256] = m.Data
		r.copy.Tree.Entries = append(r.copy.Tree.Entries, m.Entry)
	}
	for _, role := range []string{"solution", "generator"} {
		expected := []string{}
		if role == "solution" {
			expected = []string{"Accepted"}
		}
		r.add(role, "program", "vertex/programs/"+role+".json", domain.ProgramMaterial{SchemaVersion: 1, Name: role, Role: role, Language: "python", Protocol: "stdio", Files: []string{}, ExpectedVerdicts: expected, Arguments: []string{}})
	}
	return r
}
func TestGenerationIsAuthorizedPreviewedAndAtomic(t *testing.T) {
	r := workflowFixture(t)
	service := NewWorkbench(r)
	input := domain.GenerationInput{ETag: r.copy.ETag, ID: "plan", Preview: true, Plan: domain.GenerationPlan{SchemaVersion: 1, Name: "随机", Generator: "generator", Solution: "solution", Rules: []domain.GenerationRule{{ID: "small", Name: "小规模", Count: 3, SeedStart: 10, Parameters: "100 {seed}"}}}}
	preview, err := service.Generate(context.Background(), "p", input)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Cases) != 3 || r.writes != 0 || r.uploads != 0 {
		t.Fatal("preview mutated state")
	}
	input.Preview = false
	result, err := service.Generate(context.Background(), "p", input)
	if err != nil {
		t.Fatal(err)
	}
	if r.writes != 1 || result.Copy == nil {
		t.Fatal("not one atomic save")
	}
	for _, entry := range result.Copy.Tree.Entries {
		if entry.Kind == domain.EntryTest {
			var test domain.TestMaterial
			_ = json.Unmarshal(r.blobs[entry.Blob.SHA256], &test)
			if test.Input.Kind != "generator" || test.Answer.Solution != "solution" || entry.Attributes["generationPlan"] != "plan" {
				t.Fatal("generation provenance missing")
			}
		}
	}
	if _, err = service.Generate(context.Background(), "p", input); !errors.Is(err, domain.ErrWorkingCopyConflict) {
		t.Fatal("stale request accepted")
	}
	input.ETag = r.copy.ETag
	before := r.copy.Tree
	_, err = service.Generate(context.Background(), "p", input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, r.copy.Tree) {
		t.Fatal("reapplying changed expanded tests")
	}
	r.denied = true
	r.reads = nil
	if _, err = service.Generate(context.Background(), "p", input); err == nil || len(r.reads) != 0 {
		t.Fatal("unauthorized generation accessed blobs")
	}
}
func TestGenerationUploadFailureLeavesCopyUnchanged(t *testing.T) {
	r := workflowFixture(t)
	before := r.copy.Tree
	r.uploadFailure = true
	_, err := NewWorkbench(r).Generate(context.Background(), "p", domain.GenerationInput{ETag: r.copy.ETag, ID: "plan", Plan: domain.GenerationPlan{SchemaVersion: 1, Name: "test", Generator: "generator", Solution: "solution", Rules: []domain.GenerationRule{{ID: "r", Name: "r", Count: 2, SeedStart: 1}}}})
	if err == nil || r.writes != 0 || !reflect.DeepEqual(before, r.copy.Tree) {
		t.Fatal("partial generation save")
	}
}
func TestStatementPreviewNeverReadsSecretTestBytes(t *testing.T) {
	r := workflowFixture(t)
	for _, name := range []string{"public", "secret"} {
		body := "1 2\n"
		if name == "secret" {
			body = "TOP_SECRET_INPUT"
		}
		r.add(name+"-input", "input", "data/"+name+".in", body)
		r.add(name+"-answer", "answer", "data/"+name+".ans", name+" answer")
		r.add(name, "test", "vertex/tests/"+name+".json", domain.TestMaterial{SchemaVersion: 1, Name: name, IsSample: name == "public", Input: domain.TestInput{Kind: "file", Entry: name + "-input", Arguments: []string{}}, Answer: domain.TestAnswer{Kind: "file", Entry: name + "-answer"}})
	}
	for i, e := range r.copy.Tree.Entries {
		if e.ID == "problem" {
			var metadata domain.PackageMetadata
			_ = json.Unmarshal(r.blobs[e.Blob.SHA256], &metadata)
			metadata.TestOrder = []string{"public", "secret"}
			data, _ := json.Marshal(metadata)
			ref := domain.Reference(data)
			r.blobs[ref.SHA256] = data
			r.copy.Tree.Entries[i].Blob = ref
		}
	}
	result, err := NewWorkbench(r).PreviewStatement(context.Background(), "p", domain.StatementPreviewInput{ETag: r.copy.ETag, EntryID: "statement-zh", Content: "## 说明\n\n{{remainingsamples}}"})
	if err != nil {
		t.Fatal(err)
	}
	if result.SampleCount != 1 || !strings.Contains(result.Content, "1 2") || strings.Contains(result.Content, "TOP_SECRET") {
		t.Fatal("bad public sample projection")
	}
	secret := domain.Reference([]byte("TOP_SECRET_INPUT")).SHA256
	for _, digest := range r.reads {
		if digest == secret {
			t.Fatal("secret input was read even though not displayed")
		}
	}
}
