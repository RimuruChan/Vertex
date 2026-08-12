-- Vertex OJ — UX 读模型支撑
--
-- 本迁移只增加判题进度列与读路径索引，不引入冗余物化表：
-- 「用户是否通过某题」由 submissions 直接推导，避免 rejudge / 比赛重算
-- 时出现双写不一致（rebuildProblemCounters 已经是同样的重算思路）。

-- ---------- 判题进度 ----------
-- worker 在续租时上报已判测试点数，前端据此显示 "3 / 10" 而不是空转轮询。
-- total_cases 在派发时就已知（problem_testdata.case_count），判完由最终结果覆盖。
ALTER TABLE submissions
    ADD COLUMN judged_cases INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN total_cases  INTEGER NOT NULL DEFAULT 0;

ALTER TABLE submissions
    ADD CONSTRAINT submissions_progress_non_negative
    CHECK (judged_cases >= 0 AND total_cases >= 0);

-- ---------- 题库个人状态 ----------
-- 题目列表需要对每道题回答「当前用户 已通过 / 尝试过 / 未做」。
-- (user_id, problem_id, status) 让该判定走 index-only scan。
CREATE INDEX idx_submissions_user_problem_status
    ON submissions (user_id, problem_id, status);

-- 已通过题目集合是个人主页和题库过滤的高频查询，单独给一个部分索引。
CREATE INDEX idx_submissions_user_accepted
    ON submissions (user_id, problem_id)
    WHERE status = 'Accepted';

-- ---------- 标签目录 ----------
-- GET /api/tags 按题目数量倒序列出标签。
CREATE INDEX idx_problem_tags_tag ON problem_tags (tag_id);
