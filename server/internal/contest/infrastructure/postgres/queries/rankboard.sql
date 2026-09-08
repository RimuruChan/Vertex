-- name: ListRankboardParticipants :many
SELECT participant.user_id, u.username
		 FROM contest_participants AS participant
		 JOIN users u ON u.id = participant.user_id
		 WHERE participant.contest_id = sqlc.arg(contest_id)::uuid;

-- name: ListRankboardCells :many
SELECT user_id, problem_id, attempts, penalty_sec, score, solved_at,
		        public_attempts, public_penalty_sec, public_score, public_solved_at,
		        pending_count, last_submit_at
		 FROM contest_submission_cells WHERE contest_submission_cells.contest_id = sqlc.arg(contest_id)::uuid;
