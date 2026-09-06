package contest

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrClarificationNotFound = errors.New("clarification not found")
	ErrClarificationClosed   = errors.New("clarifications are closed")
)

// Clarification is one message in a contest's question channel. Teams open
// threads, the jury replies into them, and the jury can also open a thread of
// its own as a broadcast announcement.
type Clarification struct {
	ID          int64
	ContestID   string
	ProblemID   *string
	ProblemName string
	ParentID    *int64
	AuthorID    *string
	AuthorName  string
	RecipientID *string
	FromJury    bool
	Subject     string
	Body        string
	Answered    bool
	CreatedAt   time.Time
	// Replies is populated for thread roots.
	Replies []Clarification
}

// IsAnnouncement reports a jury message addressed to the whole contest.
func (c *Clarification) IsAnnouncement() bool { return c.FromJury && c.RecipientID == nil }

// VisibleTo decides whether one contestant may read this message. Staff read
// everything and do not go through here.
func (c *Clarification) VisibleTo(userID string) bool {
	if c.IsAnnouncement() {
		return true
	}
	if c.AuthorID != nil && *c.AuthorID == userID {
		return true
	}
	return c.RecipientID != nil && *c.RecipientID == userID
}

// ClarificationInput is a new message from either side of the channel.
type ClarificationInput struct {
	ContestID string
	ProblemID *string
	ParentID  *int64
	AuthorID  string
	// RecipientID targets one contestant. Jury replies inherit the asker when
	// it is empty; a jury message with no parent and no recipient is a
	// broadcast announcement.
	RecipientID *string
	FromJury    bool
	Subject     string
	Body        string
}

const maxClarificationBody = 8000

// ClarificationRepository is the persistence boundary for the question channel.
type ClarificationRepository interface {
	CreateClarification(ctx context.Context, input ClarificationInput) (*Clarification, error)
	ListClarifications(ctx context.Context, contestID string, viewer Viewer) ([]Clarification, error)
	GetClarification(ctx context.Context, contestID string, id int64) (*Clarification, error)
}

// Ask records a contestant's question. Questions are only accepted while the
// contest is reachable: before it starts there is nothing to ask about, and
// once it is over the jury channel is closed.
func (s *Service) Ask(ctx context.Context, input ClarificationInput) (*Clarification, error) {
	item, err := s.repository.Get(ctx, input.ContestID)
	if err != nil {
		return nil, err
	}
	if s.clarifications == nil {
		return nil, ErrClarificationNotFound
	}
	now := s.now()
	if now.Before(item.BeginAt) || item.Ended(now) {
		return nil, ErrClarificationClosed
	}
	registered, err := s.repository.IsParticipant(ctx, input.ContestID, input.AuthorID)
	if err != nil {
		return nil, err
	}
	if !registered {
		return nil, ErrNotParticipant
	}
	prepared, err := prepareClarification(input, false)
	if err != nil {
		return nil, err
	}
	return s.clarifications.CreateClarification(ctx, *prepared)
}

// Reply records a jury answer or announcement. A reply marks its thread as
// answered so the jury queue drains.
func (s *Service) Reply(ctx context.Context, input ClarificationInput) (*Clarification, error) {
	if s.clarifications == nil {
		return nil, ErrClarificationNotFound
	}
	if _, err := s.repository.Get(ctx, input.ContestID); err != nil {
		return nil, err
	}
	prepared, err := prepareClarification(input, true)
	if err != nil {
		return nil, err
	}
	created, err := s.clarifications.CreateClarification(ctx, *prepared)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Clarifications returns the threads one viewer may read: everything for
// staff, and only their own threads plus announcements for a contestant.
func (s *Service) Clarifications(ctx context.Context, contestID, userID, role string) ([]Clarification, error) {
	if s.clarifications == nil {
		return nil, nil
	}
	item, err := s.repository.Get(ctx, contestID)
	if err != nil {
		return nil, err
	}
	viewer, registered, err := s.resolveViewerAccess(ctx, item, userID, role)
	if err != nil {
		return nil, err
	}

	switch item.Visibility {
	case "public":
		// Public contest announcements may be read by any authenticated user.
	case "password":
		if !viewer.IsStaff() && !registered {
			return nil, ErrRegistrationNeeded
		}
	case "private":
		// resolveViewerAccess already admitted only staff, owner or admin.
	default:
		// Unknown persisted states must never become public by accident.
		return nil, ErrNotFound
	}
	return s.clarifications.ListClarifications(ctx, contestID, viewer)
}

func prepareClarification(input ClarificationInput, fromJury bool) (*ClarificationInput, error) {
	input.FromJury = fromJury
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)
	if input.Body == "" {
		return nil, invalid("message body is required")
	}
	if len(input.Body) > maxClarificationBody {
		return nil, invalid("message body is too long")
	}
	if len(input.Subject) > 200 {
		return nil, invalid("subject must be at most 200 characters")
	}
	if !fromJury {
		// A contestant can only open a thread; they never address anyone.
		input.ParentID = nil
		input.RecipientID = nil
	}
	if fromJury && input.ParentID == nil && input.RecipientID == nil && input.Subject == "" {
		input.Subject = "公告"
	}
	return &input, nil
}

// ---------- persistence ----------

const clarificationColumns = `c.id, c.contest_id, c.problem_id, c.parent_id, c.author_id,
	COALESCE(author.username, ''), c.recipient_id, c.from_jury, c.subject, c.body,
	c.answered, c.created_at, COALESCE(p.title, '')`

func scanClarification(scanner interface{ Scan(...any) error }) (Clarification, error) {
	var item Clarification
	err := scanner.Scan(&item.ID, &item.ContestID, &item.ProblemID, &item.ParentID,
		&item.AuthorID, &item.AuthorName, &item.RecipientID, &item.FromJury,
		&item.Subject, &item.Body, &item.Answered, &item.CreatedAt, &item.ProblemName)
	return item, err
}

// CreateClarification stores one message. A jury reply with no explicit
// recipient inherits the thread author, so answering a question never turns it
// into a broadcast by accident.
func (s *ContestStore) CreateClarification(ctx context.Context, input ClarificationInput) (*Clarification, error) {
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
			return nil, ErrForbidden
		}
	} else {
		if !access.Registered || !access.Permissions.Submit {
			return nil, ErrNotParticipant
		}
		if time.Now().Before(access.BeginAt) || time.Now().After(access.EndAt) {
			return nil, ErrClarificationClosed
		}
	}

	recipient := input.RecipientID
	if input.ParentID != nil {
		var parentContest string
		var parentAuthor *string
		var parentProblem *string
		if err := tx.QueryRowContext(ctx,
			`SELECT contest_id, author_id, problem_id FROM clarifications WHERE id = $1`,
			*input.ParentID).Scan(&parentContest, &parentAuthor, &parentProblem); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrClarificationNotFound
			}
			return nil, err
		}
		if parentContest != input.ContestID {
			return nil, ErrClarificationNotFound
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
			return nil, invalid("problem is not in contest")
		}
		var matched string
		if err := tx.QueryRowContext(ctx,
			`SELECT problem_id FROM contest_problems
			 WHERE contest_id = $1 AND problem_id = $2
			 FOR KEY SHARE`, input.ContestID, parsed).Scan(&matched); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, invalid("problem is not in contest")
			}
			return nil, err
		}
		input.ProblemID = &matched
	}

	var author *string
	if input.AuthorID != "" {
		author = &input.AuthorID
	}
	var id int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO clarifications
		   (contest_id, problem_id, parent_id, author_id, recipient_id, from_jury, subject, body, domain_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id`,
		input.ContestID, input.ProblemID, input.ParentID, author, recipient,
		input.FromJury, input.Subject, input.Body, domain.ID(ctx)).Scan(&id); err != nil {
		return nil, err
	}
	if input.FromJury && input.ParentID != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE clarifications SET answered=true WHERE contest_id=$1 AND id=$2", input.ContestID, *input.ParentID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetClarification(ctx, input.ContestID, id)
}

func (s *ContestStore) GetClarification(ctx context.Context, contestID string, id int64) (*Clarification, error) {
	item, err := scanClarification(s.db.Pool.QueryRowContext(ctx,
		`SELECT `+clarificationColumns+`
		 FROM clarifications AS c
		 LEFT JOIN users AS author ON author.id = c.author_id
		 LEFT JOIN problems AS p ON p.id = c.problem_id
		 WHERE c.contest_id = $1 AND c.id = $2
		 AND EXISTS (SELECT 1 FROM contests WHERE id = $1 AND domain_id = $3)`, contestID, id, domain.ID(ctx)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrClarificationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// ListClarifications returns threads newest first, with replies nested under
// their root. Contestants only receive threads they participate in plus
// announcements; the filter runs in SQL so a large contest does not ship every
// team's questions to every client.
func (s *ContestStore) ListClarifications(ctx context.Context, contestID string, viewer Viewer) ([]Clarification, error) {
	visibility := ""
	args := []any{contestID, domain.ID(ctx)}
	if !viewer.IsStaff() {
		args = append(args, viewer.UserID)
		visibility = ` AND (
		    (c.from_jury AND c.recipient_id IS NULL)
		    OR c.author_id = $3::uuid
		    OR c.recipient_id = $3::uuid
		    OR c.parent_id IN (SELECT id FROM clarifications WHERE author_id = $3::uuid)
		  )`
	}

	rows, err := s.db.Pool.QueryContext(ctx,
		`SELECT `+clarificationColumns+`
		 FROM clarifications AS c
		 LEFT JOIN users AS author ON author.id = c.author_id
		 LEFT JOIN problems AS p ON p.id = c.problem_id
		 WHERE c.contest_id = $1 AND EXISTS (SELECT 1 FROM contests WHERE id = $1 AND domain_id = $2)`+visibility+`
		 ORDER BY COALESCE(c.parent_id, c.id) DESC, c.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	threads := []Clarification{}
	index := map[int64]int{}
	orphans := []Clarification{}
	for rows.Next() {
		item, err := scanClarification(rows)
		if err != nil {
			return nil, err
		}
		if item.ParentID == nil {
			index[item.ID] = len(threads)
			threads = append(threads, item)
			continue
		}
		if position, ok := index[*item.ParentID]; ok {
			threads[position].Replies = append(threads[position].Replies, item)
			continue
		}
		// A reply whose root the viewer cannot see still belongs to them.
		orphans = append(orphans, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return append(threads, orphans...), nil
}
