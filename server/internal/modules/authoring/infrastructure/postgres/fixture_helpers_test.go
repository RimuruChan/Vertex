package postgres_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	app "github.com/RimuruChan/Vertex/server/internal/modules/authoring/application"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/filesystem"
	pg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	. "github.com/onsi/gomega"
)

type authoringFixture struct {
	ctx       context.Context
	id        string
	repo      *pg.RevisionRepository
	service   *app.Workbench
	blobs     *filesystem.BlobStore
	publisher *filesystem.TestdataPublisher
	copy      *domain.WorkingCopy
}

func newAuthoringFixture(ctx context.Context, id, root string) *authoringFixture {
	blobs, err := filesystem.NewBlobStore(root, 64<<20)
	Expect(err).NotTo(HaveOccurred())
	publisher := filesystem.NewTestdataPublisher(root)
	repo := pg.NewRevisionRepository(integrationDB, blobs, publisher)
	f := &authoringFixture{ctx: ctx, id: id, repo: repo, service: app.NewWorkbench(repo), blobs: blobs, publisher: publisher}
	f.copy, err = repo.Open(ctx, id)
	Expect(err).NotTo(HaveOccurred())
	f.save("main", "solutions/main.cpp", domain.EntrySource, "private source")
	f.document("reference", "vertex/programs/reference.json", domain.EntryProgram, domain.ProgramMaterial{SchemaVersion: 1, Name: "Reference", Directory: "solutions", Role: "solution", Language: "cpp", Protocol: "stdio", Files: []string{"main"}, EntryPoint: "main", ExpectedVerdicts: []string{"Accepted"}})
	f.save("input", "data/1.in", domain.EntryInput, "hidden input")
	f.save("answer", "data/1.ans", domain.EntryAnswer, "fixture answer")
	f.document("case", "vertex/tests/1.json", domain.EntryTest, domain.TestMaterial{SchemaVersion: 1, Name: "Fixture", Input: domain.TestInput{Kind: "file", Entry: "input"}, Answer: domain.TestAnswer{Kind: "file", Entry: "answer"}})
	material, err := f.service.Material(ctx, id, "problem", 0)
	Expect(err).NotTo(HaveOccurred())
	material.Metadata.MainSolution = "reference"
	f.document("problem", "vertex/problem.json", domain.EntryMetadata, material.Metadata)
	return f
}

func (f *authoringFixture) save(id, path, kind, text string) {
	next, err := f.service.SaveEntry(f.ctx, f.id, f.copy.ETag, domain.TreeEntry{ID: id, Path: path, Kind: kind}, &text)
	Expect(err).NotTo(HaveOccurred())
	f.copy = next
}
func (f *authoringFixture) document(id, path, kind string, value any) {
	encoded, err := json.Marshal(value)
	Expect(err).NotTo(HaveOccurred())
	f.save(id, path, kind, string(encoded))
}
func (f *authoringFixture) commit(requestID string) int64 {
	value, err := f.repo.Commit(f.ctx, f.id, domain.CommitInput{ETag: f.copy.ETag, Message: "Reviewed fixture", RequestID: requestID})
	Expect(err).NotTo(HaveOccurred())
	f.copy = &value.Copy
	return value.Commit.Revision
}

// This creates a completed fixture artifact for authorization/transaction tests.
// No code is executed; native Worker execution is covered by the E2E suite.
func (f *authoringFixture) checked() string {
	check, err := f.repo.StartCheck(f.ctx, f.id, domain.CheckSelection{ETag: f.copy.ETag})
	Expect(err).NotTo(HaveOccurred())
	load := func(ref domain.BlobRef) ([]byte, error) {
		reader, err := f.blobs.Open(f.ctx, f.id, ref)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	}
	_, snapshot, err := domain.InspectMaterials(f.copy.Tree, load)
	Expect(err).NotTo(HaveOccurred())
	manifest := domain.CheckArtifact{SchemaVersion: 1, ToolchainKey: strings.Repeat("c", 64), Snapshot: *snapshot, Tests: []domain.ArtifactTest{{ID: "case", Input: *snapshot.Tests[0].Input, Answer: *snapshot.Tests[0].Answer}}}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	write := func(name string, data []byte) {
		entry, err := writer.Create(name)
		Expect(err).NotTo(HaveOccurred())
		_, err = entry.Write(data)
		Expect(err).NotTo(HaveOccurred())
	}
	encoded, err := json.Marshal(manifest)
	Expect(err).NotTo(HaveOccurred())
	write("artifact.json", encoded)
	for index, test := range manifest.Tests {
		for suffix, ref := range map[string]domain.BlobRef{"in": test.Input, "out": test.Answer} {
			data, err := load(ref)
			Expect(err).NotTo(HaveOccurred())
			write(fmt.Sprintf("%d.%s", index+1, suffix), data)
		}
	}
	for _, program := range snapshot.Programs {
		for _, file := range program.Files {
			data, err := load(file.Blob)
			Expect(err).NotTo(HaveOccurred())
			write("programs/"+program.ID+"/"+file.Path, data)
		}
	}
	Expect(writer.Close()).To(Succeed())
	upload, err := f.publisher.Publish(f.id, archive.Bytes())
	Expect(err).NotTo(HaveOccurred())
	_, err = integrationDB.Pool.ExecContext(f.ctx, "UPDATE problem_build_jobs SET state='succeeded',toolchain_key=$2,package_manifest=$3,package_path=$4,package_sha256=$5,package_cases=1,finished_at=now(),log='Authorization fixture only; programs were not executed' WHERE id=$1", check.ID, manifest.ToolchainKey, encoded, upload.StoragePath, upload.SHA256)
	Expect(err).NotTo(HaveOccurred())
	return check.ID
}
