package problem

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/RimuruChan/Vertex/server/internal/domain"
)

func (s *ProblemStore) GetWorkspace(ctx context.Context, id string) (*Problem, error) {
	var p Problem
	var tags []byte
	err := s.db.Pool.QueryRowxContext(ctx, `SELECT p.id,p.public_id,w.title,w.statement_md,w.difficulty,w.source,w.time_limit_ms,w.memory_limit_kb,p.visibility,p.author_id,
	 p.submission_count,p.accepted_count,p.solved_user_count,w.judge_type,p.created_at,w.updated_at,p.owner_id,p.domain_id,COALESCE(p.published_version,0),w.tags_json
	 FROM problems p JOIN problem_workspaces w ON w.problem_id=p.id WHERE p.id=$1 AND p.domain_id=$2`, id, domain.ID(ctx)).Scan(
		&p.ID, &p.PublicID, &p.Title, &p.StatementMD, &p.Difficulty, &p.Source, &p.TimeLimitMs, &p.MemoryLimitKb, &p.Visibility, &p.AuthorID,
		&p.SubmissionCount, &p.AcceptedCount, &p.SolvedUserCount, &p.JudgeType, &p.CreatedAt, &p.UpdatedAt, &p.OwnerID, &p.DomainID, &p.PublishedVersion, &tags)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(tags, &p.Tags); err != nil {
		return nil, err
	}
	return &p, nil
}
