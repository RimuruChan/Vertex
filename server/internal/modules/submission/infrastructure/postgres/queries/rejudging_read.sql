-- name: GetRejudgingProgress :one
SELECT r.id, r.contest_id, r.problem_id, r.reason, r.state, r.total_count,
	r.created_by, r.created_at, r.finished_at,
 COALESCE((SELECT public_id::text FROM contests WHERE id=r.contest_id),'')::text AS contest_number,
 COALESCE((SELECT public_id::text FROM problems WHERE id=r.problem_id),'')::text AS problem_number,
	(SELECT count(*) FROM rejudging_submissions AS member
	   JOIN submissions AS sub ON sub.id = member.submission_id
       JOIN judgements evaluation ON evaluation.submission_id=member.submission_id AND evaluation.generation=member.generation
       JOIN judgements prior ON prior.submission_id=member.submission_id AND prior.generation=member.prior_generation
	   WHERE member.rejudging_id = r.id
	     AND (sub.judge_generation > member.generation
	          OR (sub.judge_generation = member.generation AND evaluation.judged_at IS NOT NULL)))::integer AS done_count,
	(SELECT count(*) FROM rejudging_submissions AS member
	   JOIN submissions AS sub ON sub.id = member.submission_id
       JOIN judgements evaluation ON evaluation.submission_id=member.submission_id AND evaluation.generation=member.generation
       JOIN judgements prior ON prior.submission_id=member.submission_id AND prior.generation=member.prior_generation
	   WHERE member.rejudging_id = r.id
	     AND evaluation.judged_at IS NOT NULL
	     AND (evaluation.status <> prior.status OR evaluation.score <> prior.score))::integer AS changed_count FROM rejudgings AS r WHERE r.id = sqlc.arg(batch_id) AND r.domain_id = sqlc.arg(domain_id);

-- name: ListRejudgings :many
SELECT r.id, r.contest_id, r.problem_id, r.reason, r.state, r.total_count,
	r.created_by, r.created_at, r.finished_at,
 COALESCE((SELECT public_id::text FROM contests WHERE id=r.contest_id),'')::text AS contest_number,
 COALESCE((SELECT public_id::text FROM problems WHERE id=r.problem_id),'')::text AS problem_number,
	(SELECT count(*) FROM rejudging_submissions AS member
	   JOIN submissions AS sub ON sub.id = member.submission_id
       JOIN judgements evaluation ON evaluation.submission_id=member.submission_id AND evaluation.generation=member.generation
       JOIN judgements prior ON prior.submission_id=member.submission_id AND prior.generation=member.prior_generation
	   WHERE member.rejudging_id = r.id
	     AND (sub.judge_generation > member.generation
	          OR (sub.judge_generation = member.generation AND evaluation.judged_at IS NOT NULL)))::integer AS done_count,
	(SELECT count(*) FROM rejudging_submissions AS member
	   JOIN submissions AS sub ON sub.id = member.submission_id
       JOIN judgements evaluation ON evaluation.submission_id=member.submission_id AND evaluation.generation=member.generation
       JOIN judgements prior ON prior.submission_id=member.submission_id AND prior.generation=member.prior_generation
	   WHERE member.rejudging_id = r.id
	     AND evaluation.judged_at IS NOT NULL
	     AND (evaluation.status <> prior.status OR evaluation.score <> prior.score))::integer AS changed_count FROM rejudgings AS r
		 WHERE (sqlc.arg(contest_filter)::text = '' OR r.contest_id = NULLIF(sqlc.arg(contest_filter)::text, '')::uuid) AND r.domain_id = sqlc.arg(domain_id)
		 AND (sqlc.arg(can_manage)::boolean OR (sqlc.arg(active_member)::boolean AND (
		   EXISTS(SELECT 1 FROM contests c WHERE c.id=r.contest_id AND (c.owner_id=sqlc.arg(viewer_id) OR EXISTS(SELECT 1 FROM contest_staff s WHERE s.contest_id=c.id AND s.user_id=sqlc.arg(viewer_id))))
		   OR (r.contest_id IS NULL AND EXISTS(SELECT 1 FROM problems p WHERE p.id=r.problem_id AND (p.owner_id=sqlc.arg(viewer_id) OR EXISTS(
		     SELECT 1 FROM problem_access a WHERE a.problem_id=p.id AND (a.user_id=sqlc.arg(viewer_id) OR a.group_id IN (SELECT group_id FROM domain_group_members WHERE domain_id=sqlc.arg(domain_id) AND user_id=sqlc.arg(viewer_id)))))))
		 ))) ORDER BY r.created_at DESC LIMIT sqlc.arg(page_limit)::integer;

-- name: FinishRejudging :one
UPDATE rejudgings SET state = 'finished', finished_at = now()
		 WHERE id = sqlc.arg(batch_id) AND state = 'running' AND domain_id = sqlc.arg(domain_id)
		 RETURNING state, finished_at;

-- name: ListRejudgingChanges :many
SELECT member.submission_id, sub.public_id::text AS submission_number,u.username, p.title AS problem_title,
		        prior.status AS prior_status, prior.score AS prior_score,
		        evaluation.status, evaluation.score,
		        (evaluation.judged_at IS NOT NULL)::boolean AS judged
		 FROM rejudging_submissions AS member
		 JOIN submissions AS sub ON sub.id = member.submission_id
       JOIN judgements evaluation ON evaluation.submission_id=member.submission_id AND evaluation.generation=member.generation
       JOIN judgements prior ON prior.submission_id=member.submission_id AND prior.generation=member.prior_generation
		 JOIN users AS u ON u.id = sub.user_id
		 JOIN problem_versions AS p ON p.problem_id=evaluation.problem_id AND p.version_no=evaluation.problem_version
		 WHERE member.rejudging_id = sqlc.arg(batch_id) AND member.domain_id = sqlc.arg(domain_id)
		   AND (evaluation.status <> prior.status OR evaluation.score <> prior.score)
         AND EXISTS(SELECT 1 FROM judge_jobs job WHERE job.submission_id=member.submission_id AND job.generation=member.generation AND job.state<>'cancelled')
         ORDER BY sub.submitted_at
		 LIMIT sqlc.arg(page_limit)::integer;
