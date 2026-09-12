package postgres

import (
	"context"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
)

// ---------- scoreboard ----------

// Rankboard assembles the scoreboard from the stored cells. Both the frozen
// and the unfrozen view live in the same row, so this is two queries no matter
// how many contestants there are.
func (s *Repository) Rankboard(ctx context.Context, contestID string, jury bool) (*contestdomain.Rankboard, error) {
	item, err := s.Get(ctx, contestID)
	if err != nil {
		return nil, err
	}
	problems, err := s.Problems(ctx, contestID)
	if err != nil {
		return nil, err
	}
	problemIndex := make(map[string]int, len(problems))
	problemIDs := make([]string, len(problems))
	for i, problem := range problems {
		problemIndex[problem.ProblemID] = i
		problemIDs[i] = problem.ProblemID
	}

	rows, err := s.queries.ListRankboardParticipants(ctx, contestID)
	if err != nil {
		return nil, err
	}
	board := make([]contestdomain.RankRow, 0, 64)
	rowIndex := make(map[string]int, 64)
	for _, record := range rows {
		var row contestdomain.RankRow
		row.UserID = record.UserID
		row.Username = record.Username
		row.Cells = make([]contestdomain.Cell, len(problems))
		rowIndex[row.UserID] = len(board)
		board = append(board, row)
	}

	cellRows, err := s.queries.ListRankboardCells(ctx, contestID)
	if err != nil {
		return nil, err
	}
	for _, record := range cellRows {
		row, hasRow := rowIndex[record.UserID]
		column, hasColumn := problemIndex[record.ProblemID]
		if !hasRow || !hasColumn {
			continue
		}
		board[row].Cells[column] = contestdomain.Cell{Attempts: record.Attempts, PenaltySec: record.PenaltySec, Score: record.Score, SolvedAt: record.SolvedAt,
			PublicAttempts: record.PublicAttempts, PublicPenaltySec: record.PublicPenaltySec, PublicScore: record.PublicScore, PublicSolvedAt: record.PublicSolvedAt,
			PendingCount: record.PendingCount, LastSubmitAt: record.LastSubmitAt}
	}

	format := item.Format()
	firstSolvers := make(map[string]string, len(problems))
	firstSolvedAt := make(map[string]int64, len(problems))
	for i := range board {
		totals := contestdomain.Totals(board[i].Cells, jury)
		board[i].Solved = totals.Solved
		board[i].Score = totals.Score
		board[i].Penalty = totals.PenaltySec
		board[i].LastAcceptedAt = totals.LastAcceptedAt
		for column, cell := range board[i].Cells {
			if cell.PublicPendingCount(format) > 0 && !jury {
				board[i].HasPending = true
			}
			solvedAt := cell.PublicSolvedAt
			if jury {
				solvedAt = cell.SolvedAt
			}
			if solvedAt == nil {
				continue
			}
			problemID := problemIDs[column]
			if best, seen := firstSolvedAt[problemID]; !seen || solvedAt.UnixNano() < best {
				firstSolvedAt[problemID] = solvedAt.UnixNano()
				firstSolvers[problemID] = board[i].UserID
			}
		}
	}
	contestdomain.AssignRanks(format, board)

	return &contestdomain.Rankboard{
		FullResults: jury,
		Format:      format, ProblemCount: len(problems), ProblemIDs: problemIDs,
		Problems: problems, Rows: board, FirstSolvers: firstSolvers,
	}, nil
}
