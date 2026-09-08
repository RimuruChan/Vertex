package postgres

import (
	"context"

	domain "github.com/RimuruChan/Vertex/server/internal/modules/console/domain"
)

func (r *Repository) Stats(ctx context.Context) (*domain.Stats, error) {
	site, err := r.queries.GetSiteStats(ctx)
	if err != nil {
		return nil, err
	}
	queue, err := r.queries.GetJudgeQueueStats(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := r.queries.ListRecentVerdictCounts(ctx)
	if err != nil {
		return nil, err
	}
	stats := &domain.Stats{Users: site.Users, UsersToday: site.UsersToday, Problems: site.Problems, PublicProblems: site.PublicProblems,
		Submissions: site.Submissions, SubmissionsToday: site.SubmissionsToday, Contests: site.Contests, RunningContests: site.RunningContests,
		Editorials: site.Editorials, ProblemSets: site.ProblemSets, QueuedJobs: queue.QueuedJobs, RunningJobs: queue.RunningJobs,
		DeadJobs: queue.DeadJobs, ActiveWorkers: queue.ActiveWorkers, VerdictBreakdown: make([]domain.VerdictCount, 0, len(rows))}
	// Count and minimum come from the same snapshot. An empty queue has no oldest job.
	if queue.QueuedJobs > 0 {
		stats.OldestQueued = &queue.OldestQueued
	}
	for _, row := range rows {
		stats.VerdictBreakdown = append(stats.VerdictBreakdown, domain.VerdictCount{Verdict: row.Status, Count: row.Count})
	}
	return stats, nil
}
