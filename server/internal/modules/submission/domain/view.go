package domain

import "slices"

// SubmissionView is a viewer-specific result. Persistence accepts facts and
// returns SubmissionRecord; it never accepts a redacted view as a write model.
type SubmissionView SubmissionRecord

// Disclosure separates source access from result feedback and scoreboard freeze.
type Disclosure struct {
	ReadSource bool
	Feedback   string
	Frozen     bool
}

// Project owns all field-level disclosure without mutating stored facts.
func Project(record SubmissionRecord, policy Disclosure) SubmissionView {
	view := SubmissionView(record)
	view.CaseResults = slices.Clone(record.CaseResults)
	if record.JudgedAt != nil {
		at := *record.JudgedAt
		view.JudgedAt = &at
	}
	if record.ContestID != nil {
		id := *record.ContestID
		view.ContestID = &id
	}
	if record.ContestPublicID != nil {
		number := *record.ContestPublicID
		view.ContestPublicID = &number
	}
	if !policy.ReadSource {
		view.SourceCode = ""
		view.CompileResult = ""
	}
	if policy.Frozen || policy.Feedback == "frozen" {
		redact(&view, "frozen")
	} else {
		switch policy.Feedback {
		case "full", "summary", "first_error", "none":
		default:
			policy.Feedback = "none"
		}
		redact(&view, policy.Feedback)
	}
	return view
}
