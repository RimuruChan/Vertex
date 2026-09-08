package application

import (
	"context"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/contest/domain"
)

// Ask records a contestant's question. Questions are only accepted while the
// contest is reachable: before it starts there is nothing to ask about, and
// once it is over the jury channel is closed.
func (s *Service) Ask(ctx context.Context, input contestdomain.ClarificationInput) (*contestdomain.Clarification, error) {
	item, err := s.repository.Get(ctx, input.ContestID)
	if err != nil {
		return nil, err
	}
	if s.clarifications == nil {
		return nil, contestdomain.ErrClarificationNotFound
	}
	now := s.now()
	if now.Before(item.BeginAt) || item.Ended(now) {
		return nil, contestdomain.ErrClarificationClosed
	}
	registered, err := s.repository.IsParticipant(ctx, input.ContestID, input.AuthorID)
	if err != nil {
		return nil, err
	}
	if !registered {
		return nil, contestdomain.ErrNotParticipant
	}
	prepared, err := contestdomain.PrepareClarification(input, false)
	if err != nil {
		return nil, err
	}
	return s.clarifications.CreateClarification(ctx, *prepared)
}

// Reply records a jury answer or announcement. A reply marks its thread as
// answered so the jury queue drains.
func (s *Service) Reply(ctx context.Context, input contestdomain.ClarificationInput) (*contestdomain.Clarification, error) {
	if s.clarifications == nil {
		return nil, contestdomain.ErrClarificationNotFound
	}
	if _, err := s.repository.Get(ctx, input.ContestID); err != nil {
		return nil, err
	}
	prepared, err := contestdomain.PrepareClarification(input, true)
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
func (s *Service) Clarifications(ctx context.Context, contestID, userID, role string) ([]contestdomain.Clarification, error) {
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
			return nil, contestdomain.ErrRegistrationNeeded
		}
	case "private":
		// resolveViewerAccess already admitted only staff, owner or admin.
	default:
		// Unknown persisted states must never become public by accident.
		return nil, contestdomain.ErrNotFound
	}
	return s.clarifications.ListClarifications(ctx, contestID, viewer)
}
