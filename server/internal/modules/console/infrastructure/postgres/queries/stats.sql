-- name: GetSiteStats :one
SELECT (SELECT count(*) FROM users)::integer AS users,
 (SELECT count(*) FROM users WHERE created_at>=now()-interval '1 day')::integer AS users_today,
 (SELECT count(*) FROM problems)::integer AS problems,
 (SELECT count(*) FROM problems WHERE visibility='public' AND published_version IS NOT NULL)::integer AS public_problems,
 (SELECT count(*) FROM submissions)::integer AS submissions,
 (SELECT count(*) FROM submissions WHERE submitted_at>=now()-interval '1 day')::integer AS submissions_today,
 (SELECT count(*) FROM contests)::integer AS contests,
 (SELECT count(*) FROM contests WHERE now() BETWEEN begin_at AND end_at)::integer AS running_contests,
 (SELECT count(*) FROM editorials WHERE status='published')::integer AS editorials,
 (SELECT count(*) FROM problem_sets)::integer AS problem_sets;

-- name: GetJudgeQueueStats :one
SELECT count(*) FILTER(WHERE state='queued')::integer AS queued_jobs,
 count(*) FILTER(WHERE state='running')::integer AS running_jobs,
 count(*) FILTER(WHERE state='dead')::integer AS dead_jobs,
 COALESCE(min(created_at) FILTER(WHERE state='queued'),TIMESTAMPTZ 'epoch')::timestamptz AS oldest_queued,
 count(DISTINCT worker_id) FILTER(WHERE state='running' AND lease_expires_at>=now())::integer AS active_workers
FROM judge_jobs;

-- name: ListRecentVerdictCounts :many
SELECT status,count(*)::integer AS count FROM submissions WHERE submitted_at>=now()-interval '1 day' GROUP BY status ORDER BY count(*) DESC;
