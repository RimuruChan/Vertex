package config_test

import (
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/config"
	"github.com/RimuruChan/Vertex/worker/internal/run"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parse", func() {
	It("uses process-safe defaults", func() {
		cfg, err := config.Parse(configuredLookup(nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Workers).To(Equal(2))
		Expect(cfg.LongPollTimeout).To(Equal(25 * time.Second))
		Expect(cfg.HTTPTimeout).To(Equal(40 * time.Second))
		Expect(cfg.SandboxPolicy).To(Equal(run.DefaultPolicy()))
		Expect(cfg.JudgeWorkerID).NotTo(BeEmpty())
		Expect(cfg.SandboxInstanceID).To(MatchRegexp(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`))
	})

	It("requires an explicit API token", func() {
		_, err := config.Parse(mapLookup(nil))
		Expect(err).To(MatchError("JUDGE_API_TOKEN must contain at least 32 characters"))
	})

	It("parses worker and sandbox overrides", func() {
		cfg, err := config.Parse(configuredLookup(map[string]string{
			"JUDGE_WORKERS":             "3",
			"JUDGE_WORKER_ID":           "judge-a",
			"JUDGE_LONG_POLL_TIMEOUT":   "20s",
			"JUDGE_HTTP_TIMEOUT":        "30s",
			"SANDBOX_BOX_ID":            "10",
			"SANDBOX_INSTANCE_ID":       "worker-east_1",
			"SANDBOX_INSTANCE_LOCK":     "/locks/worker-east_1.lock",
			"SANDBOX_TIME_OVERSHOOT_MS": "250",
			"SANDBOX_WORKSPACE_BYTES":   "123456",
			"SANDBOX_WORKSPACE_INODES":  "789",
			"SANDBOX_CPUSET":            "0-2,4",
		}))
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Workers).To(Equal(3))
		Expect(cfg.SandboxBoxID).To(Equal(10))
		Expect(cfg.SandboxInstanceID).To(Equal("worker-east_1"))
		Expect(cfg.SandboxInstanceLock).To(Equal("/locks/worker-east_1.lock"))
		Expect(cfg.SandboxPolicy).To(Equal(run.Policy{
			TimeOvershootMs: 250, WorkspaceBytes: 123456, WorkspaceInodes: 789, CPUSet: "0-2,4",
		}))
	})

	DescribeTable("rejects invalid settings",
		func(values map[string]string, message string) {
			_, err := config.Parse(configuredLookup(values))
			Expect(err).To(MatchError(ContainSubstring(message)))
		},
		Entry("worker count", map[string]string{"JUDGE_WORKERS": "0"}, "JUDGE_WORKERS"),
		Entry("box range", map[string]string{"JUDGE_WORKERS": "2", "SANDBOX_BOX_ID": "4095"}, "sandbox box range"),
		Entry("API URL", map[string]string{"JUDGE_API_URL": "postgres://database"}, "JUDGE_API_URL"),
		Entry("fractional long poll", map[string]string{"JUDGE_LONG_POLL_TIMEOUT": "1500ms"}, "whole number"),
		Entry("short HTTP timeout", map[string]string{"JUDGE_HTTP_TIMEOUT": "25s"}, "greater than"),
		Entry("worker ID whitespace", map[string]string{"JUDGE_WORKER_ID": " judge"}, "JUDGE_WORKER_ID"),
		Entry("instance ID traversal", map[string]string{"SANDBOX_INSTANCE_ID": "../worker"}, "SANDBOX_INSTANCE_ID"),
		Entry("instance ID leading punctuation", map[string]string{"SANDBOX_INSTANCE_ID": ".worker"}, "SANDBOX_INSTANCE_ID"),
		Entry("instance ID too long", map[string]string{"SANDBOX_INSTANCE_ID": "a1234567890123456789012345678901234567890123456789012345678901234"}, "SANDBOX_INSTANCE_ID"),
		Entry("negative overshoot", map[string]string{"SANDBOX_TIME_OVERSHOOT_MS": "-1"}, "SANDBOX_TIME_OVERSHOOT_MS"),
		Entry("zero workspace bytes", map[string]string{"SANDBOX_WORKSPACE_BYTES": "0"}, "SANDBOX_WORKSPACE_BYTES"),
		Entry("negative workspace inodes", map[string]string{"SANDBOX_WORKSPACE_INODES": "-1"}, "SANDBOX_WORKSPACE_INODES"),
		Entry("CPU set whitespace", map[string]string{"SANDBOX_CPUSET": " 0-1"}, "SANDBOX_CPUSET"),
	)
})

func configuredLookup(overrides map[string]string) func(string) (string, bool) {
	values := map[string]string{
		"JUDGE_API_TOKEN": "a-judge-token-with-at-least-thirty-two-characters",
	}
	for key, value := range overrides {
		values[key] = value
	}
	return mapLookup(values)
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
