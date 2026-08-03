package dto

import (
	"time"

	"github.com/RimuruChan/Vertex/web/internal/contest"
)

type RankboardCellResponse struct {
	Attempts     int        `json:"attempts"`
	PenaltySec   int        `json:"penaltySec"`
	SolvedAt     *time.Time `json:"solvedAt,omitempty"`
	PendingCount int        `json:"pendingCount"`
}

type RankboardRowResponse struct {
	Rank         int                     `json:"rank"`
	Username     string                  `json:"username"`
	UserID       string                  `json:"userId"`
	Solved       int                     `json:"solved"`
	Penalty      int                     `json:"penalty"`
	Cells        []RankboardCellResponse `json:"cells"`
	HasFreezeHit bool                    `json:"hasFreezeHit"`
}

type RankboardResponse struct {
	ProblemCount int                    `json:"problemCount"`
	ProblemIDs   []string               `json:"problemIds"`
	Rows         []RankboardRowResponse `json:"rows"`
	Frozen       bool                   `json:"frozen"`
	FrozenAt     *time.Time             `json:"frozenAt,omitempty"`
}

func FromRankboard(board *contest.Rankboard) RankboardResponse {
	response := RankboardResponse{
		ProblemCount: board.ProblemCount, ProblemIDs: board.ProblemIDs, Frozen: board.Frozen,
		FrozenAt: board.FrozenAt, Rows: make([]RankboardRowResponse, 0, len(board.Rows)),
	}
	for _, row := range board.Rows {
		item := RankboardRowResponse{
			Rank: row.Rank, Username: row.Username, UserID: row.UserID, Solved: row.Solved,
			Penalty: row.Penalty, HasFreezeHit: row.HasFreezeHit,
			Cells: make([]RankboardCellResponse, 0, len(row.Cells)),
		}
		for _, cell := range row.Cells {
			item.Cells = append(item.Cells, RankboardCellResponse{
				Attempts: cell.Attempts, PenaltySec: cell.PenaltySec,
				SolvedAt: cell.SolvedAt, PendingCount: cell.PendingCount,
			})
		}
		response.Rows = append(response.Rows, item)
	}
	return response
}
