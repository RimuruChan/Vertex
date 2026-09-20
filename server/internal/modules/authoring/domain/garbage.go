package domain

import (
	"context"
	"time"
)

type StorageReferences struct{ Blobs, Artifacts map[string]bool }
type SweepResult struct {
	Objects int
	Bytes   int64
}
type GarbageStorage interface {
	Namespaces(context.Context) ([]string, error)
	Sweep(context.Context, string, StorageReferences, time.Time) (SweepResult, error)
}
type GarbageStats struct {
	Namespaces, Busy              int
	Imports, Trees, Blobs, Grants int64
	Objects                       int
	Bytes                         int64
	Next                          string
}
type GarbageRepository interface {
	Collect(context.Context, time.Time, string, int) (GarbageStats, error)
}
