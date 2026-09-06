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
    disabled_at   TIMESTAMPTZ,
    disabled_reason TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_username ON users (username);
CREATE INDEX idx_users_disabled ON users (disabled_at) WHERE disabled_at IS NOT NULL;

CREATE TABLE auth_sessions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    refresh_token_hash BYTEA NOT NULL UNIQUE,
    expires_at         TIMESTAMPTZ NOT NULL,
    revoked_at         TIMESTAMPTZ,
    last_used_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_auth_sessions_active_user
    ON auth_sessions (user_id)
    WHERE revoked_at IS NULL;

CREATE INDEX idx_auth_sessions_expires_at ON auth_sessions (expires_at);

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
    judge_type          TEXT NOT NULL DEFAULT 'normal' CHECK (judge_type IN ('normal', 'interactive')),
    statement_language  TEXT NOT NULL DEFAULT 'zh',
    package_revision    INTEGER NOT NULL DEFAULT 0,
    built_revision      INTEGER NOT NULL DEFAULT 0,
    last_built_at       TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT problems_revision_non_negative
        CHECK (package_revision >= 0 AND built_revision >= 0)
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

CREATE INDEX idx_problem_tags_tag ON problem_tags (tag_id);

-- 测试数据元信息;文件内容存于共享卷,DB 只存路径与哈希
CREATE TABLE problem_testdata (
    problem_id   UUID PRIMARY KEY REFERENCES problems (id) ON DELETE CASCADE,
    data_version INTEGER NOT NULL DEFAULT 1,
    storage_path TEXT NOT NULL DEFAULT '',
    sha256       TEXT NOT NULL DEFAULT '',
    case_count   INTEGER NOT NULL DEFAULT 0,
    checker      TEXT NOT NULL DEFAULT 'diff' CHECK (checker IN ('diff', 'spj', 'interactive', 'testlib')),
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

-- ---------- Problem authoring ----------
-- 题面按语言存储并渲染到 problems.statement_md，公开读路径无需读取工作区。
CREATE TABLE problem_statements (
    problem_id    UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    language      TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    legend        TEXT NOT NULL DEFAULT '',
    input_format  TEXT NOT NULL DEFAULT '',
    output_format TEXT NOT NULL DEFAULT '',
    notes         TEXT NOT NULL DEFAULT '',
    tutorial      TEXT NOT NULL DEFAULT '',
    scoring       TEXT NOT NULL DEFAULT '',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (problem_id, language)
);

CREATE TABLE problem_files (
    id               BIGSERIAL PRIMARY KEY,
    problem_id       UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    kind             TEXT NOT NULL CHECK (kind IN ('checker', 'validator', 'generator', 'solution', 'interactor')),
    name             TEXT NOT NULL,
    language         TEXT NOT NULL DEFAULT 'cpp',
    source_code      TEXT NOT NULL DEFAULT '',
    expected_verdict TEXT NOT NULL DEFAULT '',
    is_active        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (problem_id, kind, name)
);

CREATE UNIQUE INDEX ux_problem_files_active
    ON problem_files (problem_id, kind)
    WHERE is_active;

CREATE INDEX idx_problem_files_problem ON problem_files (problem_id, kind, name);

CREATE TABLE problem_tests (
    id           BIGSERIAL PRIMARY KEY,
    problem_id   UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    test_index   INTEGER NOT NULL CHECK (test_index > 0),
    group_name   TEXT NOT NULL DEFAULT '',
    source       TEXT NOT NULL CHECK (source IN ('manual', 'generator')),
    input_data   TEXT NOT NULL DEFAULT '',
    generate_cmd TEXT NOT NULL DEFAULT '',
    is_sample    BOOLEAN NOT NULL DEFAULT FALSE,
    points       INTEGER NOT NULL DEFAULT 0 CHECK (points >= 0),
    description  TEXT NOT NULL DEFAULT '',
    UNIQUE (problem_id, test_index),
    CHECK (source <> 'generator' OR generate_cmd <> '')
);

-- 构建任务复用判题任务的租约和 generation 围栏语义。
CREATE TABLE problem_build_jobs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    problem_id       UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    revision         INTEGER NOT NULL,
    state            TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'succeeded', 'failed', 'cancelled', 'dead')),
    stage            TEXT NOT NULL DEFAULT 'queued',
    priority         INTEGER NOT NULL DEFAULT 0,
    attempt          INTEGER NOT NULL DEFAULT 0,
    available_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    worker_id        TEXT,
    lease_token      UUID,
    lease_expires_at TIMESTAMPTZ,
    progress_done    INTEGER NOT NULL DEFAULT 0,
    progress_total   INTEGER NOT NULL DEFAULT 0,
    log              TEXT NOT NULL DEFAULT '',
    error_message    TEXT NOT NULL DEFAULT '',
    tests_json       JSONB NOT NULL DEFAULT '[]'::jsonb,
    solutions_json   JSONB NOT NULL DEFAULT '[]'::jsonb,
    package_path     TEXT NOT NULL DEFAULT '',
    package_sha256   TEXT NOT NULL DEFAULT '',
    package_cases    INTEGER NOT NULL DEFAULT 0,
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX ux_problem_build_jobs_active
    ON problem_build_jobs (problem_id)
    WHERE state IN ('queued', 'running');

CREATE INDEX idx_problem_build_jobs_claim
    ON problem_build_jobs (state, priority DESC, available_at, created_at);

CREATE INDEX idx_problem_build_jobs_expired_lease
    ON problem_build_jobs (lease_expires_at)
    WHERE state = 'running';

CREATE INDEX idx_problem_build_jobs_problem
    ON problem_build_jobs (problem_id, created_at DESC);

-- ---------- Contests ----------
CREATE TABLE contests (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title              TEXT NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    rule               TEXT NOT NULL DEFAULT 'acm' CHECK (rule IN ('acm', 'icpc', 'ioi', 'oi')),
    begin_at           TIMESTAMPTZ NOT NULL,
    end_at             TIMESTAMPTZ NOT NULL,
    freeze_at          TIMESTAMPTZ,          -- 封榜时间,NULL=不封榜
    unfreeze_at        TIMESTAMPTZ,
    visibility         TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private', 'password')),
    password_hash      TEXT NOT NULL DEFAULT '',
    rankboard_visible  BOOLEAN NOT NULL DEFAULT TRUE,
    penalty_minutes    INTEGER NOT NULL DEFAULT 20 CHECK (penalty_minutes >= 0 AND penalty_minutes <= 1440),
    penalize_compile_error BOOLEAN NOT NULL DEFAULT TRUE,
    feedback           TEXT NOT NULL DEFAULT 'full' CHECK (feedback IN ('full', 'summary', 'none')),
    created_by         UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT contests_unfreeze_after_freeze
        CHECK (unfreeze_at IS NULL OR freeze_at IS NULL OR unfreeze_at >= freeze_at)
);

CREATE INDEX idx_contests_begin ON contests (begin_at);

CREATE TABLE contest_problems (
    contest_id UUID NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    problem_id UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    label      TEXT NOT NULL DEFAULT '',
    color      TEXT NOT NULL DEFAULT '',
    points     INTEGER NOT NULL DEFAULT 100 CHECK (points >= 0),
    PRIMARY KEY (contest_id, problem_id)
);

CREATE TABLE contest_participants (
    contest_id UUID NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status     TEXT NOT NULL DEFAULT 'registered' CHECK (status IN ('registered', 'in_contest', 'finished')),
    PRIMARY KEY (contest_id, user_id)
);

CREATE TABLE contest_staff (
    contest_id UUID NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('jury', 'observer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (contest_id, user_id)
);

CREATE INDEX idx_contest_staff_user ON contest_staff (user_id);

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
    public_attempts    INTEGER NOT NULL DEFAULT 0 CHECK (public_attempts >= 0),
    public_penalty_sec INTEGER NOT NULL DEFAULT 0 CHECK (public_penalty_sec >= 0),
    public_score       INTEGER NOT NULL DEFAULT 0 CHECK (public_score >= 0),
    public_solved_at   TIMESTAMPTZ,
    last_submit_at     TIMESTAMPTZ,
    PRIMARY KEY (contest_id, user_id, problem_id)
);

-- ---------- Contest clarifications ----------
CREATE TABLE clarifications (
    id           BIGSERIAL PRIMARY KEY,
    contest_id   UUID NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    problem_id   UUID REFERENCES problems (id) ON DELETE SET NULL,
    parent_id    BIGINT REFERENCES clarifications (id) ON DELETE CASCADE,
    author_id    UUID REFERENCES users (id) ON DELETE SET NULL,
    recipient_id UUID REFERENCES users (id) ON DELETE CASCADE,
    from_jury    BOOLEAN NOT NULL DEFAULT FALSE,
    subject      TEXT NOT NULL DEFAULT '',
    body         TEXT NOT NULL,
    answered     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (parent_id IS NULL OR recipient_id IS NOT NULL OR from_jury)
);

CREATE INDEX idx_clarifications_contest ON clarifications (contest_id, created_at DESC);
CREATE INDEX idx_clarifications_thread ON clarifications (parent_id);
CREATE INDEX idx_clarifications_recipient ON clarifications (contest_id, recipient_id);
CREATE INDEX idx_clarifications_open
    ON clarifications (contest_id, answered)
    WHERE parent_id IS NULL AND NOT from_jury;

-- ---------- Submissions ----------
-- submissions 保存用户可见的判题状态；调度状态由 judge_jobs 独立维护。
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
    judged_cases    INTEGER NOT NULL DEFAULT 0,
    total_cases     INTEGER NOT NULL DEFAULT 0,
    contest_id      UUID REFERENCES contests (id) ON DELETE CASCADE,  -- NULL=练习
    submitted_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    judged_at       TIMESTAMPTZ,
    judge_generation INTEGER NOT NULL DEFAULT 1,
    CONSTRAINT submissions_progress_non_negative
        CHECK (judged_cases >= 0 AND total_cases >= 0)
);

CREATE INDEX idx_submissions_user_time ON submissions (user_id, submitted_at DESC);
CREATE INDEX idx_submissions_problem_status ON submissions (problem_id, status);
CREATE INDEX idx_submissions_status ON submissions (status);
CREATE INDEX idx_submissions_contest ON submissions (contest_id);
CREATE INDEX idx_submissions_user_problem_status
    ON submissions (user_id, problem_id, status);
CREATE INDEX idx_submissions_user_accepted
    ON submissions (user_id, problem_id)
    WHERE status = 'Accepted';

CREATE TABLE judge_jobs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id    UUID NOT NULL REFERENCES submissions (id) ON DELETE CASCADE,
    generation       INTEGER NOT NULL CHECK (generation > 0),
    state            TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'completed', 'cancelled', 'dead')),
    priority         INTEGER NOT NULL DEFAULT 0,
    attempt          INTEGER NOT NULL DEFAULT 0,
    available_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    worker_id        TEXT,
    lease_token      UUID,
    lease_expires_at TIMESTAMPTZ,
    last_error       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ,
    UNIQUE (submission_id, generation)
);

CREATE INDEX idx_judge_jobs_claim
    ON judge_jobs (state, priority DESC, available_at, created_at);

CREATE INDEX idx_judge_jobs_expired_lease
    ON judge_jobs (lease_expires_at)
    WHERE state = 'running';

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

-- ---------- Rejudging ----------
CREATE TABLE rejudgings (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contest_id    UUID REFERENCES contests (id) ON DELETE CASCADE,
    problem_id    UUID REFERENCES problems (id) ON DELETE CASCADE,
    reason        TEXT NOT NULL DEFAULT '',
    state         TEXT NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'finished', 'cancelled')),
    total_count   INTEGER NOT NULL DEFAULT 0 CHECK (total_count >= 0),
    created_by    UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ
);

CREATE INDEX idx_rejudgings_contest ON rejudgings (contest_id, created_at DESC);
CREATE INDEX idx_rejudgings_created ON rejudgings (created_at DESC);

CREATE TABLE rejudging_submissions (
    rejudging_id             UUID NOT NULL REFERENCES rejudgings (id) ON DELETE CASCADE,
    submission_id            UUID NOT NULL REFERENCES submissions (id) ON DELETE CASCADE,
    generation               INTEGER NOT NULL CHECK (generation > 0),
    prior_status             TEXT NOT NULL DEFAULT '',
    prior_score              INTEGER NOT NULL DEFAULT 0,
    prior_total_time_ms      INTEGER NOT NULL DEFAULT 0,
    prior_peak_memory_kb     INTEGER NOT NULL DEFAULT 0,
    prior_compile_result     TEXT NOT NULL DEFAULT '',
    prior_case_results       JSONB NOT NULL DEFAULT '[]'::jsonb,
    prior_judged_cases       INTEGER NOT NULL DEFAULT 0,
    prior_total_cases        INTEGER NOT NULL DEFAULT 0,
    prior_judged_at          TIMESTAMPTZ,
    PRIMARY KEY (rejudging_id, submission_id)
);

CREATE INDEX idx_rejudging_submissions_submission
    ON rejudging_submissions (submission_id);

-- ---------- Problem sets ----------
CREATE TABLE problem_sets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    author_id   UUID REFERENCES users (id) ON DELETE SET NULL,
    visibility  TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_problem_sets_author ON problem_sets (author_id);
CREATE INDEX idx_problem_sets_visibility ON problem_sets (visibility, created_at DESC);

CREATE TABLE problem_set_problems (
    set_id     UUID NOT NULL REFERENCES problem_sets (id) ON DELETE CASCADE,
    problem_id UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    note       TEXT NOT NULL DEFAULT '',
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
    solved_only BOOLEAN NOT NULL DEFAULT FALSE,
    vote_count  INTEGER NOT NULL DEFAULT 0 CHECK (vote_count >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_editorials_problem ON editorials (problem_id);
CREATE INDEX idx_editorials_popular ON editorials (problem_id, vote_count DESC, created_at DESC);

CREATE TABLE editorial_votes (
    editorial_id UUID NOT NULL REFERENCES editorials (id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (editorial_id, user_id)
);

CREATE INDEX idx_editorial_votes_user ON editorial_votes (user_id);

CREATE TABLE discussion_posts (
    id           BIGSERIAL PRIMARY KEY,
    problem_id   UUID REFERENCES problems (id) ON DELETE CASCADE,
    editorial_id UUID REFERENCES editorials (id) ON DELETE CASCADE,
    contest_id   UUID REFERENCES contests (id) ON DELETE CASCADE,
    author_id    UUID REFERENCES users (id) ON DELETE SET NULL,
    content_md   TEXT NOT NULL,
    parent_id    BIGINT REFERENCES discussion_posts (id) ON DELETE CASCADE,  -- 线程评论
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(problem_id, editorial_id, contest_id) = 1)
);

CREATE INDEX idx_discussion_problem ON discussion_posts (problem_id);
CREATE INDEX idx_discussion_editorial ON discussion_posts (editorial_id);
CREATE INDEX idx_discussion_contest ON discussion_posts (contest_id);

-- ---------- Announcements ----------
CREATE TABLE announcements (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    content_md  TEXT NOT NULL DEFAULT '',
    pinned      BOOLEAN NOT NULL DEFAULT FALSE,
    published   BOOLEAN NOT NULL DEFAULT TRUE,
    created_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_announcements_feed
    ON announcements (pinned DESC, created_at DESC)
    WHERE published;
