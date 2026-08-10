DROP INDEX IF EXISTS idx_problem_tags_tag;
DROP INDEX IF EXISTS idx_submissions_user_accepted;
DROP INDEX IF EXISTS idx_submissions_user_problem_status;

ALTER TABLE submissions DROP CONSTRAINT IF EXISTS submissions_progress_non_negative;
ALTER TABLE submissions DROP COLUMN IF EXISTS total_cases;
ALTER TABLE submissions DROP COLUMN IF EXISTS judged_cases;
