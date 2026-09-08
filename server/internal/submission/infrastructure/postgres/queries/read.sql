-- name: CountVisibleSubmissions :one
SELECT count(*) FROM submissions s JOIN users u ON u.id=s.user_id JOIN problems p ON p.id=s.problem_id
WHERE s.domain_id=sqlc.arg(domain_id)::uuid AND (
		sqlc.arg(is_manager)::boolean
		OR s.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
		OR (
			s.contest_id IS NULL
			AND ((p.visibility = 'public' AND p.published_version IS NOT NULL) OR (sqlc.arg(active_member)::boolean AND (p.owner_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR EXISTS (
			 SELECT 1 FROM problem_access a WHERE a.problem_id=p.id AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (
			 SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))))))
		)
		OR EXISTS (
			SELECT 1 FROM contests c
			WHERE c.id = s.contest_id
			  AND (
				(sqlc.arg(active_member)::boolean AND c.owner_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
				OR EXISTS (
					SELECT 1 FROM contest_staff staff
					WHERE staff.contest_id = c.id AND staff.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
				)
				OR (
					c.end_at < now()
					AND c.rankboard_visible
					AND NOT (
						c.freeze_at IS NOT NULL
						AND now() > c.freeze_at
						AND (c.unfreeze_at IS NULL OR now() < c.unfreeze_at)
					)
					AND (
						(c.visibility = 'public' AND (
							p.visibility = 'public'
							OR EXISTS (
								SELECT 1 FROM contest_participants participant
								WHERE participant.contest_id = c.id
								  AND participant.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
							)
						))
						OR (c.visibility = 'password' AND EXISTS (
							SELECT 1 FROM contest_participants participant
							WHERE participant.contest_id = c.id
							  AND participant.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
						))
					)
				)
			  )
		)
	)
AND (sqlc.arg(user_filter)::text='' OR s.user_id::text=sqlc.arg(user_filter)::text OR u.username=sqlc.arg(user_filter)::text)
AND (sqlc.arg(problem_filter)::text='' OR s.problem_id=NULLIF(sqlc.arg(problem_filter)::text,'')::uuid)
AND (sqlc.arg(contest_filter)::text='' OR s.contest_id=NULLIF(sqlc.arg(contest_filter)::text,'')::uuid)
AND (sqlc.arg(language_filter)::text='' OR s.language=sqlc.arg(language_filter)::text)
AND (sqlc.arg(status_filter)::text='' OR (CASE WHEN s.status NOT IN ('Pending','Judging') AND EXISTS (
	 SELECT 1 FROM contests feedback_contest WHERE feedback_contest.id=s.contest_id
	 AND feedback_contest.feedback='none' AND feedback_contest.end_at>=now()
	 AND NOT (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND feedback_contest.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
	 OR EXISTS(SELECT 1 FROM contest_staff staff WHERE staff.contest_id=feedback_contest.id AND staff.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))
	) THEN 'Submitted' ELSE s.status END)=sqlc.arg(status_filter)::text);

-- name: ListVisibleSubmissions :many
SELECT s.id,s.public_id,s.user_id,u.username,s.problem_id,p.public_id AS problem_public_id,v.title AS problem_title,s.language,s.status,s.score,s.total_time_ms,s.peak_memory_kb,s.judged_cases,s.total_cases,s.contest_id,COALESCE(c.public_id::text,'')::text AS contest_public_id,s.submitted_at,s.problem_version FROM submissions s JOIN users u ON u.id=s.user_id JOIN problems p ON p.id=s.problem_id JOIN problem_versions v ON v.problem_id=s.problem_id AND v.version_no=s.problem_version LEFT JOIN contests c ON c.id=s.contest_id
WHERE s.domain_id=sqlc.arg(domain_id)::uuid AND (
		sqlc.arg(is_manager)::boolean
		OR s.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
		OR (
			s.contest_id IS NULL
			AND ((p.visibility = 'public' AND p.published_version IS NOT NULL) OR (sqlc.arg(active_member)::boolean AND (p.owner_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR EXISTS (
			 SELECT 1 FROM problem_access a WHERE a.problem_id=p.id AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (
			 SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))))))
		)
		OR EXISTS (
			SELECT 1 FROM contests c
			WHERE c.id = s.contest_id
			  AND (
				(sqlc.arg(active_member)::boolean AND c.owner_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
				OR EXISTS (
					SELECT 1 FROM contest_staff staff
					WHERE staff.contest_id = c.id AND staff.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
				)
				OR (
					c.end_at < now()
					AND c.rankboard_visible
					AND NOT (
						c.freeze_at IS NOT NULL
						AND now() > c.freeze_at
						AND (c.unfreeze_at IS NULL OR now() < c.unfreeze_at)
					)
					AND (
						(c.visibility = 'public' AND (
							p.visibility = 'public'
							OR EXISTS (
								SELECT 1 FROM contest_participants participant
								WHERE participant.contest_id = c.id
								  AND participant.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
							)
						))
						OR (c.visibility = 'password' AND EXISTS (
							SELECT 1 FROM contest_participants participant
							WHERE participant.contest_id = c.id
							  AND participant.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
						))
					)
				)
			  )
		)
	)
AND (sqlc.arg(user_filter)::text='' OR s.user_id::text=sqlc.arg(user_filter)::text OR u.username=sqlc.arg(user_filter)::text)
AND (sqlc.arg(problem_filter)::text='' OR s.problem_id=NULLIF(sqlc.arg(problem_filter)::text,'')::uuid)
AND (sqlc.arg(contest_filter)::text='' OR s.contest_id=NULLIF(sqlc.arg(contest_filter)::text,'')::uuid)
AND (sqlc.arg(language_filter)::text='' OR s.language=sqlc.arg(language_filter)::text)
AND (sqlc.arg(status_filter)::text='' OR (CASE WHEN s.status NOT IN ('Pending','Judging') AND EXISTS (
	 SELECT 1 FROM contests feedback_contest WHERE feedback_contest.id=s.contest_id
	 AND feedback_contest.feedback='none' AND feedback_contest.end_at>=now()
	 AND NOT (sqlc.arg(is_manager)::boolean OR (sqlc.arg(active_member)::boolean AND feedback_contest.owner_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
	 OR EXISTS(SELECT 1 FROM contest_staff staff WHERE staff.contest_id=feedback_contest.id AND staff.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))
	) THEN 'Submitted' ELSE s.status END)=sqlc.arg(status_filter)::text)
ORDER BY s.submitted_at DESC,s.id DESC LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: GetVisibleSubmission :one
SELECT s.id,s.public_id,s.user_id,u.username,s.problem_id,p.public_id AS problem_public_id,v.title AS problem_title,s.language,s.status,s.score,s.total_time_ms,s.peak_memory_kb,s.judged_cases,s.total_cases,s.contest_id,COALESCE(c.public_id::text,'')::text AS contest_public_id,s.submitted_at,s.problem_version,s.source_code,s.compile_result,s.case_results,s.judged_at FROM submissions s JOIN users u ON u.id=s.user_id JOIN problems p ON p.id=s.problem_id JOIN problem_versions v ON v.problem_id=s.problem_id AND v.version_no=s.problem_version LEFT JOIN contests c ON c.id=s.contest_id
WHERE s.id=sqlc.arg(submission_id)::uuid AND s.domain_id=sqlc.arg(domain_id)::uuid AND (
		sqlc.arg(is_manager)::boolean
		OR s.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
		OR (
			s.contest_id IS NULL
			AND ((p.visibility = 'public' AND p.published_version IS NOT NULL) OR (sqlc.arg(active_member)::boolean AND (p.owner_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR EXISTS (
			 SELECT 1 FROM problem_access a WHERE a.problem_id=p.id AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (
			 SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))))))
		)
		OR EXISTS (
			SELECT 1 FROM contests c
			WHERE c.id = s.contest_id
			  AND (
				(sqlc.arg(active_member)::boolean AND c.owner_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
				OR EXISTS (
					SELECT 1 FROM contest_staff staff
					WHERE staff.contest_id = c.id AND staff.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
				)
				OR (
					c.end_at < now()
					AND c.rankboard_visible
					AND NOT (
						c.freeze_at IS NOT NULL
						AND now() > c.freeze_at
						AND (c.unfreeze_at IS NULL OR now() < c.unfreeze_at)
					)
					AND (
						(c.visibility = 'public' AND (
							p.visibility = 'public'
							OR EXISTS (
								SELECT 1 FROM contest_participants participant
								WHERE participant.contest_id = c.id
								  AND participant.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
							)
						))
						OR (c.visibility = 'password' AND EXISTS (
							SELECT 1 FROM contest_participants participant
							WHERE participant.contest_id = c.id
							  AND participant.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
						))
					)
				)
			  )
		)
	);

-- name: GetVisibleProgress :one
SELECT s.id,s.user_id,s.contest_id,s.status,s.score,s.total_time_ms,s.peak_memory_kb,s.compile_result,s.case_results,s.judged_cases,s.total_cases FROM submissions s JOIN problems p ON p.id=s.problem_id
WHERE s.id=sqlc.arg(submission_id)::uuid AND s.domain_id=sqlc.arg(domain_id)::uuid AND (
		sqlc.arg(is_manager)::boolean
		OR s.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
		OR (
			s.contest_id IS NULL
			AND ((p.visibility = 'public' AND p.published_version IS NOT NULL) OR (sqlc.arg(active_member)::boolean AND (p.owner_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR EXISTS (
			 SELECT 1 FROM problem_access a WHERE a.problem_id=p.id AND (a.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid OR a.group_id IN (
			 SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid))))))
		)
		OR EXISTS (
			SELECT 1 FROM contests c
			WHERE c.id = s.contest_id
			  AND (
				(sqlc.arg(active_member)::boolean AND c.owner_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid)
				OR EXISTS (
					SELECT 1 FROM contest_staff staff
					WHERE staff.contest_id = c.id AND staff.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
				)
				OR (
					c.end_at < now()
					AND c.rankboard_visible
					AND NOT (
						c.freeze_at IS NOT NULL
						AND now() > c.freeze_at
						AND (c.unfreeze_at IS NULL OR now() < c.unfreeze_at)
					)
					AND (
						(c.visibility = 'public' AND (
							p.visibility = 'public'
							OR EXISTS (
								SELECT 1 FROM contest_participants participant
								WHERE participant.contest_id = c.id
								  AND participant.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
							)
						))
						OR (c.visibility = 'password' AND EXISTS (
							SELECT 1 FROM contest_participants participant
							WHERE participant.contest_id = c.id
							  AND participant.user_id = NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
						))
					)
				)
			  )
		)
	);
