package postgres

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres/internal/dbgen"
)

type counterExecer = dbgen.DBTX

// RebuildPracticeCounters refreshes the public problem aggregates from
// standalone practice submissions only. Contest results have their own
// scoreboard and must not leak through public counters while feedback is
// withheld or the board is frozen.
func RebuildPracticeCounters(ctx context.Context, tx counterExecer, problemID string) error {
	if err := dbgen.New(tx).LockPracticeCounters(ctx, "problem:"+problemID); err != nil {
		return err
	}
	return dbgen.New(tx).RebuildPracticeCounters(ctx, problemID)
}
