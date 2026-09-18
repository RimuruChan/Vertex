-- name: CountVisibleSubmissions :one
SELECT count(*) FROM visible_submissions(sqlc.arg(domain_id)::uuid,sqlc.arg(viewer_id)::text,sqlc.arg(is_manager)::boolean,sqlc.arg(active_member)::boolean,sqlc.arg(can_submit)::boolean,sqlc.arg(as_of)::timestamptz) s JOIN users u ON u.id=s.user_id JOIN problems p ON p.id=s.problem_id
WHERE (sqlc.arg(user_filter)::text='' OR s.user_id::text=sqlc.arg(user_filter)::text OR u.username=sqlc.arg(user_filter)::text)
AND (sqlc.arg(problem_filter)::text='' OR s.problem_id=NULLIF(sqlc.arg(problem_filter)::text,'')::uuid)
AND (sqlc.arg(contest_filter)::text='' OR s.contest_id=NULLIF(sqlc.arg(contest_filter)::text,'')::uuid)
AND (sqlc.arg(language_filter)::text='' OR s.language=sqlc.arg(language_filter)::text)
AND (sqlc.arg(status_filter)::text='' OR (CASE WHEN EXISTS (SELECT 1 FROM contests frozen_contest
 WHERE frozen_contest.id=s.contest_id AND frozen_contest.domain_id=s.domain_id
 AND frozen_contest.rule <> 'oi' AND frozen_contest.freeze_at IS NOT NULL AND sqlc.arg(as_of)::timestamptz > frozen_contest.freeze_at
 AND s.submitted_at >= frozen_contest.freeze_at
 AND (frozen_contest.unfreeze_at IS NULL OR sqlc.arg(as_of)::timestamptz < frozen_contest.unfreeze_at)
 AND s.user_id <> NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
 AND NOT (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND frozen_contest.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
 OR EXISTS(SELECT 1 FROM contest_staff staff WHERE staff.contest_id=frozen_contest.id AND staff.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))) THEN 'Pending' WHEN s.status NOT IN ('Pending','Judging') AND EXISTS (
	 SELECT 1 FROM contests feedback_contest WHERE feedback_contest.id=s.contest_id
	 AND (feedback_contest.rule='oi' OR feedback_contest.feedback='none') AND feedback_contest.end_at>=sqlc.arg(as_of)::timestamptz
	 AND NOT (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND feedback_contest.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
	 OR EXISTS(SELECT 1 FROM contest_staff staff WHERE staff.contest_id=feedback_contest.id AND staff.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))
	) THEN 'Submitted' ELSE s.status END)=sqlc.arg(status_filter)::text);

-- name: ListVisibleSubmissions :many
SELECT (EXISTS (SELECT 1 FROM contests frozen_contest
 WHERE frozen_contest.id=s.contest_id AND frozen_contest.domain_id=s.domain_id
 AND frozen_contest.rule <> 'oi' AND frozen_contest.freeze_at IS NOT NULL AND sqlc.arg(as_of)::timestamptz > frozen_contest.freeze_at
 AND s.submitted_at >= frozen_contest.freeze_at
 AND (frozen_contest.unfreeze_at IS NULL OR sqlc.arg(as_of)::timestamptz < frozen_contest.unfreeze_at)
 AND s.user_id <> NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
 AND NOT (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND frozen_contest.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
 OR EXISTS(SELECT 1 FROM contest_staff staff WHERE staff.contest_id=frozen_contest.id AND staff.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)))) AS frozen_result, s.id,s.public_id,s.user_id,u.username,s.problem_id,p.public_id AS problem_public_id,v.title AS problem_title,s.language,s.status,s.score,s.total_time_ms,s.peak_memory_kb,s.judged_cases,s.total_cases,s.contest_id,COALESCE(c.public_id::text,'')::text AS contest_public_id,s.submitted_at,s.problem_version FROM visible_submissions(sqlc.arg(domain_id)::uuid,sqlc.arg(viewer_id)::text,sqlc.arg(is_manager)::boolean,sqlc.arg(active_member)::boolean,sqlc.arg(can_submit)::boolean,sqlc.arg(as_of)::timestamptz) s JOIN users u ON u.id=s.user_id JOIN problems p ON p.id=s.problem_id JOIN problem_versions v ON v.problem_id=s.problem_id AND v.version_no=s.problem_version LEFT JOIN contests c ON c.id=s.contest_id
WHERE (sqlc.arg(user_filter)::text='' OR s.user_id::text=sqlc.arg(user_filter)::text OR u.username=sqlc.arg(user_filter)::text)
AND (sqlc.arg(problem_filter)::text='' OR s.problem_id=NULLIF(sqlc.arg(problem_filter)::text,'')::uuid)
AND (sqlc.arg(contest_filter)::text='' OR s.contest_id=NULLIF(sqlc.arg(contest_filter)::text,'')::uuid)
AND (sqlc.arg(language_filter)::text='' OR s.language=sqlc.arg(language_filter)::text)
AND (sqlc.arg(status_filter)::text='' OR (CASE WHEN EXISTS (SELECT 1 FROM contests frozen_contest
 WHERE frozen_contest.id=s.contest_id AND frozen_contest.domain_id=s.domain_id
 AND frozen_contest.rule <> 'oi' AND frozen_contest.freeze_at IS NOT NULL AND sqlc.arg(as_of)::timestamptz > frozen_contest.freeze_at
 AND s.submitted_at >= frozen_contest.freeze_at
 AND (frozen_contest.unfreeze_at IS NULL OR sqlc.arg(as_of)::timestamptz < frozen_contest.unfreeze_at)
 AND s.user_id <> NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
 AND NOT (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND frozen_contest.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
 OR EXISTS(SELECT 1 FROM contest_staff staff WHERE staff.contest_id=frozen_contest.id AND staff.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))) THEN 'Pending' WHEN s.status NOT IN ('Pending','Judging') AND EXISTS (
	 SELECT 1 FROM contests feedback_contest WHERE feedback_contest.id=s.contest_id
	 AND (feedback_contest.rule='oi' OR feedback_contest.feedback='none') AND feedback_contest.end_at>=sqlc.arg(as_of)::timestamptz
	 AND NOT (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND feedback_contest.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
	 OR EXISTS(SELECT 1 FROM contest_staff staff WHERE staff.contest_id=feedback_contest.id AND staff.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))
	) THEN 'Submitted' ELSE s.status END)=sqlc.arg(status_filter)::text)
ORDER BY s.submitted_at DESC,s.id DESC LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: GetVisibleSubmission :one
SELECT (EXISTS (SELECT 1 FROM contests frozen_contest
 WHERE frozen_contest.id=s.contest_id AND frozen_contest.domain_id=s.domain_id
 AND frozen_contest.rule <> 'oi' AND frozen_contest.freeze_at IS NOT NULL AND sqlc.arg(as_of)::timestamptz > frozen_contest.freeze_at
 AND s.submitted_at >= frozen_contest.freeze_at
 AND (frozen_contest.unfreeze_at IS NULL OR sqlc.arg(as_of)::timestamptz < frozen_contest.unfreeze_at)
 AND s.user_id <> NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
 AND NOT (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND frozen_contest.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
 OR EXISTS(SELECT 1 FROM contest_staff staff WHERE staff.contest_id=frozen_contest.id AND staff.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)))) AS frozen_result, s.id,s.public_id,s.user_id,u.username,s.problem_id,p.public_id AS problem_public_id,v.title AS problem_title,s.language,s.status,s.score,s.total_time_ms,s.peak_memory_kb,s.judged_cases,s.total_cases,s.contest_id,COALESCE(c.public_id::text,'')::text AS contest_public_id,s.submitted_at,s.problem_version,s.source_code,s.compile_result,s.case_results,s.judged_at FROM visible_submissions(sqlc.arg(domain_id)::uuid,sqlc.arg(viewer_id)::text,sqlc.arg(is_manager)::boolean,sqlc.arg(active_member)::boolean,sqlc.arg(can_submit)::boolean,sqlc.arg(as_of)::timestamptz) s JOIN users u ON u.id=s.user_id JOIN problems p ON p.id=s.problem_id JOIN problem_versions v ON v.problem_id=s.problem_id AND v.version_no=s.problem_version LEFT JOIN contests c ON c.id=s.contest_id
WHERE s.id=sqlc.arg(submission_id)::uuid ;

-- name: GetVisibleProgress :one
SELECT (EXISTS (SELECT 1 FROM contests frozen_contest
 WHERE frozen_contest.id=s.contest_id AND frozen_contest.domain_id=s.domain_id
 AND frozen_contest.rule <> 'oi' AND frozen_contest.freeze_at IS NOT NULL AND sqlc.arg(as_of)::timestamptz > frozen_contest.freeze_at
 AND s.submitted_at >= frozen_contest.freeze_at
 AND (frozen_contest.unfreeze_at IS NULL OR sqlc.arg(as_of)::timestamptz < frozen_contest.unfreeze_at)
 AND s.user_id <> NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
 AND NOT (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND frozen_contest.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
 OR EXISTS(SELECT 1 FROM contest_staff staff WHERE staff.contest_id=frozen_contest.id AND staff.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)))) AS frozen_result, s.id,s.public_id,s.user_id,s.problem_id,s.contest_id,s.status,s.score,s.total_time_ms,s.peak_memory_kb,s.compile_result,s.case_results,s.judged_cases,s.total_cases FROM visible_submissions(sqlc.arg(domain_id)::uuid,sqlc.arg(viewer_id)::text,sqlc.arg(is_manager)::boolean,sqlc.arg(active_member)::boolean,sqlc.arg(can_submit)::boolean,sqlc.arg(as_of)::timestamptz) s JOIN problems p ON p.id=s.problem_id
WHERE s.id=sqlc.arg(submission_id)::uuid ;
