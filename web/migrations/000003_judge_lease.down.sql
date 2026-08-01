DROP INDEX IF EXISTS idx_submissions_judge_lease;
ALTER TABLE submissions DROP COLUMN IF EXISTS judge_started_at;
