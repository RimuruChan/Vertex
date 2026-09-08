package application

import (
	"context"
	"sync"
	"testing"
	"time"

	judgedomain "github.com/RimuruChan/Vertex/server/internal/judge/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service", func() {
	It("waits without busy polling and wakes when a job is queued", func() {
		repository := &fakeRepository{calls: make(chan any, 8)}
		dispatcher := NewDispatcher(1)
		service, err := NewService(repository, dispatcher, 3*time.Second, 2*time.Second)
		Expect(err).NotTo(HaveOccurred())

		result := make(chan *judgedomain.Job, 1)
		go func() {
			defer GinkgoRecover()
			job, claimErr := service.Claim(context.Background(), "worker-1", 2*time.Second)
			Expect(claimErr).NotTo(HaveOccurred())
			result <- job
		}()

		Eventually(repository.calls).Should(Receive(Equal(1)))
		repository.setJob(&judgedomain.Job{ID: "job-1"})
		dispatcher.Notify()
		var received *judgedomain.Job
		Eventually(result).Should(Receive(&received))
		Expect(received.ID).To(Equal("job-1"))
		Expect(repository.callCount()).To(Equal(2))
	})

	It("returns no job at the long-poll deadline without busy polling", func() {
		repository := &fakeRepository{calls: make(chan any, 8)}
		service, err := NewService(repository, NewDispatcher(1), 3*time.Second, 2*time.Second)
		Expect(err).NotTo(HaveOccurred())

		job, err := service.Claim(context.Background(), "worker-1", 20*time.Millisecond)
		Expect(err).NotTo(HaveOccurred())
		Expect(job).To(BeNil())
		// One initial claim plus one final boundary check; no interval polling occurs.
		Expect(repository.callCount()).To(Equal(2))
	})

	It("uses one shared periodic fallback wake-up", func() {
		repository := &fakeRepository{calls: make(chan any, 8)}
		dispatcher := NewDispatcher(1)
		service, err := NewService(repository, dispatcher, 3*time.Second, 2*time.Second)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go dispatcher.RunFallback(ctx, 20*time.Millisecond)

		result := make(chan *judgedomain.Job, 1)
		go func() {
			defer GinkgoRecover()
			job, claimErr := service.Claim(ctx, "worker-1", time.Second)
			Expect(claimErr).NotTo(HaveOccurred())
			result <- job
		}()
		Eventually(repository.calls).Should(Receive(Equal(1)))
		repository.setJob(&judgedomain.Job{ID: "job-fallback"})
		Eventually(result).Should(Receive(Equal(&judgedomain.Job{ID: "job-fallback"})))
		Expect(repository.callCount()).To(Equal(2))
	})

	It("clamps reported progress instead of failing the lease renewal", func() {
		repository := &fakeRepository{calls: make(chan any, 8)}
		service, err := NewService(repository, NewDispatcher(1), 3*time.Second, 2*time.Second)
		Expect(err).NotTo(HaveOccurred())

		Expect(service.Heartbeat(context.Background(), "job-1", 1, "lease-1", "worker-1", -5)).To(Succeed())
		Expect(repository.heartbeat).To(Equal([]any{"job-1", 1, "lease-1", "worker-1", 0}))

		Expect(service.Heartbeat(context.Background(), "job-1", 1, "lease-1", "worker-1", judgedomain.MaxResultCases+1)).To(Succeed())
		Expect(repository.heartbeat[4]).To(Equal(judgedomain.MaxResultCases))
	})

	It("rejects malformed results before persistence", func() {
		repository := &fakeRepository{calls: make(chan any, 8)}
		service, err := NewService(repository, NewDispatcher(1), 3*time.Second, 2*time.Second)
		Expect(err).NotTo(HaveOccurred())
		result := judgedomain.Result{
			JobID: "job-1", SubmissionID: "submission-1", Generation: 1,
			LeaseToken: "lease-1", WorkerID: "worker-1", Status: "Pending",
		}
		Expect(service.Complete(context.Background(), result)).To(MatchError(ContainSubstring("invalid judge result")))
		Expect(repository.completed).To(BeFalse())

		result.Status = "Accepted"
		result.Cases = []judgedomain.CaseResult{
			{CaseIndex: 1, Verdict: "Accepted"},
			{CaseIndex: 1, Verdict: "Accepted"},
		}
		Expect(service.Complete(context.Background(), result)).To(MatchError(ContainSubstring("duplicate case index")))
		Expect(repository.completed).To(BeFalse())

		result.Cases = make([]judgedomain.CaseResult, judgedomain.MaxResultCases+1)
		Expect(service.Complete(context.Background(), result)).To(MatchError(ContainSubstring("case result count exceeds")))
		Expect(repository.completed).To(BeFalse())
	})
})

type fakeRepository struct {
	mu         sync.Mutex
	job        *judgedomain.Job
	claimCalls int
	calls      chan any
	heartbeat  []any
	completed  bool
}

func (f *fakeRepository) Claim(_ context.Context, _ string, _ time.Duration) (*judgedomain.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = make(chan any, 8)
	}
	f.claimCalls++
	f.calls <- f.claimCalls
	job := f.job
	f.job = nil
	return job, nil
}

func (f *fakeRepository) Heartbeat(_ context.Context, jobID string, generation int, leaseToken, workerID string, judgedCases int, _ time.Duration) error {
	f.heartbeat = []any{jobID, generation, leaseToken, workerID, judgedCases}
	return nil
}

func (f *fakeRepository) Complete(context.Context, judgedomain.Result) error {
	f.completed = true
	return nil
}

func (f *fakeRepository) setJob(job *judgedomain.Job) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.job = job
}

func (f *fakeRepository) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.claimCalls
}

func TestService(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Judge service") }
