package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/executor"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Scheduler lease lifecycle", func() {
	It("opens the next claim immediately after an empty long poll", func() {
		ctx, cancel := context.WithCancel(context.Background())
		client := &fakeJobClient{claim: func() (*Submission, error) {
			return nil, nil
		}}
		client.afterClaim = func(count int32) {
			if count == 2 {
				cancel()
			}
		}
		scheduler := New(client, "worker-1", []WorkerRuntime{{}})

		done := make(chan struct{})
		go func() {
			scheduler.loop(ctx, 0, &scheduler.workers[0])
			close(done)
		}()

		Eventually(done).Should(BeClosed())
		Expect(client.claims.Load()).To(BeNumerically(">=", 2))
	})

	It("cancels active judging when the server rejects the lease", func() {
		client := &fakeJobClient{heartbeatErr: ErrLeaseLost}
		scheduler := New(client, "worker-1", []WorkerRuntime{{}})
		ctx, cancel := context.WithCancel(context.Background())
		leaseLost := make(chan struct{}, 1)
		done := make(chan struct{})
		var judgedCases atomic.Int64
		go func() {
			scheduler.heartbeatLoop(ctx, &Submission{
				JobID: "job-1", ID: "submission-1", Generation: 2,
				LeaseUntil: time.Now().Add(3 * time.Second),
			}, &judgedCases, leaseLost, cancel)
			close(done)
		}()

		Eventually(leaseLost, 2*time.Second).Should(Receive())
		Eventually(ctx.Done()).Should(BeClosed())
		Eventually(done).Should(BeClosed())
		Expect(client.heartbeats.Load()).To(Equal(int32(1)))
	})

	It("reports the current case progress on every heartbeat", func() {
		client := &fakeJobClient{}
		scheduler := New(client, "worker-1", []WorkerRuntime{{}})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		leaseLost := make(chan struct{}, 1)
		done := make(chan struct{})
		var judgedCases atomic.Int64
		judgedCases.Store(4)
		go func() {
			scheduler.heartbeatLoop(ctx, &Submission{
				JobID: "job-1", ID: "submission-1", Generation: 1,
				LeaseUntil: time.Now().Add(3 * time.Second),
			}, &judgedCases, leaseLost, cancel)
			close(done)
		}()

		Eventually(func() int { return int(client.lastProgress.Load()) }, 2*time.Second).Should(Equal(4))
		cancel()
		Eventually(done).Should(BeClosed())
	})
})

type fakeJobClient struct {
	claim        func() (*Submission, error)
	afterClaim   func(int32)
	heartbeatErr error
	claims       atomic.Int32
	heartbeats   atomic.Int32
	lastProgress atomic.Int32
}

func (f *fakeJobClient) ClaimNext(context.Context) (*Submission, error) {
	count := f.claims.Add(1)
	if f.afterClaim != nil {
		f.afterClaim(count)
	}
	if f.claim == nil {
		return nil, nil
	}
	return f.claim()
}

func (f *fakeJobClient) Heartbeat(_ context.Context, _ *Submission, judgedCases int) error {
	f.heartbeats.Add(1)
	f.lastProgress.Store(int32(judgedCases))
	return f.heartbeatErr
}

func (f *fakeJobClient) MarkResult(
	context.Context, *Submission, string, int, int64, int, string, []executor.CaseResult,
) error {
	return errors.New("unexpected result write")
}
