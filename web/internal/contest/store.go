package contest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/RimuruChan/Vertex/web/internal/database"
)

// ContestStore 负责比赛读写与榜单计算。
type ContestStore struct{ db *database.DB }

func NewContestStore(db *database.DB) *ContestStore { return &ContestStore{db: db} }

// Create 创建比赛。
func (s *ContestStore) Create(ctx context.Context, createdBy string, in *PersistInput) (*Contest, error) {
	var c Contest
	err := s.db.Pool.QueryRowContext(ctx,
		`INSERT INTO contests (title, description, rule, begin_at, end_at, freeze_at,
		                      visibility, password_hash, rankboard_visible, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, title, description, rule, begin_at, end_at, freeze_at,
		           visibility, password_hash, rankboard_visible, created_by, created_at`,
		in.Title, in.Description, in.Rule, in.BeginAt, in.EndAt, in.FreezeAt,
		in.Visibility, in.PasswordHash, in.RankboardVisible, createdBy,
	).Scan(&c.ID, &c.Title, &c.Description, &c.Rule, &c.BeginAt, &c.EndAt, &c.FreezeAt,
		&c.Visibility, &c.PasswordHash, &c.RankboardVisible, &c.CreatedBy, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Update replaces contest settings with domain-validated persistence values.
func (s *ContestStore) Update(ctx context.Context, id string, in *PersistInput) (*Contest, error) {
	var c Contest
	err := s.db.Pool.QueryRowContext(ctx,
		`UPDATE contests SET title = $2, description = $3, rule = $4, begin_at = $5,
		        end_at = $6, freeze_at = $7, visibility = $8,
		        password_hash = CASE
		          WHEN $9 <> '' THEN $9
		          WHEN $8 = 'password' THEN password_hash
		          ELSE ''
		        END,
		        rankboard_visible = $10
		 WHERE id = $1
		 RETURNING id, title, description, rule, begin_at, end_at, freeze_at,
		           visibility, password_hash, rankboard_visible, created_by, created_at`,
		id, in.Title, in.Description, in.Rule, in.BeginAt, in.EndAt, in.FreezeAt,
		in.Visibility, in.PasswordHash, in.RankboardVisible,
	).Scan(&c.ID, &c.Title, &c.Description, &c.Rule, &c.BeginAt, &c.EndAt, &c.FreezeAt,
		&c.Visibility, &c.PasswordHash, &c.RankboardVisible, &c.CreatedBy, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List 比赛列表(公开可见),分页。
func (s *ContestStore) List(ctx context.Context, limit, offset int) ([]Contest, int, error) {
	return s.list(ctx, limit, offset, true)
}

// ListAdmin 管理员比赛列表，包含非公开比赛。
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
		`SELECT id, title, description, rule, begin_at, end_at, freeze_at,
		        visibility, password_hash, rankboard_visible, created_by, created_at
		 FROM contests WHERE (NOT $3 OR visibility <> 'private')
		 ORDER BY begin_at DESC LIMIT $1 OFFSET $2`, limit, offset, publicOnly)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []Contest{}
	for rows.Next() {
		var c Contest
		if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.Rule, &c.BeginAt, &c.EndAt, &c.FreezeAt,
			&c.Visibility, &c.PasswordHash, &c.RankboardVisible, &c.CreatedBy, &c.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, c)
	}
	return list, total, rows.Err()
}

// Get 取比赛详情。
func (s *ContestStore) Get(ctx context.Context, id string) (*Contest, error) {
	var c Contest
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT id, title, description, rule, begin_at, end_at, freeze_at,
		        visibility, password_hash, rankboard_visible, created_by, created_at
		 FROM contests WHERE id = $1`, id,
	).Scan(&c.ID, &c.Title, &c.Description, &c.Rule, &c.BeginAt, &c.EndAt, &c.FreezeAt,
		&c.Visibility, &c.PasswordHash, &c.RankboardVisible, &c.CreatedBy, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Problems 取比赛题目列表(带排序)。
func (s *ContestStore) Problems(ctx context.Context, contestID string) ([]Problem, error) {
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT cp.contest_id, cp.problem_id, cp.sort_order, p.title, p.difficulty,
		        p.visibility, COALESCE(jsonb_agg(t.name ORDER BY t.name)
		        FILTER (WHERE t.name IS NOT NULL), '[]'::jsonb)
		 FROM contest_problems cp
		 JOIN problems p ON p.id = cp.problem_id
		 LEFT JOIN problem_tags pt ON pt.problem_id = p.id
		 LEFT JOIN tags t ON t.id = pt.tag_id
		 WHERE cp.contest_id = $1
		 GROUP BY cp.contest_id, cp.problem_id, cp.sort_order, p.title, p.difficulty, p.visibility
		 ORDER BY cp.sort_order`, contestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Problem{}
	for rows.Next() {
		var cp Problem
		var tagsJSON []byte
		if err := rows.Scan(&cp.ContestID, &cp.ProblemID, &cp.SortOrder, &cp.Title,
			&cp.Difficulty, &cp.Visibility, &tagsJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tagsJSON, &cp.Tags); err != nil {
			return nil, err
		}
		list = append(list, cp)
	}
	return list, rows.Err()
}

// SetProblems 重建比赛题目集。
func (s *ContestStore) SetProblems(ctx context.Context, contestID string, problemIDs []string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM contest_problems WHERE contest_id = $1`, contestID); err != nil {
		return err
	}
	for i, pid := range problemIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO contest_problems (contest_id, problem_id, sort_order) VALUES ($1, $2, $3)`,
			contestID, pid, i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// IsParticipant 判断用户是否已注册比赛。
func (s *ContestStore) IsParticipant(ctx context.Context, contestID, userID string) (bool, error) {
	var exists bool
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM contest_participants WHERE contest_id = $1 AND user_id = $2)`,
		contestID, userID).Scan(&exists)
	return exists, err
}

// Register 注册比赛(幂等)。
func (s *ContestStore) Register(ctx context.Context, contestID, userID string) error {
	_, err := s.db.Pool.ExecContext(ctx,
		`INSERT INTO contest_participants (contest_id, user_id) VALUES ($1, $2)
		 ON CONFLICT (contest_id, user_id) DO NOTHING`,
		contestID, userID)
	return err
}

// HasProblem reports whether a problem is assigned to a contest.
func (s *ContestStore) HasProblem(ctx context.Context, contestID, problemID string) (bool, error) {
	var problemExists bool
	if err := s.db.Pool.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM contest_problems WHERE contest_id = $1 AND problem_id = $2
		)`, contestID, problemID).Scan(&problemExists); err != nil {
		return false, err
	}
	return problemExists, nil
}

// --- ACM 榜单 ---

// ACMPenaltyPerProblem 每次非 AC 罚时(秒)= 20 分钟。
const ACMPenaltyPerProblem = 20 * 60

// RecordContestSubmission 提交判定完成后更新比赛积分格(ACM 赛制)。
// 由判题完成回调调用;提交时间在比赛时间内才计。
// 幂等规则:只认首次 AC(已 AC 后不再更新任何字段);非 AC 的 attempts
// 仅在未 AC 时递增。rejudge 后重复调用不会污染积分。
func (s *ContestStore) RecordContestSubmission(ctx context.Context, contestID, userID, problemID string,
	submittedAt time.Time, accepted bool) error {

	contest, err := s.Get(ctx, contestID)
	if err != nil {
		return err
	}
	// 只在比赛时间内计入榜单
	if submittedAt.Before(contest.BeginAt) || submittedAt.After(contest.EndAt) {
		return nil
	}

	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if accepted {
		// 首次 AC:插入 solved_at + 罚时;已有 solved_at 则忽略(幂等)
		solveTime := int(submittedAt.Sub(contest.BeginAt).Seconds())
		_, err = tx.ExecContext(ctx,
			`INSERT INTO contest_submission_cells (contest_id, user_id, problem_id, attempts, penalty_sec, solved_at)
			 VALUES ($1, $2, $3, 1, $4, $5)
			 ON CONFLICT (contest_id, user_id, problem_id) DO UPDATE SET
			   attempts = CASE WHEN contest_submission_cells.solved_at IS NULL
			                   THEN contest_submission_cells.attempts + 1
			                   ELSE contest_submission_cells.attempts END,
			   penalty_sec = CASE WHEN contest_submission_cells.solved_at IS NULL
			                      THEN $4 + contest_submission_cells.attempts * `+itoaConst(ACMPenaltyPerProblem)+`
			                      ELSE contest_submission_cells.penalty_sec END,
			   solved_at = CASE WHEN contest_submission_cells.solved_at IS NULL
			                    THEN EXCLUDED.solved_at
			                    ELSE contest_submission_cells.solved_at END`,
			contestID, userID, problemID, solveTime, submittedAt)
		return tx.Commit()
	}

	// 未 AC:仅在未 AC 时递增 attempts
	_, err = tx.ExecContext(ctx,
		`INSERT INTO contest_submission_cells (contest_id, user_id, problem_id, attempts)
		 VALUES ($1, $2, $3, 1)
		 ON CONFLICT (contest_id, user_id, problem_id) DO UPDATE SET
		   attempts = CASE WHEN contest_submission_cells.solved_at IS NULL
		                   THEN contest_submission_cells.attempts + 1
		                   ELSE contest_submission_cells.attempts END`,
		contestID, userID, problemID)
	return tx.Commit()
}

// Rankboard 计算比赛榜单(ACM 赛制,支持封榜)。
// frozen=true 时:封榜时间后的提交隐藏(不计入 solved/penalty,显示为 pending)。
// rejudge 安全性:榜单由积分格派生,而积分格由提交事件幂等更新。
func (s *ContestStore) Rankboard(ctx context.Context, contestID string, frozen bool) (*Rankboard, error) {
	contest, err := s.Get(ctx, contestID)
	if err != nil {
		return nil, err
	}

	problems, err := s.Problems(ctx, contestID)
	if err != nil {
		return nil, err
	}
	problemIDs := make([]string, len(problems))
	for i, cp := range problems {
		problemIDs[i] = cp.ProblemID
	}

	// 参与者 + 用户名
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT cp.user_id, u.username
		 FROM contest_participants cp JOIN users u ON u.id = cp.user_id
		 WHERE cp.contest_id = $1`, contestID)
	if err != nil {
		return nil, err
	}
	userNames := map[string]string{}
	userOrder := []string{}
	for rows.Next() {
		var uid, uname string
		if err := rows.Scan(&uid, &uname); err != nil {
			rows.Close()
			return nil, err
		}
		userNames[uid] = uname
		userOrder = append(userOrder, uid)
	}
	rows.Close()

	// 每个用户的积分格
	cellsByUser := map[string]map[string]ACMCell{}
	for _, uid := range userOrder {
		cellsByUser[uid] = map[string]ACMCell{}
	}
	cellRows, err := s.db.Pool.QueryContext(ctx,
		`SELECT user_id, problem_id, attempts, penalty_sec, solved_at, pending_count
		 FROM contest_submission_cells WHERE contest_id = $1`, contestID)
	if err != nil {
		return nil, err
	}
	for cellRows.Next() {
		var (
			uid, pid string
			cell     ACMCell
		)
		if err := cellRows.Scan(&uid, &pid, &cell.Attempts, &cell.PenaltySec, &cell.SolvedAt, &cell.PendingCount); err != nil {
			cellRows.Close()
			return nil, err
		}
		cellsByUser[uid][pid] = cell
	}
	cellRows.Close()

	// 封榜处理:freeze_at 之后的提交不应显示 solved(计为 pending)
	// 由于积分格已包含全部提交,这里用冻结时间过滤:
	// 从 submissions 中找该用户该题在 freeze_at 后且已 AC 的提交 → 视为 pending。
	if frozen && contest.FreezeAt != nil {
		freeze := *contest.FreezeAt
		for _, uid := range userOrder {
			subRows, err := s.db.Pool.QueryContext(ctx,
				`SELECT problem_id FROM submissions
				 WHERE contest_id = $1 AND user_id = $2 AND status = 'Accepted'
				   AND submitted_at > $3
				   AND NOT EXISTS (
				     SELECT 1 FROM submissions earlier
				     WHERE earlier.contest_id = submissions.contest_id
				       AND earlier.user_id = submissions.user_id
				       AND earlier.problem_id = submissions.problem_id
				       AND earlier.status = 'Accepted' AND earlier.submitted_at <= $3
				   )`, contestID, uid, freeze)
			if err != nil {
				return nil, err
			}
			for subRows.Next() {
				var pid string
				if err := subRows.Scan(&pid); err != nil {
					subRows.Close()
					return nil, err
				}
				cell := cellsByUser[uid][pid]
				cell.SolvedAt = nil
				cell.PendingCount++
				cellsByUser[uid][pid] = cell
			}
			subRows.Close()
		}
	}

	// 汇总行
	rowsOut := make([]RankRow, 0, len(userOrder))
	for _, uid := range userOrder {
		row := RankRow{UserID: uid, Username: userNames[uid], Cells: make([]ACMCell, len(problemIDs))}
		hasFreezeHit := false
		for i, pid := range problemIDs {
			cell, _ := cellsByUser[uid][pid]
			row.Cells[i] = cell
			if cell.SolvedAt != nil {
				row.Solved++
				row.Penalty += cell.PenaltySec
			}
			if cell.PendingCount > 0 {
				hasFreezeHit = true
			}
		}
		row.HasFreezeHit = hasFreezeHit
		rowsOut = append(rowsOut, row)
	}

	// 排序:solved 降序,penalty 升序,用户名升序(稳定)
	// ACM:先 solved,再罚时少者在前
	sortRankRows(rowsOut)

	// 排名(并列同 rank)
	for i := range rowsOut {
		if i > 0 && rowsOut[i].Solved == rowsOut[i-1].Solved && rowsOut[i].Penalty == rowsOut[i-1].Penalty {
			rowsOut[i].Rank = rowsOut[i-1].Rank
		} else {
			rowsOut[i].Rank = i + 1
		}
	}

	rb := &Rankboard{
		ProblemCount: len(problemIDs),
		ProblemIDs:   problemIDs,
		Rows:         rowsOut,
		Frozen:       frozen,
		FrozenAt:     contest.FreezeAt,
	}
	return rb, nil
}

func sortRankRows(rows []RankRow) {
	// 简单插入排序(比赛人数通常 < 几百)
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0; j-- {
			a, b := rows[j], rows[j-1]
			better := a.Solved > b.Solved ||
				(a.Solved == b.Solved && a.Penalty < b.Penalty) ||
				(a.Solved == b.Solved && a.Penalty == b.Penalty && a.Username < b.Username)
			if better {
				rows[j], rows[j-1] = rows[j-1], rows[j]
			} else {
				break
			}
		}
	}
}

func itoaConst(n int) string {
	return fmt.Sprintf("%d", n)
}
