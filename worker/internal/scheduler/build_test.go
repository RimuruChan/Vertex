package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/builder"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeBuildClient records the protocol calls a build makes so the specs can
// assert on fencing behavior instead of on HTTP.
type fakeBuildClient struct {
	mutex sync.Mutex

	claims      int
	job         *builder.Job
	progressErr error
	uploadErr   error
	completeErr error

	uploaded  [][]byte
	completed []*builder.Report
	logs      []string
}

func (c *fakeBuildClient) ClaimBuild(context.Context) (*builder.Job, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.claims++
	if c.claims > 1 {
		return nil, nil
	}
	return c.job, nil
}

func (c *fakeBuildClient) ReportBuildProgress(_ context.Context, _ *builder.Job, _ string, _, _ int, _ string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.progressErr
}

func (c *fakeBuildClient) UploadBuildPackage(_ context.Context, _ *builder.Job, archive []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.uploaded = append(c.uploaded, archive)
	return c.uploadErr
}

func (c *fakeBuildClient) CompleteBuild(_ context.Context, _ *builder.Job, report *builder.Report, log string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.completed = append(c.completed, report)
	c.logs = append(c.logs, log)
	return c.completeErr
}

func (c *fakeBuildClient) results() []*builder.Report {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return append([]*builder.Report(nil), c.completed...)
}

type fakePackageBuilder struct {
	report *builder.Report
	err    error
	// block delays the build so a progress tick can fire first.
	block time.Duration
}

func (b *fakePackageBuilder) Build(ctx context.Context, _ *builder.Job, reporter builder.Reporter) (*builder.Report, error) {
	reporter.Logf("building")
	if b.block > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(b.block):
		}
	}
	return b.report, b.err
}

func newBuildJob() *builder.Job {
	return &builder.Job{
		BuildID: "build-1", ProblemID: "problem-1", LeaseToken: "lease-1",
		LeaseExpires: time.Now().Add(time.Minute),
	}
}

var _ = Describe("BuildScheduler", func() {
	It("uploads the archive before reporting a successful build", func() {
		client := &fakeBuildClient{job: newBuildJob()}
		packageBuilder := &fakePackageBuilder{report: &builder.Report{
			Success: true, Stage: "package", Checker: "testlib", Archive: []byte("zip"),
		}}
		scheduler := NewBuildScheduler(client, packageBuilder, "worker-1", time.Hour)

		scheduler.buildOne(context.Background(), newBuildJob())

		Expect(client.uploaded).To(HaveLen(1))
		Expect(client.uploaded[0]).To(Equal([]byte("zip")))
		Expect(client.results()).To(HaveLen(1))
		Expect(client.results()[0].Success).To(BeTrue())
	})

	It("does not upload anything for a failed build", func() {
		client := &fakeBuildClient{job: newBuildJob()}
		packageBuilder := &fakePackageBuilder{report: &builder.Report{
			Stage: "generate", ErrorMessage: "生成失败",
		}}
		scheduler := NewBuildScheduler(client, packageBuilder, "worker-1", time.Hour)

		scheduler.buildOne(context.Background(), newBuildJob())

		Expect(client.uploaded).To(BeEmpty())
		Expect(client.results()).To(HaveLen(1))
		Expect(client.results()[0].Success).To(BeFalse())
	})

	It("reports a failure when the artifact cannot be uploaded", func() {
		client := &fakeBuildClient{job: newBuildJob(), uploadErr: errors.New("disk full")}
		packageBuilder := &fakePackageBuilder{report: &builder.Report{
			Success: true, Archive: []byte("zip"),
		}}
		scheduler := NewBuildScheduler(client, packageBuilder, "worker-1", time.Hour)

		scheduler.buildOne(context.Background(), newBuildJob())

		Expect(client.results()).To(HaveLen(1))
		Expect(client.results()[0].Success).To(BeFalse())
		Expect(client.results()[0].ErrorMessage).To(ContainSubstring("disk full"))
	})

	It("discards a result whose lease was lost mid-build", func() {
		client := &fakeBuildClient{job: newBuildJob(), progressErr: ErrLeaseLost}
		packageBuilder := &fakePackageBuilder{
			report: &builder.Report{Success: true, Archive: []byte("zip")},
			block:  2 * time.Second,
		}
		scheduler := NewBuildScheduler(client, packageBuilder, "worker-1", 10*time.Millisecond)

		scheduler.buildOne(context.Background(), newBuildJob())

		Expect(client.uploaded).To(BeEmpty())
		Expect(client.results()).To(BeEmpty())
	})

	It("still reports a terminal result when the builder itself fails", func() {
		client := &fakeBuildClient{job: newBuildJob()}
		packageBuilder := &fakePackageBuilder{err: errors.New("workspace unavailable")}
		scheduler := NewBuildScheduler(client, packageBuilder, "worker-1", time.Hour)

		scheduler.buildOne(context.Background(), newBuildJob())

		Expect(client.results()).To(HaveLen(1))
		Expect(client.results()[0].Success).To(BeFalse())
		Expect(client.results()[0].ErrorMessage).To(ContainSubstring("workspace unavailable"))
	})
})

var _ = Describe("progressTracker", func() {
	It("returns each log line exactly once", func() {
		tracker := newProgressTracker()
		tracker.Logf("first %d", 1)
		tracker.Logf("second")

		Expect(tracker.drainLog()).To(Equal("first 1\nsecond\n"))
		Expect(tracker.drainLog()).To(BeEmpty())
	})

	It("reports the most recent stage", func() {
		tracker := newProgressTracker()
		tracker.Stage("generate", 3, 10)
		stage, done, total := tracker.snapshot()
		Expect(stage).To(Equal("generate"))
		Expect(done).To(Equal(3))
		Expect(total).To(Equal(10))
	})

	It("stops buffering once the log grows past its bound", func() {
		tracker := newProgressTracker()
		for i := 0; i < 10000; i++ {
			tracker.Logf("a line of build output that repeats many times")
		}
		Expect(len(tracker.drainLog())).To(BeNumerically("<", 2*maxTrackedLogBytes))
	})
})
