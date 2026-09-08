package database

import (
	"database/sql"
	"time"
)

// Ptr adapts required domain values to nullable database columns.
func Ptr[T any](value T) *T { return &value }

func NullTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *value, Valid: true}
}

func NullString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func Int64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func StringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
