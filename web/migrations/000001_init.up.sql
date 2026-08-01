-- Vertex OJ — 初始 Schema
-- PostgreSQL 16,使用 pgcrypto 的 gen_random_uuid() 生成 UUID
--
-- 表创建顺序注意:FK 只能引用已存在的表,因此依赖顺序为:
--   users → problems(+tags/testdata/versions) → contests(+关联) → submissions → 社区表

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------- Identity ----------
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT NOT NULL UNIQUE,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    rating        INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_username ON users (username);

-- ---------- Problems ----------
CREATE TABLE problems (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title               TEXT NOT NULL,
    statement_md        TEXT NOT NULL DEFAULT '',
    difficulty          INTEGER NOT NULL DEFAULT 1,   -- 1-10
    source              TEXT NOT NULL DEFAULT '',
    time_limit_ms       INTEGER NOT NULL DEFAULT 1000,
    memory_limit_kb     INTEGER NOT NULL DEFAULT 262144,  -- 256MB
    visibility          TEXT NOT NULL DEFAULT 'draft' CHECK (visibility IN ('draft', 'private', 'public')),
    author_id           UUID REFERENCES users (id) ON DELETE SET NULL,
    submission_count    INTEGER NOT NULL DEFAULT 0,
    accepted_count      INTEGER NOT NULL DEFAULT 0,
    solved_user_count   INTEGER NOT NULL DEFAULT 0,
    judge_type          TEXT NOT NULL DEFAULT 'normal' CHECK (judge_type IN ('normal', 'interactive')),  -- SPJ 预留
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_problems_visibility ON problems (visibility);
CREATE INDEX idx_problems_difficulty ON problems (difficulty);
CREATE INDEX idx_problems_author ON problems (author_id);

CREATE TABLE tags (
    id   BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE problem_tags (
    problem_id UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    tag_id     BIGINT NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (problem_id, tag_id)
);

-- 测试数据元信息;文件内容存于共享卷,DB 只存路径与哈希
CREATE TABLE problem_testdata (
    problem_id   UUID PRIMARY KEY REFERENCES problems (id) ON DELETE CASCADE,
    data_version INTEGER NOT NULL DEFAULT 1,
    storage_path TEXT NOT NULL DEFAULT '',
    sha256       TEXT NOT NULL DEFAULT '',
    case_count   INTEGER NOT NULL DEFAULT 0,
    checker      TEXT NOT NULL DEFAULT 'diff' CHECK (checker IN ('diff', 'spj', 'interactive')),
    spj_source   TEXT NOT NULL DEFAULT '',
    config_json  JSONB NOT NULL DEFAULT '{}'::jsonb  -- 每测试点限覆盖 / batched 依赖声明
);

-- 出题工作流:线上题与暂存草稿分离
CREATE TABLE problem_versions (
    id            BIGSERIAL PRIMARY KEY,
    problem_id    UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    version_no    INTEGER NOT NULL,
    statement_md  TEXT NOT NULL DEFAULT '',
    config_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
    testdata_path TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'pending_review', 'published')),
    created_by    UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (problem_id, version_no)
);

-- ---------- Contests ----------
CREATE TABLE contests (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title              TEXT NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    rule               TEXT NOT NULL DEFAULT 'acm' CHECK (rule IN ('acm', 'ioi')),
    begin_at           TIMESTAMPTZ NOT NULL,
    end_at             TIMESTAMPTZ NOT NULL,
    freeze_at          TIMESTAMPTZ,          -- 封榜时间,NULL=不封榜
    visibility         TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private', 'password')),
    password_hash      TEXT NOT NULL DEFAULT '',
    rankboard_visible  BOOLEAN NOT NULL DEFAULT TRUE,
    created_by         UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_contests_begin ON contests (begin_at);

CREATE TABLE contest_problems (
    contest_id UUID NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    problem_id UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (contest_id, problem_id)
);

CREATE TABLE contest_participants (
    contest_id UUID NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status     TEXT NOT NULL DEFAULT 'registered' CHECK (status IN ('registered', 'in_contest', 'finished')),
    PRIMARY KEY (contest_id, user_id)
);

-- DOMjudge scorecache 式增量积分格;rejudge 安全
CREATE TABLE contest_submission_cells (
    contest_id    UUID NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    problem_id    UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    attempts      INTEGER NOT NULL DEFAULT 0,
    penalty_sec   INTEGER NOT NULL DEFAULT 0,
    solved_at     TIMESTAMPTZ,
    score         INTEGER NOT NULL DEFAULT 0,  -- IOI: 该题最高分
    pending_count INTEGER NOT NULL DEFAULT 0,  -- 封榜期间的提交数
    PRIMARY KEY (contest_id, user_id, problem_id)
);

-- ---------- Submissions ----------
-- status 列兼作判题队列:worker 用 SKIP LOCKED 领取 Pending 行
CREATE TABLE submissions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    problem_id      UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    language        TEXT NOT NULL,
    source_code     TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'Pending' CHECK (status IN (
                        'Pending', 'Judging', 'Accepted', 'Wrong Answer',
                        'Time Limit Exceeded', 'Memory Limit Exceeded',
                        'Runtime Error', 'Compile Error', 'Output Limit Exceeded',
                        'System Error', 'Skipped')),
    score           INTEGER NOT NULL DEFAULT 0,
    total_time_ms   INTEGER NOT NULL DEFAULT 0,
    peak_memory_kb  INTEGER NOT NULL DEFAULT 0,
    compile_result  TEXT NOT NULL DEFAULT '',
    case_results    JSONB NOT NULL DEFAULT '[]'::jsonb,  -- 逐测试点快照
    contest_id      UUID REFERENCES contests (id) ON DELETE CASCADE,  -- NULL=练习
    submitted_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    judged_at       TIMESTAMPTZ
);

CREATE INDEX idx_submissions_user_time ON submissions (user_id, submitted_at DESC);
CREATE INDEX idx_submissions_problem_status ON submissions (problem_id, status);
CREATE INDEX idx_submissions_status ON submissions (status);
CREATE INDEX idx_submissions_contest ON submissions (contest_id);

CREATE TABLE submission_cases (
    id             BIGSERIAL PRIMARY KEY,
    submission_id  UUID NOT NULL REFERENCES submissions (id) ON DELETE CASCADE,
    case_index     INTEGER NOT NULL,
    verdict        TEXT NOT NULL,
    time_ms        INTEGER NOT NULL DEFAULT 0,
    memory_kb      INTEGER NOT NULL DEFAULT 0,
    exit_status    TEXT NOT NULL DEFAULT '',
    checker_output TEXT NOT NULL DEFAULT '',
    UNIQUE (submission_id, case_index)
);

CREATE INDEX idx_submission_cases_sub ON submission_cases (submission_id);

-- ---------- Problem sets (题单;表已建模,v1 启用) ----------
CREATE TABLE problem_sets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    author_id   UUID REFERENCES users (id) ON DELETE SET NULL,
    visibility  TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE problem_set_problems (
    set_id     UUID NOT NULL REFERENCES problem_sets (id) ON DELETE CASCADE,
    problem_id UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (set_id, problem_id)
);

-- ---------- Editorials (题解) / Discussions ----------
CREATE TABLE editorials (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    problem_id  UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    author_id   UUID REFERENCES users (id) ON DELETE SET NULL,
    title       TEXT NOT NULL,
    content_md  TEXT NOT NULL DEFAULT '',
    visibility  TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private')),
    status      TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('draft', 'published')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_editorials_problem ON editorials (problem_id);

CREATE TABLE discussion_posts (
    id           BIGSERIAL PRIMARY KEY,
    problem_id   UUID REFERENCES problems (id) ON DELETE CASCADE,
    editorial_id UUID REFERENCES editorials (id) ON DELETE CASCADE,
    contest_id   UUID REFERENCES contests (id) ON DELETE CASCADE,
    author_id    UUID REFERENCES users (id) ON DELETE SET NULL,
    content_md   TEXT NOT NULL,
    parent_id    BIGINT REFERENCES discussion_posts (id) ON DELETE CASCADE,  -- 线程评论
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(problem_id, editorial_id, contest_id) = 1)
);

CREATE INDEX idx_discussion_problem ON discussion_posts (problem_id);
CREATE INDEX idx_discussion_editorial ON discussion_posts (editorial_id);
