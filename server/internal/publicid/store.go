// Package publicid resolves stable public numbers to internal resource UUIDs.
// Resolution is not authorization: callers must still run the domain policy.
package publicid

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/domain"
)

var ErrNotFound = errors.New("public resource not found")

var tables = map[string]string{
	"problems": "problems", "contests": "contests", "submissions": "submissions",
	"editorials": "editorials", "problem-sets": "problem_sets", "announcements": "announcements",
}

type Store struct{ db *database.DB }

func NewStore(db *database.DB) *Store { return &Store{db: db} }

func IsNumber(ref string) bool {
	if ref == "" {
		return false
	}
	for _, digit := range ref {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func (s *Store) Resolve(ctx context.Context, kind, ref string) (string, error) {
	table, ok := tables[kind]
	number, err := strconv.ParseInt(ref, 10, 64)
	if !ok || err != nil || number <= 0 {
		return "", ErrNotFound
	}
	var id string
	// Table names are constants selected from the closed resource catalogue.
	err = s.db.Pool.GetContext(ctx, &id, "SELECT id FROM "+table+" WHERE public_id = $1 AND domain_id = $2", number, domain.ID(ctx))
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}
