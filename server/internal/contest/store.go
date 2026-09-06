package contest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/database"
)

// ContestStore owns contest persistence and scoreboard reads.
type ContestStore struct{ db *database.DB }

func NewContestStore(db *database.DB) *ContestStore { return &ContestStore{db: db} }

const contestColumns = `id, title, description, rule, begin_at, end_at, freeze_at, unfreeze_at,
	penalty_minutes, penalize_compile_error, feedback, visibility, password_hash,
	rankboard_visible, created_by, created_at`

func scanContest(scanner interface{ Scan(...any) error }) (Contest, error) {
	var item Contest
	err := scanner.Scan(&item.ID, &item.Title, &item.Description, &item.Rule,
		&item.BeginAt, &item.EndAt, &item.FreezeAt, &item.UnfreezeAt,
		&item.PenaltyMinutes, &item.PenalizeCompileError, &item.Feedback,
		&item.Visibility, &item.PasswordHash, &item.RankboardVisible,
		&item.CreatedBy, &item.CreatedAt)
	return item, err
}

func (s *ContestStore) Create(ctx context.Context, createdBy string, in *PersistInput) (*Contest, error) {
	item, err := scanContest(s.db.Pool.QueryRowContext(ctx,
		`INSERT INTO contests (title, description, rule, begin_at, end_at, freeze_at, unfreeze_at,
		                      penalty_minutes, penalize_compile_error, feedback,
		                      visibility, password_hash, rankboard_visible, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 RETURNING `+contestColumns,
		in.Title, in.Description, in.Rule, in.BeginAt, in.EndAt, in.FreezeAt, in.UnfreezeAt,
		in.PenaltyMinutes, in.PenalizeCompileError, in.Feedback,
		in.Visibility, in.PasswordHash, in.RankboardVisible, createdBy))
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// Update replaces contest settings and, because the scoring rules may have
// changed, recomputes the whole scoreboard in the same transaction.
func (s *ContestStore) Update(ctx context.Context, id string, in *PersistInput) (*Contest, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	item, err := scanContest(tx.QueryRowContext(ctx,
		`UPDATE contests SET title = $2, description = $3, rule = $4, begin_at = $5,
		        end_at = $6, freeze_at = $7, unfreeze_at = $8,
		        penalty_minutes = $9, penalize_compile_error = $10, feedback = $11,
		        visibility = $12,
		        password_hash = CASE
		          WHEN $13 <> '' THEN $13
		          WHEN $12 = 'password' THEN password_hash
		          ELSE ''
		        END,
		        rankboard_visible = $14
		 WHERE id = $1
		 RETURNING `+contestColumns,
		id, in.Title, in.Description, in.Rule, in.BeginAt, in.EndAt, in.FreezeAt, in.UnfreezeAt,
		in.PenaltyMinutes, in.PenalizeCompileError, in.Feedback,
		in.Visibility, in.PasswordHash, in.RankboardVisible))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := RebuildContest(ctx, tx, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *ContestStore) List(ctx context.Context, limit, offset int) ([]Contest, int, error) {
	return s.list(ctx, limit, offset, true)
}

func (s *ContestStore) ListAdmin(ctx context.Context, limit, offset int) ([]Contest, int, error) {
	return s.list(ctx, limit, offset, false)
}

func (s *ContestStore) list(ctx context.Context, limit, offset int, publicOnly bool) ([]Contest, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int
	if err := s.db.Pool.QueryRowContext(ctx,
		`SELECT count(*) FROM contests WHERE (NOT $1 OR visibility <> 'private')`, publicOnly,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT `+contestColumns+`
		 FROM contests WHERE (NOT $3 OR visibility <> 'private')
		 ORDER BY begin_at DESC LIMIT $1 OFFSET $2`, limit, offset, publicOnly)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Contest{}
	for rows.Next() {
		item, err := scanContest(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, item)
	}
	return list, total, rows.Err()
}

func (s *ContestStore) Get(ctx context.Context, id string) (*Contest, error) {
	item, err := scanContest(s.db.Pool.QueryRowContext(ctx,
		`SELECT `+contestColumns+` FROM contests WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// Problems returns the contest problem set with its jury metadata.
func (s *ContestStore) Problems(ctx context.Context, contestID string) ([]Problem, error) {
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT cp.contest_id, cp.problem_id, cp.sort_order, cp.label, cp.color, cp.points,
		        p.title, p.difficulty, p.visibility,
		        COALESCE(jsonb_agg(t.name ORDER BY t.name)
		        FILTER (WHERE t.name IS NOT NULL), '[]'::jsonb)
		 FROM contest_problems cp
		 JOIN problems p ON p.id = cp.problem_id
		 LEFT JOIN problem_tags pt ON pt.problem_id = p.id
		 LEFT JOIN tags t ON t.id = pt.tag_id
		 WHERE cp.contest_id = $1
		 GROUP BY cp.contest_id, cp.problem_id, cp.sort_order, cp.label, cp.color, cp.points,
		          p.title, p.difficulty, p.visibility
		 ORDER BY cp.sort_order`, contestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Problem{}
	for rows.Next() {
		var item Problem
		var tagsJSON []byte
		if err := rows.Scan(&item.ContestID, &item.ProblemID, &item.SortOrder,
			&item.Label, &item.Color, &item.Points,
			&item.Title, &item.Difficulty, &item.Visibility, &tagsJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tagsJSON, &item.Tags); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

// Problem loads one full statement only when the problem is linked to the
// requested contest. Authorization remains in Service; keeping the relation
// in this SQL makes a guessed problem ID insufficient.
func (s *ContestStore) Problem(ctx context.Context, contestID, problemID string) (*ProblemDetail, error) {
	var item ProblemDetail
	var tagsJSON []byte
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT cp.contest_id, cp.problem_id, cp.sort_order, cp.label, cp.color, cp.points,
		        p.title, p.difficulty, p.visibility,
		        COALESCE(jsonb_agg(t.name ORDER BY t.name)
		          FILTER (WHERE t.name IS NOT NULL), '[]'::jsonb),
		        p.statement_md, p.source, p.time_limit_ms, p.memory_limit_kb, p.judge_type
		 FROM contest_problems AS cp
		 JOIN problems AS p ON p.id = cp.problem_id
		 LEFT JOIN problem_tags AS pt ON pt.problem_id = p.id
		 LEFT JOIN tags AS t ON t.id = pt.tag_id
		 WHERE cp.contest_id = $1 AND cp.problem_id = $2
		 GROUP BY cp.contest_id, cp.problem_id, cp.sort_order, cp.label, cp.color, cp.points,
		          p.title, p.difficulty, p.visibility, p.statement_md, p.source,
		          p.time_limit_ms, p.memory_limit_kb, p.judge_type`,
		contestID, problemID).Scan(
		&item.ContestID, &item.ProblemID, &item.SortOrder, &item.Label, &item.Color, &item.Points,
		&item.Title, &item.Difficulty, &item.Visibility, &tagsJSON,
		&item.StatementMD, &item.Source, &item.TimeLimitMs, &item.MemoryLimitKB, &item.JudgeType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProblemNotInContest
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(tagsJSON, &item.Tags); err != nil {
		return nil, err
	}
	return &item, nil
}

// SetProblems replaces the contest problem set and rebuilds the scoreboard,
// because changing a problem's point value changes every cell that scored it.
func (s *ContestStore) SetProblems(ctx context.Context, contestID string, entries []ProblemEntry) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM contest_problems WHERE contest_id = $1`, contestID); err != nil {
		return err
	}
	for index, entry := range entries {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO contest_problems (contest_id, problem_id, sort_order, label, color, points)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			contestID, entry.ProblemID, index, entry.Label, entry.Color, entry.Points); err != nil {
			return err
		}
	}
	// Cells for problems that left the contest are stale; drop them before the
	// rebuild so they cannot linger on the board.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM contest_submission_cells
		 WHERE contest_id = $1 AND problem_id NOT IN (
		   SELECT problem_id FROM contest_problems WHERE contest_id = $1)`, contestID); err != nil {
		return err
	}
	if err := RebuildContest(ctx, tx, contestID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *ContestStore) IsParticipant(ctx context.Context, contestID, userID string) (bool, error) {
	var exists bool
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM contest_participants WHERE contest_id = $1 AND user_id = $2)`,
		contestID, userID).Scan(&exists)
	return exists, err
}

func (s *ContestStore) Register(ctx context.Context, contestID, userID string) error {
	_, err := s.db.Pool.ExecContext(ctx,
		`INSERT INTO contest_participants (contest_id, user_id) VALUES ($1, $2)
		 ON CONFLICT (contest_id, user_id) DO NOTHING`,
		contestID, userID)
	return err
}

func (s *ContestStore) HasProblem(ctx context.Context, contestID, problemID string) (bool, error) {
	var exists bool
	if err := s.db.Pool.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM contest_problems WHERE contest_id = $1 AND problem_id = $2
		)`, contestID, problemID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// ---------- staff ----------

// StaffRole returns the caller's contest role, or an empty string when they
// hold none.
func (s *ContestStore) StaffRole(ctx context.Context, contestID, userID string) (string, error) {
	var role string
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT role FROM contest_staff WHERE contest_id = $1 AND user_id = $2`,
		contestID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return role, err
}

func (s *ContestStore) ListStaff(ctx context.Context, contestID string) ([]Staff, error) {
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT staff.contest_id, staff.user_id, u.username, staff.role, staff.created_at
		 FROM contest_staff AS staff JOIN users u ON u.id = staff.user_id
		 WHERE staff.contest_id = $1 ORDER BY staff.role, u.username`, contestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Staff{}
	for rows.Next() {
		var item Staff
		if err := rows.Scan(&item.ContestID, &item.UserID, &item.Username,
			&item.Role, &item.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

// AddStaff grants a contest role by username, which is what a jury actually
// has at hand when setting up a contest.
func (s *ContestStore) AddStaff(ctx context.Context, contestID, username, role string) (*Staff, error) {
	var item Staff
	err := s.db.Pool.QueryRowContext(ctx,
		`WITH target AS (SELECT id, username FROM users WHERE username = $2),
		 upserted AS (
		   INSERT INTO contest_staff (contest_id, user_id, role)
		   SELECT $1, target.id, $3 FROM target
		   ON CONFLICT (contest_id, user_id) DO UPDATE SET role = EXCLUDED.role
		   RETURNING contest_id, user_id, role, created_at
		 )
		 SELECT upserted.contest_id, upserted.user_id, target.username, upserted.role, upserted.created_at
		 FROM upserted JOIN target ON target.id = upserted.user_id`,
		contestID, username, role).Scan(
		&item.ContestID, &item.UserID, &item.Username, &item.Role, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, invalid("no such user: " + username)
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *ContestStore) RemoveStaff(ctx context.Context, contestID, userID string) error {
	result, err := s.db.Pool.ExecContext(ctx,
		`DELETE FROM contest_staff WHERE contest_id = $1 AND user_id = $2`, contestID, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- scoreboard ----------

// Rankboard assembles the scoreboard from the stored cells. Both the frozen
// and the unfrozen view live in the same row, so this is two queries no matter
// how many contestants there are.
func (s *ContestStore) Rankboard(ctx context.Context, contestID string, jury bool) (*Rankboard, error) {
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

	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT participant.user_id, u.username
		 FROM contest_participants AS participant
		 JOIN users u ON u.id = participant.user_id
		 WHERE participant.contest_id = $1`, contestID)
	if err != nil {
		return nil, err
	}
	board := make([]RankRow, 0, 64)
	rowIndex := make(map[string]int, 64)
	for rows.Next() {
		var row RankRow
		if err := rows.Scan(&row.UserID, &row.Username); err != nil {
			rows.Close()
			return nil, err
		}
		row.Cells = make([]Cell, len(problems))
		rowIndex[row.UserID] = len(board)
		board = append(board, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	cellRows, err := s.db.Pool.QueryContext(ctx,
		`SELECT user_id, problem_id, attempts, penalty_sec, score, solved_at,
		        public_attempts, public_penalty_sec, public_score, public_solved_at,
		        pending_count, last_submit_at
		 FROM contest_submission_cells WHERE contest_id = $1`, contestID)
	if err != nil {
		return nil, err
	}
	for cellRows.Next() {
		var userID, problemID string
		var cell Cell
		var solvedAt, publicSolvedAt, lastSubmitAt sql.NullTime
		if err := cellRows.Scan(&userID, &problemID,
			&cell.Attempts, &cell.PenaltySec, &cell.Score, &solvedAt,
			&cell.PublicAttempts, &cell.PublicPenaltySec, &cell.PublicScore, &publicSolvedAt,
			&cell.PendingCount, &lastSubmitAt); err != nil {
			cellRows.Close()
			return nil, err
		}
		cell.SolvedAt = scanTime(solvedAt)
		cell.PublicSolvedAt = scanTime(publicSolvedAt)
		cell.LastSubmitAt = scanTime(lastSubmitAt)

		row, hasRow := rowIndex[userID]
		column, hasColumn := problemIndex[problemID]
		if !hasRow || !hasColumn {
			// A cell for someone who left the contest, or for a problem that
			// was removed from it; neither belongs on the board.
			continue
		}
		board[row].Cells[column] = cell
	}
	cellRows.Close()
	if err := cellRows.Err(); err != nil {
		return nil, err
	}

	format := item.Format()
	firstSolvers := make(map[string]string, len(problems))
	firstSolvedAt := make(map[string]int64, len(problems))
	for i := range board {
		totals := Totals(board[i].Cells, jury)
		board[i].Solved = totals.Solved
		board[i].Score = totals.Score
		board[i].Penalty = totals.PenaltySec
		board[i].LastAcceptedAt = totals.LastAcceptedAt
		for column, cell := range board[i].Cells {
			if cell.PendingCount > 0 && !jury {
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
	AssignRanks(format, board)

	return &Rankboard{
		Format: format, ProblemCount: len(problems), ProblemIDs: problemIDs,
		Problems: problems, Rows: board, FirstSolvers: firstSolvers,
	}, nil
}
