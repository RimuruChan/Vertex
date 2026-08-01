-- 比赛积分格记录函数:判题完成后由 judge worker 调用。
-- 幂等规则(与 web/internal/store/contest.go 的 Go 实现一致):
--   - 只认首次 AC(已 AC 后不再更新任何字段)
--   - 非 AC 的 attempts 仅在未 AC 时递增
-- rejudge 后重复调用不会污染积分。
CREATE OR REPLACE FUNCTION record_contest_submission(
    p_contest_id UUID,
    p_user_id UUID,
    p_problem_id UUID,
    p_submitted_at TIMESTAMPTZ,
    p_accepted BOOLEAN
) RETURNS VOID AS $$
DECLARE
    v_begin TIMESTAMPTZ;
    v_end   TIMESTAMPTZ;
    v_solve_sec INTEGER;
BEGIN
    SELECT begin_at, end_at INTO v_begin, v_end FROM contests WHERE id = p_contest_id;
    -- 只在比赛时间内计入榜单
    IF p_submitted_at < v_begin OR p_submitted_at > v_end THEN
        RETURN;
    END IF;

    IF p_accepted THEN
        v_solve_sec := EXTRACT(EPOCH FROM (p_submitted_at - v_begin))::INTEGER;
        INSERT INTO contest_submission_cells (contest_id, user_id, problem_id, attempts, penalty_sec, solved_at)
        VALUES (p_contest_id, p_user_id, p_problem_id, 1, v_solve_sec, p_submitted_at)
        ON CONFLICT (contest_id, user_id, problem_id) DO UPDATE SET
            attempts = CASE WHEN contest_submission_cells.solved_at IS NULL
                            THEN contest_submission_cells.attempts + 1
                            ELSE contest_submission_cells.attempts END,
            penalty_sec = CASE WHEN contest_submission_cells.solved_at IS NULL
                               THEN v_solve_sec + contest_submission_cells.attempts * 1200
                               ELSE contest_submission_cells.penalty_sec END,
            solved_at = CASE WHEN contest_submission_cells.solved_at IS NULL
                             THEN EXCLUDED.solved_at
                             ELSE contest_submission_cells.solved_at END;
    ELSE
        INSERT INTO contest_submission_cells (contest_id, user_id, problem_id, attempts)
        VALUES (p_contest_id, p_user_id, p_problem_id, 1)
        ON CONFLICT (contest_id, user_id, problem_id) DO UPDATE SET
            attempts = CASE WHEN contest_submission_cells.solved_at IS NULL
                            THEN contest_submission_cells.attempts + 1
                            ELSE contest_submission_cells.attempts END;
    END IF;
END;
$$ LANGUAGE plpgsql;
