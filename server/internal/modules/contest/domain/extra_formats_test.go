package domain_test

import (
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	"testing"
)

func TestAdditionalScoringFormats(t *testing.T) {
	tests := []struct {
		name, format    string
		runs            []contestdomain.ScoredSubmission
		score, attempts int
		solved          bool
	}{
		{"leduo second attempt", "leduo", []contestdomain.ScoredSubmission{submission(10, "Wrong Answer", 0), submission(20, "Accepted", 100)}, 950, 2, true},
		{"leduo keeps best adjusted partial", "leduo", []contestdomain.ScoredSubmission{submission(10, "Wrong Answer", 80), submission(20, "Wrong Answer", 40)}, 800, 2, false},
		{"leduo compile errors count", "leduo", []contestdomain.ScoredSubmission{submission(10, "Compile Error", 0), submission(20, "Accepted", 100)}, 950, 2, true},
		{"cf time and wrong penalty", "cf", []contestdomain.ScoredSubmission{submission(10, "Wrong Answer", 50), submission(30, "Accepted", 100)}, 830, 2, true},
		{"cf ignores CE and runs after AC", "cf", []contestdomain.ScoredSubmission{submission(10, "Compile Error", 0), submission(30, "Accepted", 100), submission(40, "Wrong Answer", 0)}, 880, 1, true},
		{"cf rejects partial points", "cf", []contestdomain.ScoredSubmission{submission(10, "Wrong Answer", 99)}, 0, 1, false},
		{"oi last CE cannot preserve earlier score", "oi", []contestdomain.ScoredSubmission{submission(10, "Accepted", 100), submission(20, "Compile Error", 0)}, 0, 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := icpcRules()
			rules.Format = tt.format
			rules.EndAt = at(120)
			rules.MaxPoints = 1000
			rules.PenalizeCompileError = false
			cell := contestdomain.ScoreCell(rules, tt.runs)
			if cell.Score != tt.score || cell.Attempts != tt.attempts || cell.Solved() != tt.solved {
				t.Fatalf("unexpected cell: %+v", cell)
			}
			if cell.PublicScore != cell.Score {
				t.Fatal("unfrozen public projection differs")
			}
		})
	}
	t.Run("discount floors and frozen projections", func(t *testing.T) {
		runs := []contestdomain.ScoredSubmission{}
		for i := 0; i < 20; i++ {
			runs = append(runs, submission(i+1, "Wrong Answer", 0))
		}
		runs = append(runs, submission(100, "Accepted", 100))
		freeze := at(90)
		for _, format := range []string{"leduo", "cf"} {
			rules := icpcRules()
			rules.Format = format
			rules.EndAt = at(120)
			rules.MaxPoints = 1000
			rules.FreezeAt = &freeze
			cell := contestdomain.ScoreCell(rules, runs)
			want := 700
			if format == "cf" {
				want = 300
			}
			if cell.Score != want || cell.PublicScore != 0 || cell.PendingCount != 1 {
				t.Fatalf("%s: %+v", format, cell)
			}
		}
	})
	t.Run("traditional OI feedback", func(t *testing.T) {
		contest := contestdomain.Contest{Rule: "oi", Feedback: "full", EndAt: at(120)}
		freeze := at(100)
		contest.FreezeAt = &freeze
		if contest.FeedbackFor(at(119)) != "none" || contest.FeedbackFor(at(121)) != "full" {
			t.Fatal("OI feedback must open only after the contest")
		}
		if contest.Frozen(at(121)) {
			t.Fatal("OI final scores must not remain frozen")
		}
	})
}
