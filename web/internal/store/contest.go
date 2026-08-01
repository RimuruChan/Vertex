package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/vertex-oj/web/internal/model"
)

// ContestStore 负责比赛读写与榜单计算。
type ContestStore struct{ db *DB }

func NewContestStore(db *DB) *ContestStore { return &ContestStore{db: db} }

// CreateInput 创建比赛的输入。
type CreateInput struct {
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Rule             string    `json:"rule"` // acm | ioi
	BeginAt          time.Time `json:"beginAt"`
	EndAt            time.Time `json:"endAt"`
	FreezeAt         *time.Time `json:"freezeAt,omitempty"`
	Visibility       string    `json:"visibility"`
	Password         string    `json:"password"`
	RankboardVisible bool      `json:"rankboardVisible"`
}

// Create 创建比赛。
func (s *ContestStore) Create(ctx context.Context, createdBy string, in *CreateInput) (*model.Contest, error) {
	if in.Rule == "" {
		in.Rule = "acm"
	}
	if in.Visibility == "" {
		in.Visibility = "public"
	}

	var c model.Contest
	err := s.db.Pool.QueryRow(ctx,
		`INSERT INTO contests (title, description, rule, begin_at, end_at, freeze_at,
		                      visibility, password_hash, rankboard_visible, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, title, description, rule, begin_at, end_at, freeze_at,
		           visibility, password_hash, rankboard_visible, created_by, created_at`,
		in.Title, in.Description, in.Rule, in.BeginAt, in.EndAt, in.FreezeAt,
		in.Visibility, in.Password, in.RankboardVisible, createdBy,
	).Scan(&c.ID, &c.Title, &c.Description, &c.Rule, &c.BeginAt, &c.EndAt, &c.FreezeAt,
		&c.Visibility, &c.PasswordHash, &c.RankboardVisible, &c.CreatedBy, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List 比赛列表(公开可见),分页。
func (s *ContestStore) List(ctx context.Context, limit, offset int) ([]model.Contest, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int
	if err := s.db.Pool.QueryRow(ctx, `SELECT count(*) FROM contests`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Pool.Query(ctx,
		`SELECT id, title, description, rule, begin_at, end_at, freeze_at,
		        visibility, password_hash, rankboard_visible, created_by, created_at
		 FROM contests ORDER BY begin_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := []model.Contest{}
	for rows.Next() {
		var c model.Contest
		if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.Rule, &c.BeginAt, &c.EndAt, &c.FreezeAt,
			&c.Visibility, &c.PasswordHash, &c.RankboardVisible, &c.CreatedBy, &c.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, c)
	}
	return list, total, rows.Err()
}

// Get 取比赛详情。
func (s *ContestStore) Get(ctx context.Context, id string) (*model.Contest, error) {
	var c model.Contest
	err := s.db.Pool.QueryRow(ctx,
		`SELECT id, title, description, rule, begin_at, end_at, freeze_at,
		        visibility, password_hash, rankboard_visible, created_by, created_at
		 FROM contests WHERE id = $1`, id,
	).Scan(&c.ID, &c.Title, &c.Description, &c.Rule, &c.BeginAt, &c.EndAt, &c.FreezeAt,
		&c.Visibility, &c.PasswordHash, &c.RankboardVisible, &c.CreatedBy, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Problems 取比赛题目列表(带排序)。
func (s *ContestStore) Problems(ctx context.Context, contestID string) ([]model.ContestProblem, error) {
	rows, err := s.db.Pool.Query(ctx,
		`SELECT contest_id, problem_id, sort_order FROM contest_problems
		 WHERE contest_id = $1 ORDER BY sort_order`, contestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []model.ContestProblem{}
	for rows.Next() {
		var cp model.ContestProblem
		if err := rows.Scan(&cp.ContestID, &cp.ProblemID, &cp.SortOrder); err != nil {
			return nil, err
		}
		list = append(list, cp)
	}
	return list, rows.Err()
}

// SetProblems 重建比赛题目集。
func (s *ContestStore) SetProblems(ctx context.Context, contestID string, problemIDs []string) error {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM contest_problems WHERE contest_id = $1`, contestID); err != nil {
		return err
	}
	for i, pid := range problemIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO contest_problems (contest_id, problem_id, sort_order) VALUES ($1, $2, $3)`,
			contestID, pid, i); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// IsParticipant 判断用户是否已注册比赛。
func (s *ContestStore) IsParticipant(ctx context.Context, contestID, userID string) (bool, error) {
	var exists bool
	err := s.db.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM contest_participants WHERE contest_id = $1 AND user_id = $2)`,
		contestID, userID).Scan(&exists)
	return exists, err
}

// Register 注册比赛(幂等)。
func (s *ContestStore) Register(ctx context.Context, contestID, userID string) error {
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO contest_participants (contest_id, user_id) VALUES ($1, $2)
		 ON CONFLICT (contest_id, user_id) DO NOTHING`,
		contestID, userID)
	return err
}

// --- ACM 榜单 ---

// ACMCell 一个用户在比赛中的单题积分格。
type ACMCell struct {
	Attempts    int
	PenaltySec  int
	SolvedAt    *time.Time
	PendingCount int
}

// RankRow 榜单一行。
type RankRow struct {
	Rank        int    `json:"rank"`
	Username    string `json:"username"`
	UserID      string `json:"userId"`
	Solved      int    `json:"solved"`
	Penalty     int    `json:"penalty"` // 罚时(秒)
	Cells       []ACMCell `json:"cells"`       // 按题序
	HasFreezeHit bool  `json:"hasFreezeHit"`  // 封榜期间是否有提交(需 pending 标记)
}

// Rankboard 榜单:按题序排列的 cells + 总览。
type Rankboard struct {
	ProblemCount int       `json:"problemCount"`
	ProblemIDs   []string  `json:"problemIds"`
	Rows         []RankRow `json:"rows"`
	Frozen       bool      `json:"frozen"`
	FrozenAt     *time.Time `json:"frozenAt,omitempty"`
}

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

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if accepted {
		// 首次 AC:插入 solved_at + 罚时;已有 solved_at 则忽略(幂等)
		solveTime := int(submittedAt.Sub(contest.BeginAt).Seconds())
		_, err = tx.Exec(ctx,
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
		return tx.Commit(ctx)
	}

	// 未 AC:仅在未 AC 时递增 attempts
	_, err = tx.Exec(ctx,
		`INSERT INTO contest_submission_cells (contest_id, user_id, problem_id, attempts)
		 VALUES ($1, $2, $3, 1)
		 ON CONFLICT (contest_id, user_id, problem_id) DO UPDATE SET
		   attempts = CASE WHEN contest_submission_cells.solved_at IS NULL
		                   THEN contest_submission_cells.attempts + 1
		                   ELSE contest_submission_cells.attempts END`,
		contestID, userID, problemID)
	return tx.Commit(ctx)
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
	rows, err := s.db.Pool.Query(ctx,
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
	cellRows, err := s.db.Pool.Query(ctx,
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
			subRows, err := s.db.Pool.Query(ctx,
				`SELECT problem_id FROM submissions
				 WHERE contest_id = $1 AND user_id = $2 AND status = 'Accepted'
				   AND submitted_at > $3`, contestID, uid, freeze)
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
