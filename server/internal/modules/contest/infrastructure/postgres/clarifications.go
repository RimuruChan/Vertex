package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres/internal/dbgen"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/platform/database"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ contestdomain.ClarificationRepository = (*Repository)(nil)
var _ contestdomain.Repository = (*Repository)(nil)

func clarificationFromRow(row dbgen.GetClarificationRow) contestdomain.Clarification {
	return contestdomain.Clarification{ID: row.ID, ContestID: row.ContestID, ProblemID: row.ProblemID, ParentID: database.Int64Ptr(row.ParentID), AuthorID: row.AuthorID, AuthorName: row.AuthorName,
		RecipientID: row.RecipientID, FromJury: row.FromJury, Subject: row.Subject, Body: row.Body, Answered: row.Answered, CreatedAt: row.CreatedAt, ProblemName: row.ProblemName}
}
func (s *Repository) GetClarification(ctx context.Context, contestID string, id int64) (*contestdomain.Clarification, error) {
	row, err := s.queries.GetClarification(ctx, dbgen.GetClarificationParams{ContestID: contestID, ClarificationID: id, DomainID: tenancydomain.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, contestdomain.ErrClarificationNotFound
	}
	if err != nil {
		return nil, err
	}
	item := clarificationFromRow(row)
	return &item, nil
}
func (s *Repository) ListClarifications(ctx context.Context, contestID string, viewer contestdomain.Viewer) ([]contestdomain.Clarification, error) {
	if !viewer.IsStaff() && viewer.UserID == "" {
		return nil, contestdomain.ErrNotParticipant
	}
	rows, err := s.queries.ListVisibleClarifications(ctx, dbgen.ListVisibleClarificationsParams{ContestID: contestID, DomainID: tenancydomain.ID(ctx), ViewerID: viewer.UserID, IsStaff: viewer.IsStaff()})
	if err != nil {
		return nil, err
	}
	threads := []contestdomain.Clarification{}
	index := map[int64]int{}
	orphans := []contestdomain.Clarification{}
	for _, row := range rows {
		item := clarificationFromRow(dbgen.GetClarificationRow(row))
		if item.ParentID == nil {
			index[item.ID] = len(threads)
			threads = append(threads, item)
			continue
		}
		if position, ok := index[*item.ParentID]; ok {
			threads[position].Replies = append(threads[position].Replies, item)
			continue
		}
		orphans = append(orphans, item)
	}
	return append(threads, orphans...), nil
}

// CreateClarification stores one message. A jury reply with no explicit
// recipient inherits the thread author, so answering a question never turns it
// into a broadcast by accident.
func (s *Repository) CreateClarification(ctx context.Context, input contestdomain.ClarificationInput) (*contestdomain.Clarification, error) {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	access, err := LockAccess(ctx, tx, input.ContestID, input.AuthorID)
	if err != nil {
		return nil, err
	}
	if input.FromJury {
		if !access.Permissions.Reply {
			return nil, contestdomain.ErrForbidden
		}
	} else {
		if !access.Registered || !access.Permissions.Submit {
			return nil, contestdomain.ErrNotParticipant
		}
		if time.Now().Before(access.BeginAt) || time.Now().After(access.EndAt) {
			return nil, contestdomain.ErrClarificationClosed
		}
	}

	q := s.queries.WithTx(tx.Tx)
	recipient := input.RecipientID
	if input.ParentID != nil {
		parent, err := q.GetClarificationParent(ctx, *input.ParentID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, contestdomain.ErrClarificationNotFound
		}
		if err != nil {
			return nil, err
		}
		parentContest, parentAuthor, parentProblem := parent.ContestID, parent.AuthorID, parent.ProblemID

		if parentContest != input.ContestID {
			return nil, contestdomain.ErrClarificationNotFound
		}
		if recipient == nil {
			recipient = parentAuthor
		}
		if input.ProblemID == nil {
			input.ProblemID = parentProblem
		}
	}
	if input.ProblemID != nil {
		problemID := strings.TrimSpace(*input.ProblemID)
		var parsed pgtype.UUID
		if err := parsed.Scan(problemID); err != nil || !parsed.Valid {
			return nil, contestdomain.Invalid("problem is not in contest")
		}
		matched, err := q.FindClarificationProblem(ctx, dbgen.FindClarificationProblemParams{ContestID: input.ContestID, ProblemID: problemID})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, contestdomain.Invalid("problem is not in contest")
		}
		if err != nil {
			return nil, err
		}

		input.ProblemID = &matched
	}

	var author *string
	if input.AuthorID != "" {
		author = &input.AuthorID
	}
	parentID := sql.NullInt64{}
	if input.ParentID != nil {
		parentID = sql.NullInt64{Int64: *input.ParentID, Valid: true}
	}
	id, err := q.CreateClarification(ctx, dbgen.CreateClarificationParams{ContestID: input.ContestID, ProblemID: input.ProblemID, ParentID: parentID, AuthorID: author, RecipientID: recipient, FromJury: input.FromJury, Subject: input.Subject, Body: input.Body, DomainID: access.Scope.Domain.ID})
	if err != nil {
		return nil, err
	}

	if input.FromJury && input.ParentID != nil {
		if err := q.MarkClarificationAnswered(ctx, dbgen.MarkClarificationAnsweredParams{ContestID: input.ContestID, ParentID: *input.ParentID}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetClarification(ctx, input.ContestID, id)
}
