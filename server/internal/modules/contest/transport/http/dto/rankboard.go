package dto

import (
	"time"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
)

// RankboardCellResponse is one scoreboard square as the viewer is allowed to
// see it. The server resolves the frozen/jury split before serializing, so the
// client renders exactly what it receives.
type RankboardCellResponse struct {
	Attempts     int        `json:"attempts"`
	PenaltySec   int        `json:"penaltySec"`
	Score        int        `json:"score"`
	SolvedAt     *time.Time `json:"solvedAt,omitempty"`
	PendingCount int        `json:"pendingCount"`
	// FirstSolver marks the earliest solve of this problem across the board.
	FirstSolver bool `json:"firstSolver"`
}

type RankboardRowResponse struct {
	Rank           int                     `json:"rank"`
	Username       string                  `json:"username"`
	UserID         string                  `json:"userId"`
	Solved         int                     `json:"solved"`
	Score          int                     `json:"score"`
	Penalty        int                     `json:"penalty"`
	LastAcceptedAt *time.Time              `json:"lastAcceptedAt,omitempty"`
	Cells          []RankboardCellResponse `json:"cells"`
	HasPending     bool                    `json:"hasPending"`
}

type RankboardProblemResponse struct {
	ProblemID string `json:"problemId"`
	Label     string `json:"label"`
	Color     string `json:"color"`
	Points    int    `json:"points"`
	Title     string `json:"title"`
}

type RankboardResponse struct {
	Format       string                     `json:"format" enums:"icpc,ioi,oi"`
	ProblemCount int                        `json:"problemCount"`
	ProblemIDs   []string                   `json:"problemIds"`
	Problems     []RankboardProblemResponse `json:"problems"`
	Rows         []RankboardRowResponse     `json:"rows"`
	Frozen       bool                       `json:"frozen"`
	FrozenAt     *time.Time                 `json:"frozenAt,omitempty"`
	UnfreezeAt   *time.Time                 `json:"unfreezeAt,omitempty"`
	JuryView     bool                       `json:"juryView"`
}

// FromRankboard projects the domain board onto the wire. Cells carry only the
// view the caller is entitled to: a frozen board never ships jury values.
func FromRankboard(board *contestdomain.Rankboard) RankboardResponse {
	response := RankboardResponse{
		Format: board.Format, ProblemCount: board.ProblemCount, ProblemIDs: board.ProblemIDs,
		Frozen: board.Frozen, FrozenAt: board.FrozenAt, UnfreezeAt: board.UnfreezeAt,
		JuryView: board.JuryView,
		Problems: make([]RankboardProblemResponse, 0, len(board.Problems)),
		Rows:     make([]RankboardRowResponse, 0, len(board.Rows)),
	}
	for _, problem := range board.Problems {
		response.Problems = append(response.Problems, RankboardProblemResponse{
			ProblemID: problem.ProblemID, Label: problem.Label, Color: problem.Color,
			Points: problem.Points, Title: problem.Title,
		})
	}
	for _, row := range board.Rows {
		item := RankboardRowResponse{
			Rank: row.Rank, Username: row.Username, UserID: row.UserID,
			Solved: row.Solved, Score: row.Score, Penalty: row.Penalty,
			LastAcceptedAt: row.LastAcceptedAt, HasPending: row.HasPending,
			Cells: make([]RankboardCellResponse, 0, len(row.Cells)),
		}
		for index, cell := range row.Cells {
			attempts, penalty, score, solvedAt := cell.PublicAttempts, cell.PublicPenaltySec,
				cell.PublicScore, cell.PublicSolvedAt
			if board.FullResults || board.JuryView {
				attempts, penalty, score, solvedAt = cell.Attempts, cell.PenaltySec,
					cell.Score, cell.SolvedAt
			}
			firstSolver := false
			pending := cell.PendingCount
			if board.FullResults || board.JuryView {
				pending = 0
			}
			if solvedAt != nil && index < len(board.ProblemIDs) {
				firstSolver = board.FirstSolvers[board.ProblemIDs[index]] == row.UserID
			}
			item.Cells = append(item.Cells, RankboardCellResponse{
				Attempts: attempts, PenaltySec: penalty, Score: score, SolvedAt: solvedAt,
				PendingCount: pending, FirstSolver: firstSolver,
			})
		}
		response.Rows = append(response.Rows, item)
	}
	return response
}
