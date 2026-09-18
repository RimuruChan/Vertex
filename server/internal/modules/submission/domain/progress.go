package domain

// ProgressView contains only information safe for polling clients.
type ProgressView struct {
	ID            string
	Status        string
	Score         int
	TotalTimeMs   int
	PeakMemoryKb  int
	CompileResult string
	CaseResults   []CaseResult
	JudgedCases   int
	TotalCases    int
}

func (p SubmissionProgress) Record() SubmissionRecord {
	return SubmissionRecord{AsOf: p.AsOf,
		Submission:   Submission{ID: p.ID, PublicID: p.PublicID, UserID: p.UserID, ContestID: p.ContestID},
		FrozenResult: p.FrozenResult, CanReadSource: p.CanReadSource, Judgement: Judgement{Status: p.Status, Score: p.Score, TotalTimeMs: p.TotalTimeMs, PeakMemoryKb: p.PeakMemoryKb,
			CompileResult: p.CompileResult, CaseResults: p.CaseResults, JudgedCases: p.JudgedCases, TotalCases: p.TotalCases},
	}
}

func ProgressFromView(view SubmissionView) ProgressView {
	return ProgressView{ID: view.PublicID, Status: view.Status, Score: view.Score, TotalTimeMs: view.TotalTimeMs,
		PeakMemoryKb: view.PeakMemoryKb, CompileResult: view.CompileResult, CaseResults: view.CaseResults,
		JudgedCases: view.JudgedCases, TotalCases: view.TotalCases}
}
