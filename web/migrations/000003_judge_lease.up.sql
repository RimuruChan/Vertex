ALTER TABLE submissions
    ADD COLUMN judge_started_at TIMESTAMPTZ;

CREATE INDEX idx_submissions_judge_lease
    ON submissions (judge_started_at)
    WHERE status = 'Judging';
