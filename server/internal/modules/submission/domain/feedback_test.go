package domain_test

import (
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/modules/submission/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
	"time"
)

func judged() *submissiondomain.Submission {
	now := time.Now()
	return &submissiondomain.Submission{
		JudgedAt: &now,
		ID:       "submission-1", Status: "Wrong Answer", Score: 40,
		TotalTimeMs: 120, PeakMemoryKb: 2048, CompileResult: "warning: unused",
		JudgedCases: 5, TotalCases: 10,
		CaseResults: []submissiondomain.CaseResult{{CaseIndex: 1, Verdict: "Accepted"}},
	}
}

var _ = Describe("Redact", func() {
	It("returns only the earliest failed case number and verdict", func() {
		item := judged()
		item.CaseResults = []submissiondomain.CaseResult{
			{CaseIndex: 8, Verdict: "Wrong Answer", CheckerOutput: "secret expected answer"},
			{CaseIndex: 1, Verdict: "Accepted"},
			{CaseIndex: 3, Verdict: "Time Limit Exceeded", TimeMs: 1000, MemoryKb: 1234, ExitStatus: "secret"},
		}
		submissiondomain.Redact(item, contestdomain.FeedbackFirstError)
		Expect(item.CaseResults).To(Equal([]submissiondomain.CaseResult{{CaseIndex: 3, Verdict: "Time Limit Exceeded"}}))
		Expect(item.TotalCases).To(BeZero())
		Expect(item.JudgedCases).To(BeZero())
		Expect(item.TotalTimeMs).To(BeZero())
		Expect(item.CompileResult).To(BeEmpty())
	})
	It("withholds case results while judging but preserves compile diagnostics for CE", func() {
		item := judged()
		item.Status = "Judging"
		item.CaseResults = []submissiondomain.CaseResult{{CaseIndex: 2, Verdict: "Wrong Answer"}}
		submissiondomain.Redact(item, contestdomain.FeedbackFirstError)
		Expect(item.CaseResults).To(BeEmpty())
		item.Status = "Compile Error"
		item.CompileResult = "compiler diagnostic"
		submissiondomain.Redact(item, contestdomain.FeedbackFirstError)
		Expect(item.CompileResult).To(Equal("compiler diagnostic"))
	})
	It("projects a frozen peer result as Pending without leaking completion or code", func() {
		item := judged()
		item.SourceCode = "secret"
		submissiondomain.Redact(item, "frozen")
		Expect(item.Status).To(Equal("Pending"))
		Expect(item.Score).To(BeZero())
		Expect(item.TotalTimeMs).To(BeZero())
		Expect(item.PeakMemoryKb).To(BeZero())
		Expect(item.JudgedAt).To(BeNil())
		Expect(item.JudgedCases).To(BeZero())
		Expect(item.TotalCases).To(BeZero())
		Expect(item.CaseResults).To(BeNil())
		Expect(item.CompileResult).To(BeEmpty())
		Expect(item.SourceCode).To(BeEmpty())
	})
	It("leaves everything intact at full feedback", func() {
		item := judged()
		submissiondomain.Redact(item, contestdomain.FeedbackFull)
		Expect(item.Status).To(Equal("Wrong Answer"))
		Expect(item.CaseResults).To(HaveLen(1))
		Expect(item.TotalTimeMs).To(Equal(120))
	})

	It("keeps the verdict but drops test detail at summary feedback", func() {
		item := judged()
		submissiondomain.Redact(item, contestdomain.FeedbackSummary)
		Expect(item.Status).To(Equal("Wrong Answer"))
		Expect(item.CaseResults).To(BeEmpty())
		Expect(item.CompileResult).To(BeEmpty())
		Expect(item.TotalTimeMs).To(Equal(0))
		Expect(item.PeakMemoryKb).To(Equal(0))
		// Progress counters would leak how far the hidden test set got.
		Expect(item.TotalCases).To(Equal(0))
		Expect(item.JudgedAt).To(BeNil())
	})

	It("hides the verdict entirely at no feedback", func() {
		item := judged()
		submissiondomain.Redact(item, contestdomain.FeedbackNone)
		Expect(item.Status).To(Equal(submissiondomain.HiddenStatus))
		Expect(item.Score).To(Equal(0))
		Expect(item.JudgedAt).To(BeNil())
		Expect(item.CaseResults).To(BeEmpty())
	})

	It("still shows a queued submission as queued at no feedback", func() {
		// A contestant must be able to tell their submission arrived, even in
		// a contest that reveals nothing about the result.
		for _, status := range []string{"Pending", "Judging"} {
			item := judged()
			item.Status = status
			submissiondomain.Redact(item, contestdomain.FeedbackNone)
			Expect(item.Status).To(Equal(status))
		}
	})

	It("never removes the identity of the submission itself", func() {
		item := judged()
		item.SourceCode = "int main(){}"
		submissiondomain.Redact(item, contestdomain.FeedbackNone)
		Expect(item.ID).To(Equal("submission-1"))
		Expect(item.SourceCode).To(Equal("int main(){}"))
	})
})

func TestFeedback(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Submission Feedback") }
