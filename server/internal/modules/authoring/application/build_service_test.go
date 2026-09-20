package application

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	authoringfiles "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/filesystem"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeBuilds struct {
	claimed         *authoringdomain.Build
	claimedPackage  *authoringdomain.CheckInput
	targetProblemID string
	targetErr       error
	recordErr       error
	recordedProblem string
	recordedUpload  *authoringdomain.PackageUpload
}

func (f *fakeBuilds) WithArtifactStorage(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}

func (f *fakeBuilds) Progress(context.Context, authoringdomain.Progress, time.Duration) error {
	return nil
}
func (f *fakeBuilds) Claim(context.Context, string, time.Duration) (*authoringdomain.Build, *authoringdomain.CheckInput, error) {
	return f.claimed, f.claimedPackage, nil
}
func (f *fakeBuilds) ResolvePackageTarget(context.Context, string, string, string) (string, error) {
	if f.targetErr != nil {
		return "", f.targetErr
	}
	if f.targetProblemID == "" {
		return "problem-1", nil
	}
	return f.targetProblemID, nil
}
func (f *fakeBuilds) RecordPackage(
	_ context.Context, _, problemID, _, _ string, upload authoringdomain.PackageUpload,
) error {
	f.recordedProblem = problemID
	f.recordedUpload = &upload
	return f.recordErr
}
func (f *fakeBuilds) Complete(context.Context, authoringdomain.BuildResult, string) error { return nil }

type fakePublisher struct {
	upload           *authoringdomain.PackageUpload
	err              error
	publishedProblem string
	publishCalls     int
	removed          []string
	removeErr        error
}

func (f *fakePublisher) Publish(problemID string, _ []byte) (*authoringdomain.PackageUpload, error) {
	f.publishCalls++
	f.publishedProblem = problemID
	if f.err != nil {
		return nil, f.err
	}
	if f.upload == nil {
		return &authoringdomain.PackageUpload{
			StoragePath: problemID + "/h", SHA256: "h", CaseCount: 1,
			Checker: "testlib", Created: true,
		}, nil
	}
	return f.upload, nil
}

func (f *fakePublisher) Remove(storagePath string) error {
	f.removed = append(f.removed, storagePath)
	return f.removeErr
}

func newWorkerService(builds *fakeBuilds, publisher authoringdomain.ArtifactPublisher) *BuildService {
	service, err := NewBuildService(builds, publisher, NewDispatcher(1), 45*time.Second, 25*time.Second)
	Expect(err).NotTo(HaveOccurred())
	return service
}

var _ = Describe("Frozen check Worker fencing", func() {
	It("only serves the current frozen-check protocol", func() {
		builds := &fakeBuilds{claimed: &authoringdomain.Build{ID: "frozen"}, claimedPackage: &authoringdomain.CheckInput{}}
		service := newWorkerService(builds, &fakePublisher{})
		_, _, err := service.Claim(context.Background(), "worker", time.Second)
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
		ctx := authoringdomain.WithCheckProtocol(context.Background(), authoringdomain.CheckPolicyVersion)
		_, _, err = service.Claim(ctx, "worker", time.Second)
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
		builds.claimedPackage.Check = &authoringdomain.CheckSnapshot{SchemaVersion: 1, PolicyVersion: authoringdomain.CheckPolicyVersion}
		build, pkg, err := service.Claim(ctx, "worker", time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(build.ID).To(Equal("frozen"))
		Expect(pkg.Check).NotTo(BeNil())
	})
	It("rejects progress without a complete lease identity", func() {
		service := newWorkerService(&fakeBuilds{}, &fakePublisher{})
		Expect(service.Progress(context.Background(), authoringdomain.Progress{BuildID: "b"})).
			To(MatchError(authoringdomain.ErrStaleLease))
	})

	It("rejects a result without a complete lease identity", func() {
		service := newWorkerService(&fakeBuilds{}, &fakePublisher{})
		Expect(service.Complete(context.Background(), authoringdomain.BuildResult{BuildID: "b"}, "testlib")).
			To(MatchError(authoringdomain.ErrStaleLease))
	})

	It("rejects a package upload without a complete lease identity", func() {
		service := newWorkerService(&fakeBuilds{}, &fakePublisher{})
		_, err := service.UploadPackage(context.Background(), "b", "p", "", "", nil)
		Expect(err).To(MatchError(authoringdomain.ErrStaleLease))
	})

	It("rejects a stale lease before writing an artifact", func() {
		publisher := &fakePublisher{}
		service := newWorkerService(
			&fakeBuilds{targetErr: authoringdomain.ErrStaleLease}, publisher)
		_, err := service.UploadPackage(context.Background(),
			"build-1", "problem-1", "worker-1", "lease-1", []byte("archive"))
		Expect(err).To(MatchError(authoringdomain.ErrStaleLease))
		Expect(publisher.publishCalls).To(BeZero())
	})

	DescribeTable("rejects an untrusted problem target before writing",
		func(requestedProblemID string) {
			publisher := &fakePublisher{}
			builds := &fakeBuilds{targetProblemID: "problem-1"}
			service := newWorkerService(builds, publisher)
			_, err := service.UploadPackage(context.Background(),
				"build-1", requestedProblemID, "worker-1", "lease-1", []byte("archive"))
			Expect(err).To(MatchError(authoringdomain.ErrPackageTarget))
			Expect(publisher.publishCalls).To(BeZero())
			Expect(builds.recordedUpload).To(BeNil())
		},
		Entry("another problem", "problem-2"),
		Entry("a parent-directory traversal", "../outside"),
		Entry("an absolute-looking path", "C:\\outside"),
	)

	It("uses the server-resolved target for both disk and database writes", func() {
		publisher := &fakePublisher{}
		builds := &fakeBuilds{targetProblemID: "problem-1"}
		service := newWorkerService(builds, publisher)
		upload, err := service.UploadPackage(context.Background(),
			"build-1", "problem-1", "worker-1", "lease-1", []byte("archive"))
		Expect(err).NotTo(HaveOccurred())
		Expect(upload.StoragePath).To(Equal("problem-1/h"))
		Expect(publisher.publishedProblem).To(Equal("problem-1"))
		Expect(builds.recordedProblem).To(Equal("problem-1"))
	})

	It("retains a newly installed artifact after a failed write because another lease may reference it", func() {
		root := GinkgoT().TempDir()
		publisher := authoringfiles.NewTestdataPublisher(root)
		builds := &fakeBuilds{targetProblemID: "problem-1", recordErr: errors.New("database failed")}
		service := newWorkerService(builds, publisher)

		_, err := service.UploadPackage(context.Background(),
			"build-1", "problem-1", "worker-1", "lease-1",
			makePackage(map[string]string{"1.in": "1\n", "1.out": "1\n"}))
		Expect(err).To(MatchError("database failed"))
		artifacts, readErr := os.ReadDir(filepath.Join(root, "problem-1"))
		Expect(readErr).NotTo(HaveOccurred())
		Expect(artifacts).To(HaveLen(1))
	})

	It("does not delete content when the lease becomes stale during publication", func() {
		publisher := &fakePublisher{}
		builds := &fakeBuilds{targetProblemID: "problem-1", recordErr: authoringdomain.ErrStaleLease}
		service := newWorkerService(builds, publisher)

		_, err := service.UploadPackage(context.Background(),
			"build-1", "problem-1", "worker-1", "lease-1", []byte("archive"))
		Expect(err).To(MatchError(authoringdomain.ErrStaleLease))
		Expect(publisher.removed).To(BeEmpty())
	})

	It("does not delete a pre-existing content-addressed artifact after a database failure", func() {
		root := GinkgoT().TempDir()
		publisher := authoringfiles.NewTestdataPublisher(root)
		archive := makePackage(map[string]string{"1.in": "1\n", "1.out": "1\n"})
		existing, err := publisher.Publish("problem-1", archive)
		Expect(err).NotTo(HaveOccurred())
		Expect(existing.Created).To(BeTrue())

		builds := &fakeBuilds{targetProblemID: "problem-1", recordErr: errors.New("database failed")}
		service := newWorkerService(builds, publisher)
		_, err = service.UploadPackage(context.Background(),
			"build-1", "problem-1", "worker-1", "lease-1", archive)
		Expect(err).To(MatchError("database failed"))
		_, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(existing.StoragePath), "1.in"))
		Expect(statErr).NotTo(HaveOccurred())
	})
})

func TestAuthoringApplication(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthoringApplication")
}

func makePackage(entries map[string]string) []byte {
	manifest := authoringdomain.CheckArtifact{SchemaVersion: 1, ToolchainKey: strings.Repeat("b", 64), Snapshot: authoringdomain.CheckSnapshot{SchemaVersion: 1, PolicyVersion: authoringdomain.CheckPolicyVersion, TreeHash: strings.Repeat("a", 64), Tests: []authoringdomain.SnapshotTest{{ID: "one"}}}, Tests: []authoringdomain.ArtifactTest{{ID: "one", Input: authoringdomain.Reference([]byte(entries["1.in"])), Answer: authoringdomain.Reference([]byte(entries["1.out"]))}}}
	manifest.Snapshot.DataHash, _ = manifest.Snapshot.DataFingerprint()
	data, err := json.Marshal(manifest)
	Expect(err).NotTo(HaveOccurred())
	files := map[string]string{"artifact.json": string(data)}
	for name, body := range entries {
		files[name] = body
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range files {
		file, err := writer.Create(name)
		Expect(err).NotTo(HaveOccurred())
		_, err = file.Write([]byte(body))
		Expect(err).NotTo(HaveOccurred())
	}
	Expect(writer.Close()).To(Succeed())
	return buffer.Bytes()
}
