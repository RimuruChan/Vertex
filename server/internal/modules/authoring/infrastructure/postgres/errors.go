package postgres

import (
	"database/sql"
	"errors"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	tenancy "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
)

func packageReadError(err error) error {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, problemdomain.ErrNotFound) || errors.Is(err, tenancy.ErrNotFound) {
		return domain.ErrNotFound
	}
	return err
}
