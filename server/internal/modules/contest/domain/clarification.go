package domain

import (
	"context"
	"errors"
	"strings"
	"time"
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

const MaxClarificationBody = 8000

// ClarificationRepository is the persistence boundary for the question channel.
type ClarificationRepository interface {
	CreateClarification(ctx context.Context, input ClarificationInput) (*Clarification, error)
	ListClarifications(ctx context.Context, contestID string, viewer Viewer) ([]Clarification, error)
	GetClarification(ctx context.Context, contestID string, id int64) (*Clarification, error)
}

func PrepareClarification(input ClarificationInput, fromJury bool) (*ClarificationInput, error) {
	input.FromJury = fromJury
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)
	if input.Body == "" {
		return nil, Invalid("message body is required")
	}
	if len(input.Body) > MaxClarificationBody {
		return nil, Invalid("message body is too long")
	}
	if len(input.Subject) > 200 {
		return nil, Invalid("subject must be at most 200 characters")
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
