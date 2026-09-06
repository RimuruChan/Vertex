package content

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/database"
)

// AccessStore is a narrow read model for resources that content can attach
// to. It deliberately returns only visibility, never another domain's model.
type AccessStore struct{ db *database.DB }

func NewAccessStore(db *database.DB) *AccessStore { return &AccessStore{db: db} }

func (s *AccessStore) CanViewProblem(
	ctx context.Context, problemID, userID string, admin bool,
) (bool, error) {
	var viewerID any
	if userID != "" {
		viewerID = userID
	}
	var visible bool
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM problems AS p
		   WHERE p.id = $1
		     AND (p.visibility = 'public' OR $3 OR p.author_id = $2::uuid)
		 )`, problemID, viewerID, admin).Scan(&visible)
	return visible, err
}

func (s *AccessStore) CanViewContest(
	ctx context.Context, contestID, userID string, admin bool,
) (bool, error) {
	var viewerID any
	if userID != "" {
		viewerID = userID
	}
	var visible bool
	err := s.db.Pool.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM contests AS c
		   WHERE c.id = $1
		     AND (
		       c.visibility = 'public'
		       OR $3
		       OR c.created_by = $2::uuid
		       OR EXISTS (
		         SELECT 1 FROM contest_staff AS staff
		         WHERE staff.contest_id = c.id AND staff.user_id = $2::uuid)
		       OR (c.visibility = 'password' AND EXISTS (
		         SELECT 1 FROM contest_participants AS participant
		         WHERE participant.contest_id = c.id AND participant.user_id = $2::uuid))
		     )
		 )`, contestID, viewerID, admin).Scan(&visible)
	return visible, err
}
