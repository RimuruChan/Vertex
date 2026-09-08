package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/contest/domain"
	contestpg "github.com/RimuruChan/Vertex/server/internal/contest/infrastructure/postgres"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/submission/domain"
	"github.com/RimuruChan/Vertex/server/internal/submission/infrastructure/postgres/internal/dbgen"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	"github.com/jmoiron/sqlx"
)

// validateSubmissionTarget repeats the service's access decision under the
// same transaction that creates the submission and judge job. The row locks
// close the gap where a contest could end, lose a problem, or revoke a
// participant after the service check but before persistence.
func validateSubmissionTarget(ctx context.Context, tx *sqlx.Tx, sub *submissiondomain.Submission) error {
	if sub.ContestID == nil || *sub.ContestID == "" {
		access, err := problempg.LockAuthorization(ctx, tx, sub.ProblemID, sub.UserID)
		if errors.Is(err, problemdomain.ErrNotFound) {
			return submissiondomain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if !access.Scope.Allows(tenancydomain.CreateSubmission) || !access.Permissions.View {
			return submissiondomain.ErrProblemForbidden
		}
		if access.PublishedVersion == 0 {
			return submissiondomain.ErrProblemUnpublished
		}
		return nil
	}

	access, err := contestpg.LockAuthorization(ctx, tx, *sub.ContestID, sub.UserID)
	if err != nil {
		if errors.Is(err, contestdomain.ErrNotFound) {
			return submissiondomain.ErrNotFound
		}
		return err
	}
	if !access.Permissions.View {
		return submissiondomain.ErrNotFound
	}
	if !access.Permissions.Submit {
		return contestdomain.ErrNotParticipant
	}
	if time.Now().Before(access.BeginAt) || time.Now().After(access.EndAt) {
		return contestdomain.ErrNotActive
	}

	// Problem deletion locks the problem before cascading into
	// contest_problems; use the same order to avoid a delete/submit deadlock.
	_, err = dbgen.New(tx).LockSubmissionProblem(ctx, dbgen.LockSubmissionProblemParams{ProblemID: sub.ProblemID, DomainID: tenancydomain.ID(ctx)})
	if errors.Is(err, sql.ErrNoRows) {
		return contestdomain.ErrProblemNotInContest
	}
	if err != nil {
		return err
	}
	_, err = dbgen.New(tx).LockSubmissionContestProblem(ctx, dbgen.LockSubmissionContestProblemParams{ContestID: *sub.ContestID, ProblemID: sub.ProblemID})
	if errors.Is(err, sql.ErrNoRows) {
		return contestdomain.ErrProblemNotInContest
	}
	return err
}
