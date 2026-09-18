package domain

import (
	"testing"
	"time"
)

func TestProjectionDoesNotModifyFacts(t *testing.T) {
	at := time.Now()
	facts := SubmissionRecord{Submission: Submission{ID: "internal", PublicID: "1", SourceCode: "secret"}, Judgement: Judgement{
		Status: StatusAccepted, Score: 100, JudgedAt: &at, CaseResults: []CaseResult{{CaseIndex: 1, Verdict: StatusAccepted, CheckerOutput: "private expected output"}},
	}}
	view := Project(facts, Disclosure{Feedback: "none"})
	if view.Status != HiddenStatus || view.Score != 0 || view.SourceCode != "" || len(view.CaseResults) != 0 {
		t.Fatalf("private result leaked: %+v", view)
	}
	if facts.Status != StatusAccepted || facts.Score != 100 || facts.SourceCode != "secret" || len(facts.CaseResults) != 1 {
		t.Fatal("projection changed facts")
	}
	full := Project(facts, Disclosure{ReadSource: true, Feedback: "full"})
	full.CaseResults[0].Verdict = StatusWrongAnswer
	*full.JudgedAt = time.Time{}
	if facts.CaseResults[0].Verdict != StatusAccepted || facts.JudgedAt.IsZero() {
		t.Fatal("projection aliases mutable fact data")
	}
	unknown := Project(facts, Disclosure{Feedback: "unrecognized"})
	if unknown.Status != HiddenStatus || unknown.Score != 0 || unknown.SourceCode != "" {
		t.Fatal("unknown disclosure policy must fail closed")
	}
}
