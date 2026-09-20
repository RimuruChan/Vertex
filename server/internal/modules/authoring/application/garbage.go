package application

import (
	"context"
	"log/slog"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// RunGarbageCollector is a maintenance loop, not an HTTP capability. Each pass
// is bounded and each namespace excludes storage writers until removal finishes.
func RunGarbageCollector(ctx context.Context, repository domain.GarbageRepository, interval, grace time.Duration) {
	if interval <= 0 || grace < time.Hour {
		slog.Error("invalid authoring collection policy")
		return
	}
	run := func() {
		pass, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		cutoff := time.Now().Add(-grace)
		cursor := ""
		total := domain.GarbageStats{}
		for {
			stats, err := repository.Collect(pass, cutoff, cursor, 32)
			total.Objects += stats.Objects
			total.Bytes += stats.Bytes
			total.Trees += stats.Trees
			total.Blobs += stats.Blobs
			total.Imports += stats.Imports
			total.Grants += stats.Grants
			if err != nil && ctx.Err() == nil {
				slog.Warn("authoring collection incomplete", "error", err)
			}
			if pass.Err() != nil || stats.Next == "" {
				break
			}
			cursor = stats.Next
		}
		if total.Objects+int(total.Trees+total.Blobs+total.Imports+total.Grants) > 0 {
			slog.Info("authoring content collected", "objects", total.Objects, "bytes", total.Bytes, "trees", total.Trees, "blobs", total.Blobs, "imports", total.Imports, "temporary_grants", total.Grants)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
