package profile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/domain"
)

// ProfileStore reads the aggregate a profile page needs. Each query is
// independent so a slow section cannot corrupt the others.
type ProfileStore struct{ db *database.DB }

func NewProfileStore(db *database.DB) *ProfileStore { return &ProfileStore{db: db} }

// ByUsername 组装完整个人主页数据;用户不存在返回 ErrNotFound。
func (s *ProfileStore) ByUsername(ctx context.Context, username string) (*Profile, error) {
	var result Profile
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT id, username, role, rating, created_at FROM users WHERE username = $1`, username,
	).Scan(&result.UserID, &result.Username, &result.Role, &result.Rating, &result.JoinedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if err := s.db.Pool.QueryRowContext(ctx,
		`SELECT count(DISTINCT problem_id) FILTER (WHERE status = 'Accepted')::int,
		        count(DISTINCT problem_id)::int,
		        count(*)::int,
		        count(*) FILTER (WHERE status = 'Accepted')::int
		 FROM submissions WHERE user_id = $1::uuid AND contest_id IS NULL AND domain_id = $2
		 AND EXISTS (SELECT 1 FROM problems WHERE id = submissions.problem_id AND visibility = 'public')`, result.UserID, domain.ID(ctx),
	).Scan(&result.SolvedCount, &result.AttemptedCount, &result.SubmissionCount, &result.AcceptedCount); err != nil {
		return nil, err
	}

	byDifficulty, err := s.byDifficulty(ctx, result.UserID)
	if err != nil {
		return nil, err
	}
	result.ByDifficulty = byDifficulty

	activity, err := s.activity(ctx, result.UserID)
	if err != nil {
		return nil, err
	}
	result.Activity = activity
	return &result, nil
}

// byDifficulty 统计每个难度档的公开题目总数与该用户已通过数。
func (s *ProfileStore) byDifficulty(ctx context.Context, userID string) ([]DifficultyProgress, error) {
	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT p.difficulty,
		        count(*) FILTER (WHERE solved.problem_id IS NOT NULL)::int AS solved,
		        count(*)::int AS total
		 FROM problems p
		 LEFT JOIN (
		     SELECT DISTINCT problem_id FROM submissions
		     WHERE user_id = $1::uuid AND contest_id IS NULL AND status = 'Accepted'
		 ) solved ON solved.problem_id = p.id
		 WHERE p.visibility = 'public' AND p.domain_id = $2
		 GROUP BY p.difficulty
		 ORDER BY p.difficulty`, userID, domain.ID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buckets := []DifficultyProgress{}
	for rows.Next() {
		var bucket DifficultyProgress
		if err := rows.Scan(&bucket.Difficulty, &bucket.Solved, &bucket.Total); err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}
	return buckets, rows.Err()
}

// activity 返回最近 ActivityWindowDays 天里有提交的日期及其提交数。
func (s *ProfileStore) activity(ctx context.Context, userID string) ([]ActivityDay, error) {
	rows, err := s.db.Pool.QueryContext(ctx,
		fmt.Sprintf(`SELECT to_char(submitted_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS day, count(*)::int
		 FROM submissions
		 WHERE user_id = $1::uuid AND contest_id IS NULL AND domain_id = $2
		   AND EXISTS (SELECT 1 FROM problems WHERE id = submissions.problem_id AND visibility = 'public')
		   AND submitted_at >= now() - interval '%d days'
		 GROUP BY day
		 ORDER BY day`, ActivityWindowDays), userID, domain.ID(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	days := []ActivityDay{}
	for rows.Next() {
		var day ActivityDay
		if err := rows.Scan(&day.Date, &day.Count); err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return days, rows.Err()
}
