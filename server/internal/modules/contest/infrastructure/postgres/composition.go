package postgres

import (
	"context"
	"errors"
	"sort"
	"time"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres/internal/dbgen"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
)

// SetProblems replaces the contest problem set and rebuilds the scoreboard,
// because changing a problem's point value changes every cell that scored it.
func (s *Repository) SetProblems(ctx context.Context, contestID string, entries []domain.ProblemEntry) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	access, err := LockAccess(ctx, tx, contestID, tenancydomain.ActorID(ctx))
	if err != nil {
		return err
	}
	if !access.Permissions.Edit {
		return domain.ErrForbidden
	}
	if !access.Permissions.ManageAccess && !time.Now().Before(access.BeginAt) {
		return domain.ErrForbidden
	}
	queries := s.queries.WithTx(tx.Tx)
	versions := map[string]int{}
	rows, err := queries.ListContestProblemVersions(ctx, contestID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		versions[row.ProblemID] = row.ProblemVersion
	}

	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ProblemID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		parent, err := problempg.LockAuthorization(ctx, tx, id, access.Scope.UserID)
		if err != nil || (versions[id] == 0 && (!parent.Permissions.View || parent.PublishedVersion == 0)) {
			if err != nil && !errors.Is(err, problemdomain.ErrNotFound) && !errors.Is(err, tenancydomain.ErrForbidden) {
				return err
			}
			return domain.Invalid("one or more problems are unavailable")
		}
		if versions[id] == 0 {
			versions[id] = parent.PublishedVersion
		}
	}

	if err := queries.DeleteContestProblems(ctx, contestID); err != nil {
		return err
	}
	for index, entry := range entries {
		if err := queries.InsertContestProblem(ctx, dbgen.InsertContestProblemParams{ContestID: contestID, ProblemID: entry.ProblemID, SortOrder: index, Label: entry.Label, Color: entry.Color, Points: entry.Points, DomainID: tenancydomain.ID(ctx), ProblemVersion: versions[entry.ProblemID]}); err != nil {
			return err
		}
	}
	// Cells for problems that left the contest are stale; drop them before the
	// rebuild so they cannot linger on the board.
	if err := queries.DeleteRemovedContestCells(ctx, contestID); err != nil {
		return err
	}
	if err := RebuildContest(ctx, tx, contestID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Repository) Register(ctx context.Context, contestID, userID string, verifiedPasswordHash ...string) error {
	tx, err := s.db.Pool.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	access, err := LockAccess(ctx, tx, contestID, userID)
	if err != nil {
		return err
	}
	if !access.Permissions.View {
		return domain.ErrForbidden
	}
	if access.Registered {
		return nil
	}
	if err := domain.RegistrationError(access.AllowSelfRegistration, access.AllowLateRegistration, access.BeginAt, access.EndAt, time.Now()); err != nil {
		return err
	}
	if !access.Permissions.Register {
		return domain.ErrForbidden
	}
	if access.Visibility == "password" && (len(verifiedPasswordHash) != 1 || verifiedPasswordHash[0] != access.PasswordHash) {
		return domain.ErrInvalidPassword
	}
	err = s.queries.WithTx(tx.Tx).RegisterContestParticipant(ctx, dbgen.RegisterContestParticipantParams{ContestID: contestID, UserID: userID, DomainID: tenancydomain.ID(ctx)})
	if err != nil {
		return err
	}
	return tx.Commit()
}
