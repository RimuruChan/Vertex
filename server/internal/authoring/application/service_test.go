package application

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/authoring/domain"
	authoringfiles "github.com/RimuruChan/Vertex/server/internal/authoring/infrastructure/filesystem"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakePackages records what the service decided to persist so the specs can
// assert on normalization rather than on SQL.
type fakePackages struct {
	copyInput *authoringdomain.CopyInput
	saved     *authoringdomain.File
	savedTest *authoringdomain.Test
	snapshot  *authoringdomain.Package
	meta      *authoringdomain.PackageMeta
}

func (f *fakePackages) Copy(_ context.Context, input authoringdomain.CopyInput, _ authoringdomain.ArtifactCopier) (*authoringdomain.CopyResult, error) {
	f.copyInput = &input
	return &authoringdomain.CopyResult{ProblemID: "copy"}, nil
}
func (*fakePackages) Origin(context.Context, string) (*authoringdomain.CopyOrigin, error) {
	return nil, nil
}

func (f *fakePackages) Statements(context.Context, string) ([]authoringdomain.Statement, error) {
	return nil, nil
}
func (f *fakePackages) SaveStatement(_ context.Context, statement authoringdomain.Statement) (*authoringdomain.Statement, error) {
	return &statement, nil
}
func (f *fakePackages) DeleteStatement(context.Context, string, string) error { return nil }
func (f *fakePackages) Files(context.Context, string, bool) ([]authoringdomain.File, error) {
	return nil, nil
}
func (f *fakePackages) File(context.Context, string, int64) (*authoringdomain.File, error) {
	return nil, authoringdomain.ErrNotFound
}
func (f *fakePackages) SaveFile(_ context.Context, file authoringdomain.File) (*authoringdomain.File, error) {
	f.saved = &file
	return &file, nil
}
func (f *fakePackages) DeleteFile(context.Context, string, int64) error { return nil }
func (f *fakePackages) Tests(context.Context, string, bool) ([]authoringdomain.Test, error) {
	return nil, nil
}
func (f *fakePackages) Test(context.Context, string, int64) (*authoringdomain.Test, error) {
	return f.savedTest, nil
}
func (f *fakePackages) CreateTest(_ context.Context, test authoringdomain.Test) (*authoringdomain.Test, error) {
	f.savedTest = &test
	return &test, nil
}
func (f *fakePackages) UpdateTest(_ context.Context, test authoringdomain.Test) (*authoringdomain.Test, error) {
	f.savedTest = &test
	return &test, nil
}
func (f *fakePackages) DeleteTest(context.Context, string, int64) error       { return nil }
func (f *fakePackages) ReorderTest(context.Context, string, int64, int) error { return nil }
func (f *fakePackages) Snapshot(context.Context, string) (*authoringdomain.Package, error) {
	if f.snapshot == nil {
		return &authoringdomain.Package{}, nil
	}
	return f.snapshot, nil
}
func (f *fakePackages) Meta(context.Context, string) (*authoringdomain.PackageMeta, error) {
	if f.meta == nil {
		return &authoringdomain.PackageMeta{}, nil
	}
	return f.meta, nil
}

type fakeBuilds struct {
	enqueued        int
	build           *authoringdomain.Build
	err             error
	targetProblemID string
	targetErr       error
	recordErr       error
	recordedProblem string
	recordedUpload  *authoringdomain.PackageUpload
}

func (f *fakePackages) Publish(context.Context, string, authoringdomain.PublishInput) (*authoringdomain.Release, error) {
	return &authoringdomain.Release{}, nil
}
func (f *fakePackages) Releases(context.Context, string) ([]authoringdomain.Release, error) {
	return nil, nil
}
func (f *fakePackages) Samples(context.Context, string) ([]authoringdomain.TestOutcome, error) {
	return nil, nil
}

func (f *fakeBuilds) Enqueue(context.Context, string, string) (*authoringdomain.Build, error) {
	f.enqueued++
	if f.err != nil {
		return f.build, f.err
	}
	return &authoringdomain.Build{ID: "build-1", State: authoringdomain.BuildQueued}, nil
}
func (f *fakeBuilds) Get(context.Context, string, string) (*authoringdomain.Build, error) {
	return f.build, nil
}
func (f *fakeBuilds) Latest(context.Context, string) (*authoringdomain.Build, error) {
	return f.build, nil
}
func (f *fakeBuilds) LatestSuccessful(context.Context, string) (*authoringdomain.Build, error) {
	return nil, nil
}
func (f *fakeBuilds) List(context.Context, string, int) ([]authoringdomain.Build, error) {
	return nil, nil
}
func (f *fakeBuilds) Cancel(context.Context, string, string) error { return nil }
func (f *fakeBuilds) Progress(context.Context, authoringdomain.Progress, time.Duration) error {
	return nil
}
func (f *fakeBuilds) Claim(context.Context, string, time.Duration) (*authoringdomain.Build, *authoringdomain.Package, error) {
	return nil, nil, nil
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

func (f *fakePublisher) Clone(_ context.Context, _, target string, _ authoringdomain.PackageUpload) (*authoringdomain.PackageUpload, error) {
	return f.Publish(target, nil)
}

func newService(packages *fakePackages, builds *fakeBuilds) *Service {
	return newServiceWithPublisher(packages, builds, &fakePublisher{})
}

func newServiceWithPublisher(packages *fakePackages, builds *fakeBuilds, publisher authoringdomain.Publisher) *Service {
	service, err := NewService(packages, builds, publisher, NewDispatcher(1), 45*time.Second, 25*time.Second)
	Expect(err).NotTo(HaveOccurred())
	return service
}

var _ = Describe("Service file rules", func() {
	var packages *fakePackages
	var service *Service

	BeforeEach(func() {
		packages = &fakePackages{}
		service = newService(packages, &fakeBuilds{})
	})

	It("rejects a checker that is not C++ because testlib is a C++ header", func() {
		_, err := service.SaveFile(context.Background(), authoringdomain.File{
			Kind: authoringdomain.KindChecker, Name: "check", Language: "python", SourceCode: "print(1)",
		})
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
	})

	It("always activates a saved checker so the package has an entry point", func() {
		_, err := service.SaveFile(context.Background(), authoringdomain.File{
			Kind: authoringdomain.KindChecker, Name: "check", Language: "cpp", SourceCode: "int main(){}",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(packages.saved.IsActive).To(BeTrue())
	})

	It("forces the main solution's expected verdict to Accepted", func() {
		_, err := service.SaveFile(context.Background(), authoringdomain.File{
			Kind: authoringdomain.KindSolution, Name: "std", Language: "cpp", SourceCode: "int main(){}",
			IsActive: true, ExpectedVerdict: "Wrong Answer",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(packages.saved.ExpectedVerdict).To(Equal("Accepted"))
	})

	It("rejects an unknown expected verdict on an alternate solution", func() {
		_, err := service.SaveFile(context.Background(), authoringdomain.File{
			Kind: authoringdomain.KindSolution, Name: "brute", Language: "cpp", SourceCode: "int main(){}",
			ExpectedVerdict: "Almost Accepted",
		})
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
	})

	It("rejects names that could escape a sandbox file name", func() {
		_, err := service.SaveFile(context.Background(), authoringdomain.File{
			Kind: authoringdomain.KindGenerator, Name: "../gen", Language: "cpp", SourceCode: "int main(){}",
		})
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
	})

	It("rejects empty sources", func() {
		_, err := service.SaveFile(context.Background(), authoringdomain.File{
			Kind: authoringdomain.KindGenerator, Name: "gen", Language: "cpp", SourceCode: "   ",
		})
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
	})
})

var _ = Describe("Service test rules", func() {
	var packages *fakePackages
	var service *Service

	BeforeEach(func() {
		packages = &fakePackages{}
		service = newService(packages, &fakeBuilds{})
	})

	It("clears the generator command on a manual test", func() {
		_, err := service.CreateTest(context.Background(), authoringdomain.Test{
			Source: authoringdomain.TestManual, InputData: "1 2\n", GenerateCmd: "gen 5",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(packages.savedTest.GenerateCmd).To(BeEmpty())
	})

	It("clears inline input on a generated test", func() {
		_, err := service.CreateTest(context.Background(), authoringdomain.Test{
			Source: authoringdomain.TestGenerator, GenerateCmd: "gen 100 -seed=7", InputData: "stale",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(packages.savedTest.InputData).To(BeEmpty())
	})

	It("rejects a manual test with no input", func() {
		_, err := service.CreateTest(context.Background(), authoringdomain.Test{Source: authoringdomain.TestManual})
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
	})

	It("rejects an unknown test source", func() {
		_, err := service.CreateTest(context.Background(), authoringdomain.Test{Source: "uploaded"})
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
	})
})

var _ = Describe("Service build gating", func() {
	It("refuses to queue a build for an incomplete package", func() {
		builds := &fakeBuilds{}
		service := newService(&fakePackages{}, builds)
		_, err := service.Build(context.Background(), "problem-1", "user-1")
		Expect(err).To(MatchError(authoringdomain.ErrNotBuildable))
		Expect(builds.enqueued).To(Equal(0))
	})

	It("queues a build for a complete package", func() {
		packages := &fakePackages{snapshot: &authoringdomain.Package{
			Solutions: []authoringdomain.File{{Name: "std", Kind: authoringdomain.KindSolution, IsActive: true}},
			Tests:     []authoringdomain.Test{{Index: 1, Source: authoringdomain.TestManual, InputData: "1"}},
		}}
		builds := &fakeBuilds{}
		service := newService(packages, builds)
		build, err := service.Build(context.Background(), "problem-1", "user-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(build.ID).To(Equal("build-1"))
		Expect(builds.enqueued).To(Equal(1))
	})

	It("surfaces the running build when one is already in flight", func() {
		packages := &fakePackages{snapshot: &authoringdomain.Package{
			Solutions: []authoringdomain.File{{Name: "std", Kind: authoringdomain.KindSolution, IsActive: true}},
			Tests:     []authoringdomain.Test{{Index: 1, Source: authoringdomain.TestManual, InputData: "1"}},
		}}
		running := &authoringdomain.Build{ID: "build-running", State: authoringdomain.BuildRunning}
		builds := &fakeBuilds{build: running, err: authoringdomain.ErrBuildRunning}
		service := newService(packages, builds)
		build, err := service.Build(context.Background(), "problem-1", "user-1")
		Expect(errors.Is(err, authoringdomain.ErrBuildRunning)).To(BeTrue())
		Expect(build).To(Equal(running))
	})
})

var _ = Describe("Service worker fencing", func() {
	It("rejects progress without a complete lease identity", func() {
		service := newService(&fakePackages{}, &fakeBuilds{})
		Expect(service.Progress(context.Background(), authoringdomain.Progress{BuildID: "b"})).
			To(MatchError(authoringdomain.ErrStaleLease))
	})

	It("rejects a result without a complete lease identity", func() {
		service := newService(&fakePackages{}, &fakeBuilds{})
		Expect(service.Complete(context.Background(), authoringdomain.BuildResult{BuildID: "b"}, "testlib")).
			To(MatchError(authoringdomain.ErrStaleLease))
	})

	It("rejects a package upload without a complete lease identity", func() {
		service := newService(&fakePackages{}, &fakeBuilds{})
		_, err := service.UploadPackage(context.Background(), "b", "p", "", "", nil)
		Expect(err).To(MatchError(authoringdomain.ErrStaleLease))
	})

	It("rejects a stale lease before writing an artifact", func() {
		publisher := &fakePublisher{}
		service := newServiceWithPublisher(&fakePackages{},
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
			service := newServiceWithPublisher(&fakePackages{}, builds, publisher)
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
		service := newServiceWithPublisher(&fakePackages{}, builds, publisher)
		upload, err := service.UploadPackage(context.Background(),
			"build-1", "problem-1", "worker-1", "lease-1", []byte("archive"))
		Expect(err).NotTo(HaveOccurred())
		Expect(upload.StoragePath).To(Equal("problem-1/h"))
		Expect(publisher.publishedProblem).To(Equal("problem-1"))
		Expect(builds.recordedProblem).To(Equal("problem-1"))
	})

	It("removes a newly installed artifact when the fenced database write fails", func() {
		root := GinkgoT().TempDir()
		publisher := authoringfiles.NewTestdataPublisher(root)
		builds := &fakeBuilds{targetProblemID: "problem-1", recordErr: errors.New("database failed")}
		service := newServiceWithPublisher(&fakePackages{}, builds, publisher)

		_, err := service.UploadPackage(context.Background(),
			"build-1", "problem-1", "worker-1", "lease-1",
			makePackage(map[string]string{"1.in": "1\n", "1.out": "1\n"}))
		Expect(err).To(MatchError("database failed"))
		artifacts, readErr := os.ReadDir(filepath.Join(root, "problem-1"))
		Expect(readErr).NotTo(HaveOccurred())
		Expect(artifacts).To(BeEmpty())
	})

	It("removes a new artifact when the lease becomes stale during publication", func() {
		publisher := &fakePublisher{}
		builds := &fakeBuilds{targetProblemID: "problem-1", recordErr: authoringdomain.ErrStaleLease}
		service := newServiceWithPublisher(&fakePackages{}, builds, publisher)

		_, err := service.UploadPackage(context.Background(),
			"build-1", "problem-1", "worker-1", "lease-1", []byte("archive"))
		Expect(err).To(MatchError(authoringdomain.ErrStaleLease))
		Expect(publisher.removed).To(Equal([]string{"problem-1/h"}))
	})

	It("does not delete a pre-existing content-addressed artifact after a database failure", func() {
		root := GinkgoT().TempDir()
		publisher := authoringfiles.NewTestdataPublisher(root)
		archive := makePackage(map[string]string{"1.in": "1\n", "1.out": "1\n"})
		existing, err := publisher.Publish("problem-1", archive)
		Expect(err).NotTo(HaveOccurred())
		Expect(existing.Created).To(BeTrue())

		builds := &fakeBuilds{targetProblemID: "problem-1", recordErr: errors.New("database failed")}
		service := newServiceWithPublisher(&fakePackages{}, builds, publisher)
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
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range entries {
		file, err := writer.Create(name)
		Expect(err).NotTo(HaveOccurred())
		_, err = file.Write([]byte(body))
		Expect(err).NotTo(HaveOccurred())
	}
	Expect(writer.Close()).To(Succeed())
	return buffer.Bytes()
}
