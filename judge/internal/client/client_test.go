package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"time"

	judgeclient "github.com/RimuruChan/Vertex/judge/internal/client"
	"github.com/RimuruChan/Vertex/judge/internal/executor"
	"github.com/RimuruChan/Vertex/judge/internal/scheduler"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const testdataRoot = "testdata-root"

var _ = Describe("Client", func() {
	It("claims a complete immutable job snapshot", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			Expect(request.URL.Path).To(Equal("/jobs/claim"))
			Expect(request.Header.Get("Authorization")).To(Equal("Bearer service-token"))
			var body map[string]any
			Expect(json.NewDecoder(request.Body).Decode(&body)).To(Succeed())
			Expect(body["workerId"]).To(Equal("worker-1"))
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{
				"jobId":"job-1","submissionId":"sub-1","generation":2,"attempt":1,
				"leaseToken":"lease-1","leaseExpiresAt":"2030-01-01T00:00:00Z",
				"language":"cpp","sourceCode":"int main(){}","problemId":"problem-1",
				"timeLimitMs":1000,"memoryLimitKb":262144,
				"testdata":{"storagePath":"problem-1","dataVersion":7,"sha256":"abc123","caseCount":3,"checker":"tokens"}
			}`))
		}))
		defer server.Close()

		client, err := judgeclient.New(server.URL, "service-token", "worker-1", testdataRoot, server.Client(), 25)
		Expect(err).NotTo(HaveOccurred())
		job, err := client.ClaimNext(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(job.JobID).To(Equal("job-1"))
		Expect(job.Generation).To(Equal(2))
		Expect(job.Limits.TimeLimitMs).To(Equal(1000))
		Expect(job.Testdata.CaseCount).To(Equal(3))
		Expect(job.Testdata.Dir).To(Equal(filepath.Join(testdataRoot, "problem-1")))
		Expect(job.Testdata.DataVersion).To(Equal(7))
		Expect(job.Testdata.SHA256).To(Equal("abc123"))
		Expect(job.Testdata.Checker).To(Equal("tokens"))
	})

	It("retries a transient claim failure", func() {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			if attempts.Add(1) == 1 {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{
				"jobId":"job-1","submissionId":"sub-1","generation":1,"attempt":1,
				"leaseToken":"lease-1","leaseExpiresAt":"2030-01-01T00:00:00Z",
				"language":"cpp","problemId":"problem-1","timeLimitMs":1000,"memoryLimitKb":1024,
				"testdata":{"storagePath":"problem-1","caseCount":1}
			}`))
		}))
		defer server.Close()
		client, err := judgeclient.New(server.URL, "service-token", "worker-1", testdataRoot, server.Client(), 25)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		job, err := client.ClaimNext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(job).NotTo(BeNil())
		Expect(attempts.Load()).To(Equal(int32(2)))
	})

	It("does not retry a permanent claim rejection", func() {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			attempts.Add(1)
			writer.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()
		client, err := judgeclient.New(server.URL, "service-token", "worker-1", testdataRoot, server.Client(), 25)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.ClaimNext(context.Background())
		Expect(err).To(MatchError("claim returned HTTP 401"))
		Expect(attempts.Load()).To(Equal(int32(1)))
	})

	It("stops claim retries when the context is canceled", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()
		client, err := judgeclient.New(server.URL, "service-token", "worker-1", testdataRoot, server.Client(), 25)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		_, err = client.ClaimNext(ctx)
		Expect(err).To(MatchError(context.DeadlineExceeded))
	})

	It("maps an empty long poll to no job", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()
		client, err := judgeclient.New(server.URL, "service-token", "worker-1", testdataRoot, server.Client(), 25)
		Expect(err).NotTo(HaveOccurred())
		job, err := client.ClaimNext(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(job).To(BeNil())
	})

	It("maps a stale heartbeat to lease loss", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusConflict)
		}))
		defer server.Close()
		client, err := judgeclient.New(server.URL, "service-token", "worker-1", testdataRoot, server.Client(), 25)
		Expect(err).NotTo(HaveOccurred())
		err = client.Heartbeat(context.Background(), &scheduler.Submission{JobID: "job-1", Generation: 1, LeaseToken: "lease-1"})
		Expect(errors.Is(err, scheduler.ErrLeaseLost)).To(BeTrue())
	})

	It("retries a transient heartbeat failure", func() {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			if attempts.Add(1) == 1 {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()
		client, err := judgeclient.New(server.URL, "service-token", "worker-1", testdataRoot, server.Client(), 25)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		Expect(client.Heartbeat(ctx, &scheduler.Submission{
			JobID: "job-1", Generation: 1, LeaseToken: "lease-1",
		})).To(Succeed())
		Expect(attempts.Load()).To(Equal(int32(2)))
	})

	It("does not retry a permanent heartbeat rejection", func() {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			attempts.Add(1)
			writer.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()
		client, err := judgeclient.New(server.URL, "service-token", "worker-1", testdataRoot, server.Client(), 25)
		Expect(err).NotTo(HaveOccurred())
		err = client.Heartbeat(context.Background(), &scheduler.Submission{
			JobID: "job-1", Generation: 1, LeaseToken: "lease-1",
		})
		Expect(err).To(MatchError("heartbeat returned HTTP 401"))
		Expect(attempts.Load()).To(Equal(int32(1)))
	})

	It("retries transient result failures without changing the idempotency fields", func() {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			var body map[string]any
			Expect(json.NewDecoder(request.Body).Decode(&body)).To(Succeed())
			Expect(body["generation"]).To(Equal(float64(3)))
			Expect(body["leaseToken"]).To(Equal("lease-3"))
			if attempts.Add(1) == 1 {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()
		client, err := judgeclient.New(server.URL, "service-token", "worker-1", testdataRoot, server.Client(), 25)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err = client.MarkResult(ctx,
			&scheduler.Submission{JobID: "job-3", ID: "sub-3", Generation: 3, LeaseToken: "lease-3"},
			"Accepted", 100, 5, 1024, "", []executor.CaseResult{{CaseIndex: 1, Verdict: "Accepted"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(attempts.Load()).To(Equal(int32(2)))
	})

	It("maps a stale result to lease loss without retrying", func() {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			attempts.Add(1)
			writer.WriteHeader(http.StatusConflict)
		}))
		defer server.Close()
		client, err := judgeclient.New(server.URL, "service-token", "worker-1", testdataRoot, server.Client(), 25)
		Expect(err).NotTo(HaveOccurred())
		err = client.MarkResult(context.Background(),
			&scheduler.Submission{JobID: "job-1", ID: "sub-1", Generation: 1, LeaseToken: "lease-1"},
			"Accepted", 100, 1, 1024, "", nil)
		Expect(errors.Is(err, scheduler.ErrLeaseLost)).To(BeTrue())
		Expect(attempts.Load()).To(Equal(int32(1)))
	})

	It("stops result retries when the context is canceled", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()
		client, err := judgeclient.New(server.URL, "service-token", "worker-1", testdataRoot, server.Client(), 25)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		err = client.MarkResult(ctx,
			&scheduler.Submission{JobID: "job-1", ID: "sub-1", Generation: 1, LeaseToken: "lease-1"},
			"Accepted", 100, 1, 1024, "", nil)
		Expect(err).To(MatchError(context.DeadlineExceeded))
	})

	It("rejects testdata paths outside the mounted root", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{
				"jobId":"job-1","submissionId":"sub-1","generation":1,"attempt":1,
				"leaseToken":"lease-1","leaseExpiresAt":"2030-01-01T00:00:00Z",
				"language":"cpp","problemId":"problem-1","timeLimitMs":1000,"memoryLimitKb":1024,
				"testdata":{"storagePath":"../secret","caseCount":1}
			}`))
		}))
		defer server.Close()
		client, err := judgeclient.New(server.URL, "service-token", "worker-1", testdataRoot, server.Client(), 25)
		Expect(err).NotTo(HaveOccurred())
		_, err = client.ClaimNext(context.Background())
		Expect(err).To(MatchError(ContainSubstring("escapes configured root")))
	})
})
