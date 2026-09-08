package domain

// ForRedaction adapts the polling projection to the existing
// feedback policy so list, detail and progress cannot drift in what they hide.
func (p *SubmissionProgress) ForRedaction() Submission {
	return Submission{
		ID: p.ID, UserID: p.UserID, ContestID: p.ContestID,
		Status: p.Status, Score: p.Score,
		TotalTimeMs: p.TotalTimeMs, PeakMemoryKb: p.PeakMemoryKb,
		CompileResult: p.CompileResult, CaseResults: p.CaseResults,
		JudgedCases: p.JudgedCases, TotalCases: p.TotalCases,
	}
}

func (p *SubmissionProgress) ApplyRedaction(value Submission) {
	p.Status = value.Status
	p.Score = value.Score
	p.TotalTimeMs = value.TotalTimeMs
	p.PeakMemoryKb = value.PeakMemoryKb
	p.CompileResult = value.CompileResult
	p.CaseResults = value.CaseResults
	p.JudgedCases = value.JudgedCases
	p.TotalCases = value.TotalCases
}
