package domain_test

import (
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/modules/submission/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

func judged() *submissiondomain.Submission {
	return &submissiondomain.Submission{
		ID: "submission-1", Status: "Wrong Answer", Score: 40,
		TotalTimeMs: 120, PeakMemoryKb: 2048, CompileResult: "warning: unused",
		JudgedCases: 5, TotalCases: 10,
		CaseResults: []submissiondomain.CaseResult{{CaseIndex: 1, Verdict: "Accepted"}},
	}
}

var _ = Describe("Redact", func() {
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
	})

	It("hides the verdict entirely at no feedback", func() {
		item := judged()
		submissiondomain.Redact(item, contestdomain.FeedbackNone)
		Expect(item.Status).To(Equal(submissiondomain.HiddenStatus))
		Expect(item.Score).To(Equal(0))
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
