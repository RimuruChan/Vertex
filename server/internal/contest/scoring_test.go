package contest_test

import (
	"time"

	contestapp "github.com/RimuruChan/Vertex/server/internal/contest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var contestStart = time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)

// at returns a submission time given as minutes from the contest start.
func at(minutes int) time.Time {
	return contestStart.Add(time.Duration(minutes) * time.Minute)
}

func icpcRules() contestapp.ScoringRules {
	return contestapp.ScoringRules{
		Format: contestapp.FormatICPC, BeginAt: contestStart, EndAt: at(300),
		PenaltyMinutes: 20, PenalizeCompileError: true, MaxPoints: 100,
	}
}

func submission(minutes int, status string, score int) contestapp.ScoredSubmission {
	return contestapp.ScoredSubmission{SubmittedAt: at(minutes), Status: status, Score: score}
}

var _ = Describe("ScoreCell ICPC", func() {
	It("charges solve time plus a penalty for each earlier rejection", func() {
		cell := contestapp.ScoreCell(icpcRules(), []contestapp.ScoredSubmission{
			submission(10, "Wrong Answer", 0),
			submission(25, "Time Limit Exceeded", 0),
			submission(40, "Accepted", 100),
		})
		Expect(cell.Solved()).To(BeTrue())
		Expect(cell.Attempts).To(Equal(3))
		// 40 minutes to solve + two rejected runs * 20 minutes.
		Expect(cell.PenaltySec).To(Equal((40 + 2*20) * 60))
	})

	It("ignores everything submitted after the first accepted run", func() {
		cell := contestapp.ScoreCell(icpcRules(), []contestapp.ScoredSubmission{
			submission(30, "Accepted", 100),
			submission(45, "Wrong Answer", 0),
			submission(50, "Wrong Answer", 0),
		})
		Expect(cell.Attempts).To(Equal(1))
		Expect(cell.PenaltySec).To(Equal(30 * 60))
	})

	It("gives an unsolved problem attempts but no penalty", func() {
		cell := contestapp.ScoreCell(icpcRules(), []contestapp.ScoredSubmission{
			submission(10, "Wrong Answer", 0),
			submission(20, "Wrong Answer", 0),
		})
		Expect(cell.Solved()).To(BeFalse())
		Expect(cell.Attempts).To(Equal(2))
		Expect(cell.PenaltySec).To(Equal(0))
	})

	It("scores an accepted first attempt with solve time only", func() {
		cell := contestapp.ScoreCell(icpcRules(), []contestapp.ScoredSubmission{
			submission(7, "Accepted", 100),
		})
		Expect(cell.PenaltySec).To(Equal(7 * 60))
	})

	It("honours a contest that does not penalize compile errors", func() {
		rules := icpcRules()
		rules.PenalizeCompileError = false
		cell := contestapp.ScoreCell(rules, []contestapp.ScoredSubmission{
			submission(10, "Compile Error", 0),
			submission(20, "Accepted", 100),
		})
		Expect(cell.Attempts).To(Equal(1))
		Expect(cell.PenaltySec).To(Equal(20 * 60))
	})

	It("counts compile errors when the contest penalizes them", func() {
		cell := contestapp.ScoreCell(icpcRules(), []contestapp.ScoredSubmission{
			submission(10, "Compile Error", 0),
			submission(20, "Accepted", 100),
		})
		Expect(cell.Attempts).To(Equal(2))
		Expect(cell.PenaltySec).To(Equal((20 + 20) * 60))
	})

	It("respects a custom penalty value", func() {
		rules := icpcRules()
		rules.PenaltyMinutes = 5
		cell := contestapp.ScoreCell(rules, []contestapp.ScoredSubmission{
			submission(10, "Wrong Answer", 0),
			submission(30, "Accepted", 100),
		})
		Expect(cell.PenaltySec).To(Equal((30 + 5) * 60))
	})

	It("never counts submissions outside the contest window", func() {
		cell := contestapp.ScoreCell(icpcRules(), []contestapp.ScoredSubmission{
			{SubmittedAt: contestStart.Add(-time.Minute), Status: "Accepted", Score: 100},
			submission(400, "Accepted", 100),
		})
		Expect(cell.Attempts).To(Equal(0))
		Expect(cell.Solved()).To(BeFalse())
	})

	It("never counts unjudged or system-error submissions", func() {
		cell := contestapp.ScoreCell(icpcRules(), []contestapp.ScoredSubmission{
			submission(5, "Pending", 0),
			submission(6, "Judging", 0),
			submission(7, "System Error", 0),
			submission(8, "Accepted", 100),
		})
		Expect(cell.Attempts).To(Equal(1))
		Expect(cell.PenaltySec).To(Equal(8 * 60))
	})

	It("orders submissions by time regardless of input order", func() {
		cell := contestapp.ScoreCell(icpcRules(), []contestapp.ScoredSubmission{
			submission(40, "Accepted", 100),
			submission(10, "Wrong Answer", 0),
		})
		Expect(cell.Attempts).To(Equal(2))
		Expect(cell.PenaltySec).To(Equal((40 + 20) * 60))
	})
})

var _ = Describe("ScoreCell freeze split", func() {
	frozenRules := func() contestapp.ScoringRules {
		rules := icpcRules()
		freeze := at(240)
		rules.FreezeAt = &freeze
		return rules
	}

	It("keeps the jury view complete and hides the post-freeze solve", func() {
		cell := contestapp.ScoreCell(frozenRules(), []contestapp.ScoredSubmission{
			submission(100, "Wrong Answer", 0),
			submission(250, "Accepted", 100),
		})
		Expect(cell.Solved()).To(BeTrue())
		Expect(cell.PenaltySec).To(Equal((250 + 20) * 60))

		Expect(cell.PublicSolvedAt).To(BeNil())
		Expect(cell.PublicAttempts).To(Equal(1))
		Expect(cell.PendingCount).To(Equal(1))
	})

	It("leaves a pre-freeze solve visible on the public board", func() {
		cell := contestapp.ScoreCell(frozenRules(), []contestapp.ScoredSubmission{
			submission(100, "Accepted", 100),
		})
		Expect(cell.PublicSolvedAt).NotTo(BeNil())
		Expect(cell.PublicPenaltySec).To(Equal(100 * 60))
		Expect(cell.PendingCount).To(Equal(0))
	})

	It("counts every post-freeze submission as pending", func() {
		cell := contestapp.ScoreCell(frozenRules(), []contestapp.ScoredSubmission{
			submission(250, "Wrong Answer", 0),
			submission(260, "Wrong Answer", 0),
			submission(270, "Accepted", 100),
		})
		Expect(cell.PendingCount).To(Equal(3))
		Expect(cell.PublicAttempts).To(Equal(0))
	})

	It("makes both views identical when the contest has no freeze", func() {
		cell := contestapp.ScoreCell(icpcRules(), []contestapp.ScoredSubmission{
			submission(250, "Accepted", 100),
		})
		Expect(cell.PublicSolvedAt).To(Equal(cell.SolvedAt))
		Expect(cell.PublicPenaltySec).To(Equal(cell.PenaltySec))
		Expect(cell.PendingCount).To(Equal(0))
	})
})

var _ = Describe("ScoreCell IOI", func() {
	ioiRules := func() contestapp.ScoringRules {
		rules := icpcRules()
		rules.Format = contestapp.FormatIOI
		return rules
	}

	It("keeps the best score even when a later submission is worse", func() {
		cell := contestapp.ScoreCell(ioiRules(), []contestapp.ScoredSubmission{
			submission(10, "Wrong Answer", 60),
			submission(20, "Wrong Answer", 30),
		})
		Expect(cell.Score).To(Equal(60))
		Expect(cell.Attempts).To(Equal(2))
		Expect(cell.PenaltySec).To(Equal(0))
	})

	It("marks a full score as solved at the moment it was first reached", func() {
		cell := contestapp.ScoreCell(ioiRules(), []contestapp.ScoredSubmission{
			submission(10, "Wrong Answer", 40),
			submission(20, "Accepted", 100),
			submission(30, "Wrong Answer", 10),
		})
		Expect(cell.Score).To(Equal(100))
		Expect(cell.SolvedAt).NotTo(BeNil())
		Expect(*cell.SolvedAt).To(Equal(at(20)))
	})

	It("scales a partial judge score onto the problem's point value", func() {
		rules := ioiRules()
		rules.MaxPoints = 50
		cell := contestapp.ScoreCell(rules, []contestapp.ScoredSubmission{
			submission(10, "Wrong Answer", 60),
		})
		// 60 of 100 on a 50-point problem, rounded toward zero.
		Expect(cell.Score).To(Equal(30))
		Expect(cell.SolvedAt).To(BeNil())
	})

	It("never inflates a partial result into a solve", func() {
		rules := ioiRules()
		rules.MaxPoints = 3
		cell := contestapp.ScoreCell(rules, []contestapp.ScoredSubmission{
			submission(10, "Wrong Answer", 99),
		})
		Expect(cell.Score).To(Equal(2))
		Expect(cell.SolvedAt).To(BeNil())
	})
})

var _ = Describe("ScoreCell OI", func() {
	oiRules := func() contestapp.ScoringRules {
		rules := icpcRules()
		rules.Format = contestapp.FormatOI
		return rules
	}

	It("scores only the last submission, even when it is worse", func() {
		cell := contestapp.ScoreCell(oiRules(), []contestapp.ScoredSubmission{
			submission(10, "Accepted", 100),
			submission(20, "Wrong Answer", 30),
		})
		Expect(cell.Score).To(Equal(30))
		Expect(cell.SolvedAt).To(BeNil())
		Expect(cell.Attempts).To(Equal(2))
	})

	It("treats a final full score as solved", func() {
		cell := contestapp.ScoreCell(oiRules(), []contestapp.ScoredSubmission{
			submission(10, "Wrong Answer", 30),
			submission(20, "Accepted", 100),
		})
		Expect(cell.Score).To(Equal(100))
		Expect(cell.SolvedAt).NotTo(BeNil())
	})

	It("charges no penalty time", func() {
		cell := contestapp.ScoreCell(oiRules(), []contestapp.ScoredSubmission{
			submission(10, "Wrong Answer", 0),
			submission(200, "Accepted", 100),
		})
		Expect(cell.PenaltySec).To(Equal(0))
	})
})

var _ = Describe("Totals", func() {
	It("sums the jury view when asked for it", func() {
		solved := at(30)
		cells := []contestapp.Cell{
			{Score: 100, PenaltySec: 1800, SolvedAt: &solved, PublicScore: 0},
			{Score: 40, PenaltySec: 0},
		}
		totals := contestapp.Totals(cells, true)
		Expect(totals.Solved).To(Equal(1))
		Expect(totals.Score).To(Equal(140))
		Expect(totals.PenaltySec).To(Equal(1800))
	})

	It("sums the public view for a frozen board", func() {
		solved := at(30)
		cells := []contestapp.Cell{
			{Score: 100, PenaltySec: 1800, SolvedAt: &solved, PublicScore: 0, PendingCount: 1},
		}
		totals := contestapp.Totals(cells, false)
		Expect(totals.Solved).To(Equal(0))
		Expect(totals.Score).To(Equal(0))
		Expect(totals.PenaltySec).To(Equal(0))
	})

	It("tracks the latest accepted time for score tie-breaking", func() {
		early, late := at(10), at(90)
		totals := contestapp.Totals([]contestapp.Cell{
			{Score: 100, SolvedAt: &early},
			{Score: 100, SolvedAt: &late},
		}, true)
		Expect(totals.LastAcceptedAt).NotTo(BeNil())
		Expect(*totals.LastAcceptedAt).To(Equal(at(90)))
	})
})

var _ = Describe("AssignRanks", func() {
	It("ranks ICPC by solved count then penalty", func() {
		rows := []contestapp.RankRow{
			{Username: "carol", Solved: 2, Penalty: 100},
			{Username: "alice", Solved: 3, Penalty: 900},
			{Username: "bob", Solved: 2, Penalty: 50},
		}
		contestapp.AssignRanks(contestapp.FormatICPC, rows)
		Expect([]string{rows[0].Username, rows[1].Username, rows[2].Username}).
			To(Equal([]string{"alice", "bob", "carol"}))
		Expect(rows[0].Rank).To(Equal(1))
	})

	It("gives tied ICPC rows the same rank and skips the next number", func() {
		rows := []contestapp.RankRow{
			{Username: "alice", Solved: 1, Penalty: 60},
			{Username: "bob", Solved: 1, Penalty: 60},
			{Username: "carol", Solved: 0},
		}
		contestapp.AssignRanks(contestapp.FormatICPC, rows)
		Expect(rows[0].Rank).To(Equal(1))
		Expect(rows[1].Rank).To(Equal(1))
		Expect(rows[2].Rank).To(Equal(3))
	})

	It("ranks IOI by score and breaks ties on who got there first", func() {
		early, late := at(10), at(90)
		rows := []contestapp.RankRow{
			{Username: "alice", Score: 150, LastAcceptedAt: &late},
			{Username: "bob", Score: 150, LastAcceptedAt: &early},
			{Username: "carol", Score: 200},
		}
		contestapp.AssignRanks(contestapp.FormatIOI, rows)
		Expect(rows[0].Username).To(Equal("carol"))
		Expect(rows[1].Username).To(Equal("bob"))
		Expect(rows[2].Username).To(Equal("alice"))
		// Equal scores share a rank even though their times differ.
		Expect(rows[1].Rank).To(Equal(2))
		Expect(rows[2].Rank).To(Equal(2))
	})

	It("sorts a contestant with no accepted submission last among equals", func() {
		solved := at(10)
		rows := []contestapp.RankRow{
			{Username: "alice", Score: 50},
			{Username: "bob", Score: 50, LastAcceptedAt: &solved},
		}
		contestapp.AssignRanks(contestapp.FormatIOI, rows)
		Expect(rows[0].Username).To(Equal("bob"))
	})

	It("falls back to username so the order is deterministic", func() {
		rows := []contestapp.RankRow{
			{Username: "bob", Solved: 1, Penalty: 60},
			{Username: "alice", Solved: 1, Penalty: 60},
		}
		contestapp.AssignRanks(contestapp.FormatICPC, rows)
		Expect(rows[0].Username).To(Equal("alice"))
	})
})

var _ = Describe("NormalizeFormat", func() {
	It("maps the legacy acm rule onto icpc", func() {
		Expect(contestapp.NormalizeFormat("acm")).To(Equal(contestapp.FormatICPC))
		Expect(contestapp.NormalizeFormat("")).To(Equal(contestapp.FormatICPC))
		Expect(contestapp.NormalizeFormat("ioi")).To(Equal(contestapp.FormatIOI))
		Expect(contestapp.NormalizeFormat("oi")).To(Equal(contestapp.FormatOI))
	})
})

var _ = Describe("Contest phase helpers", func() {
	build := func() *contestapp.Contest {
		freeze := at(240)
		return &contestapp.Contest{
			BeginAt: contestStart, EndAt: at(300), FreezeAt: &freeze,
			Feedback: contestapp.FeedbackNone,
		}
	}

	It("freezes only between the freeze time and the unfreeze time", func() {
		item := build()
		Expect(item.Frozen(at(100))).To(BeFalse())
		Expect(item.Frozen(at(250))).To(BeTrue())

		unfreeze := at(320)
		item.UnfreezeAt = &unfreeze
		Expect(item.Frozen(at(310))).To(BeTrue())
		Expect(item.Frozen(at(330))).To(BeFalse())
	})

	It("restores full feedback once the contest ends", func() {
		item := build()
		Expect(item.FeedbackFor(at(100))).To(Equal(contestapp.FeedbackNone))
		Expect(item.FeedbackFor(at(301))).To(Equal(contestapp.FeedbackFull))
	})
})
