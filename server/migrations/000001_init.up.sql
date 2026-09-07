-- Vertex OJ — 初始 Schema
-- PostgreSQL 16,使用 pgcrypto 的 gen_random_uuid() 生成 UUID
--
-- 表创建顺序注意:FK 只能引用已存在的表,因此依赖顺序为:
--   users → domains → problems(+tags/testdata/versions) → contests → submissions → 社区表

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

-- ---------- Domains and membership ----------
CREATE TABLE domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z][a-z0-9-]{1,31}$'),
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    description TEXT NOT NULL DEFAULT '',
    owner_id UUID REFERENCES users(id),
    is_official BOOLEAN NOT NULL DEFAULT FALSE,
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('public', 'private')),
    join_policy TEXT NOT NULL DEFAULT 'invite' CHECK (join_policy IN ('open', 'approval', 'invite')),
    archived BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (is_official OR owner_id IS NOT NULL),
    CHECK (NOT is_official OR (slug = 'official' AND owner_id IS NULL AND visibility = 'public' AND join_policy = 'open' AND NOT archived))
);
CREATE UNIQUE INDEX domains_one_official ON domains(is_official) WHERE is_official;

CREATE TABLE domain_roles (
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    key TEXT NOT NULL CHECK (key ~ '^[a-z][a-z0-9_-]{0,31}$'),
    name TEXT NOT NULL,
    permissions JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(permissions) = 'array'),
    builtin BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (domain_id, key)
);

CREATE TABLE domain_members (
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_key TEXT NOT NULL DEFAULT 'member',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'pending', 'invited', 'suspended')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (domain_id, user_id),
    FOREIGN KEY (domain_id, role_key) REFERENCES domain_roles(domain_id, key)
);
ALTER TABLE domains ADD CONSTRAINT domains_owner_membership
    FOREIGN KEY (id, owner_id) REFERENCES domain_members(domain_id, user_id)
    DEFERRABLE INITIALLY DEFERRED;
CREATE INDEX domain_members_user ON domain_members(user_id, status, domain_id);

-- Public numbers are allocated in the inserting transaction and never reused
-- after deletion. The counter is local to a domain and resource kind.
CREATE TABLE domain_number_counters (
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    value BIGINT NOT NULL CHECK (value > 0),
    PRIMARY KEY(domain_id,kind)
);
CREATE FUNCTION allocate_domain_number() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.public_id IS NOT NULL THEN
        RAISE EXCEPTION 'public_id is assigned by the database' USING ERRCODE = '23514';
    END IF;
    INSERT INTO domain_number_counters(domain_id,kind,value)
    VALUES(NEW.domain_id,TG_ARGV[0],TG_ARGV[1]::bigint)
    ON CONFLICT(domain_id,kind) DO UPDATE
        SET value=domain_number_counters.value+1
    RETURNING value INTO NEW.public_id;
    RETURN NEW;
END;
$$;
CREATE FUNCTION protect_resource_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR NEW.domain_id IS DISTINCT FROM OLD.domain_id
       OR NEW.public_id IS DISTINCT FROM OLD.public_id THEN
        RAISE EXCEPTION 'resource identity and domain are immutable' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TABLE domain_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    public_id BIGINT NOT NULL,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    description TEXT NOT NULL DEFAULT '',
    owner_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(domain_id, id),
    UNIQUE(domain_id, name),
    FOREIGN KEY(domain_id, owner_id) REFERENCES domain_members(domain_id, user_id)
);

CREATE TABLE domain_group_members (
    domain_id UUID NOT NULL,
    group_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('member','manager')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(group_id, user_id),
    FOREIGN KEY(domain_id, group_id) REFERENCES domain_groups(domain_id, id) ON DELETE CASCADE,
    FOREIGN KEY(domain_id, user_id) REFERENCES domain_members(domain_id, user_id) ON DELETE CASCADE
);
CREATE INDEX domain_group_members_user ON domain_group_members(domain_id, user_id, group_id);

CREATE TABLE domain_audit_events (
    id BIGSERIAL PRIMARY KEY,
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX domain_audit_events_recent ON domain_audit_events(domain_id, created_at DESC, id DESC);

INSERT INTO domains(id, slug, name, is_official, visibility, join_policy)
VALUES ('00000000-0000-4000-8000-000000000001', 'official', '官方', TRUE, 'public', 'open');
INSERT INTO domain_roles(domain_id, key, name, permissions, builtin) VALUES
('00000000-0000-4000-8000-000000000001','admin','域管理员','["domain.settings.manage","domain.members.manage","domain.roles.manage","domain.groups.manage","domain.resources.manage","problem.create","contest.create","problem_set.create","submission.create","content.create"]',TRUE),
('00000000-0000-4000-8000-000000000001','author','出题人','["problem.create","contest.create","problem_set.create","submission.create","content.create"]',TRUE),
('00000000-0000-4000-8000-000000000001','member','成员','["problem_set.create","submission.create","content.create"]',TRUE),
('00000000-0000-4000-8000-000000000001','viewer','只读成员','[]',TRUE);

-- ---------- Problems ----------
CREATE TABLE problems (
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
    public_id BIGINT NOT NULL,
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title               TEXT NOT NULL,
    statement_md        TEXT NOT NULL DEFAULT '',
    difficulty          INTEGER NOT NULL DEFAULT 1,   -- 1-10
    source              TEXT NOT NULL DEFAULT '',
    time_limit_ms       INTEGER NOT NULL DEFAULT 1000,
    memory_limit_kb     INTEGER NOT NULL DEFAULT 262144,  -- 256MB
    visibility          TEXT NOT NULL DEFAULT 'draft' CHECK (visibility IN ('draft', 'private', 'public')),
    owner_id            UUID NOT NULL REFERENCES users(id),
    author_id           UUID REFERENCES users (id) ON DELETE SET NULL,
    submission_count    INTEGER NOT NULL DEFAULT 0,
    accepted_count      INTEGER NOT NULL DEFAULT 0,
    solved_user_count   INTEGER NOT NULL DEFAULT 0,
    judge_type          TEXT NOT NULL DEFAULT 'normal' CHECK (judge_type IN ('normal', 'interactive')),
    statement_language  TEXT NOT NULL DEFAULT 'zh',
    package_revision    INTEGER NOT NULL DEFAULT 0,
    data_revision       INTEGER NOT NULL DEFAULT 0,
    published_version   INTEGER,
    built_revision      INTEGER NOT NULL DEFAULT 0,
    last_built_at       TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT problems_revision_non_negative
        CHECK (package_revision >= 0 AND data_revision >= 0 AND built_revision >= 0)
);

CREATE INDEX idx_problems_visibility ON problems (domain_id, visibility, created_at DESC, id DESC);
CREATE INDEX idx_problems_difficulty ON problems (domain_id, difficulty);
CREATE INDEX idx_problems_domain ON problems (domain_id, created_at DESC, id DESC);
CREATE INDEX idx_problems_author ON problems (author_id);
CREATE INDEX idx_problems_owner ON problems (domain_id, owner_id);

-- Mutable authoring metadata. problems keeps the currently published projection.
CREATE TABLE problem_workspaces (
    problem_id UUID PRIMARY KEY REFERENCES problems(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    statement_md TEXT NOT NULL DEFAULT '',
    difficulty INTEGER NOT NULL DEFAULT 1,
    source TEXT NOT NULL DEFAULT '',
    time_limit_ms INTEGER NOT NULL DEFAULT 1000 CHECK(time_limit_ms>0),
    memory_limit_kb INTEGER NOT NULL DEFAULT 262144 CHECK(memory_limit_kb>0),
    judge_type TEXT NOT NULL DEFAULT 'normal' CHECK(judge_type IN ('normal','interactive')),
    statement_language TEXT NOT NULL DEFAULT 'zh',
    tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE FUNCTION initialize_problem_workspace() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO problem_workspaces(problem_id,title,statement_md,difficulty,source,time_limit_ms,memory_limit_kb,judge_type,statement_language)
    VALUES(NEW.id,NEW.title,NEW.statement_md,NEW.difficulty,NEW.source,NEW.time_limit_ms,NEW.memory_limit_kb,NEW.judge_type,NEW.statement_language);
    RETURN NEW;
END $$;
CREATE TRIGGER problems_workspace AFTER INSERT ON problems FOR EACH ROW EXECUTE FUNCTION initialize_problem_workspace();

CREATE TABLE problem_access (
    id BIGSERIAL PRIMARY KEY,
    domain_id UUID NOT NULL REFERENCES domains(id),
    problem_id UUID NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    user_id UUID,
    group_id UUID,
    role TEXT NOT NULL CHECK (role IN ('reader', 'editor')),
    granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(user_id, group_id) = 1),
    FOREIGN KEY (domain_id, user_id) REFERENCES domain_members(domain_id, user_id) ON DELETE CASCADE,
    FOREIGN KEY (domain_id, group_id) REFERENCES domain_groups(domain_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX problem_access_user ON problem_access(problem_id, user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX problem_access_group ON problem_access(problem_id, group_id) WHERE group_id IS NOT NULL;

CREATE TABLE tags (
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
    id   BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    UNIQUE (domain_id, name)
);

CREATE TABLE problem_tags (
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
    problem_id UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    tag_id     BIGINT NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (problem_id, tag_id)
);

CREATE INDEX idx_problem_tags_tag ON problem_tags (tag_id);

-- Latest candidate only. Judge jobs consume immutable problem_versions instead.
CREATE TABLE problem_testdata (
    problem_id   UUID PRIMARY KEY REFERENCES problems (id) ON DELETE CASCADE,
    data_version INTEGER NOT NULL DEFAULT 1,
    data_revision INTEGER NOT NULL DEFAULT 0,
    build_id UUID,
    samples_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    storage_path TEXT NOT NULL DEFAULT '',
    sha256       TEXT NOT NULL DEFAULT '',
    case_count   INTEGER NOT NULL DEFAULT 0,
    checker      TEXT NOT NULL DEFAULT 'diff' CHECK (checker IN ('diff', 'spj', 'interactive', 'testlib')),
    spj_source   TEXT NOT NULL DEFAULT '',
    config_json  JSONB NOT NULL DEFAULT '{}'::jsonb  -- 每测试点限覆盖 / batched 依赖声明
);

-- Immutable releases. The current pointer and public projection move together.
CREATE TABLE problem_versions (
    id            BIGSERIAL PRIMARY KEY,
    problem_id    UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    version_no    INTEGER NOT NULL CHECK(version_no>0),
    workspace_revision INTEGER NOT NULL,
    data_revision INTEGER NOT NULL,
    artifact_version INTEGER NOT NULL,
    title TEXT NOT NULL,
    statement_md  TEXT NOT NULL DEFAULT '',
    difficulty INTEGER NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    time_limit_ms INTEGER NOT NULL CHECK(time_limit_ms>0),
    memory_limit_kb INTEGER NOT NULL CHECK(memory_limit_kb>0),
    judge_type TEXT NOT NULL CHECK(judge_type IN ('normal','interactive')),
    statement_language TEXT NOT NULL,
    tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    statements_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    package_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    config_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
    testdata_path TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    case_count INTEGER NOT NULL CHECK(case_count>0),
    checker TEXT NOT NULL CHECK(checker IN ('diff','spj','interactive','testlib')),
    spj_source TEXT NOT NULL DEFAULT '',
    created_by    UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (problem_id, version_no)
);
ALTER TABLE problems ADD CONSTRAINT problems_published_version FOREIGN KEY(id,published_version) REFERENCES problem_versions(problem_id,version_no);
CREATE FUNCTION protect_problem_release() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (to_jsonb(NEW)-'created_by') IS DISTINCT FROM (to_jsonb(OLD)-'created_by') THEN
        RAISE EXCEPTION 'published problem versions are immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER problem_versions_immutable BEFORE UPDATE ON problem_versions FOR EACH ROW EXECUTE FUNCTION protect_problem_release();

-- ---------- Problem authoring ----------
-- Localized working statements; explicit publication writes the public Markdown.
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
    data_revision    INTEGER NOT NULL,
    input_json       JSONB NOT NULL,
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
ALTER TABLE problem_testdata ADD FOREIGN KEY(build_id) REFERENCES problem_build_jobs(id) ON DELETE SET NULL;

-- ---------- Contests ----------
CREATE TABLE contests (
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
    public_id BIGINT NOT NULL,
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
    owner_id           UUID NOT NULL REFERENCES users(id),
    admission          TEXT NOT NULL DEFAULT 'members' CHECK (admission IN ('members', 'restricted')),
    created_by         UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT contests_unfreeze_after_freeze
        CHECK (unfreeze_at IS NULL OR freeze_at IS NULL OR unfreeze_at >= freeze_at)
);

CREATE INDEX idx_contests_begin ON contests (domain_id, begin_at DESC);

CREATE TABLE contest_problems (
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
    contest_id UUID NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    problem_id UUID NOT NULL REFERENCES problems (id),
    problem_version INTEGER NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    label      TEXT NOT NULL DEFAULT '',
    color      TEXT NOT NULL DEFAULT '',
    points     INTEGER NOT NULL DEFAULT 100 CHECK (points >= 0),
    PRIMARY KEY (contest_id, problem_id)
);
ALTER TABLE contest_problems ADD FOREIGN KEY(problem_id,problem_version) REFERENCES problem_versions(problem_id,version_no);
CREATE FUNCTION pin_contest_problem_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.problem_version IS NULL THEN
        SELECT published_version INTO NEW.problem_version FROM problems WHERE id=NEW.problem_id;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER contest_problems_version BEFORE INSERT ON contest_problems FOR EACH ROW EXECUTE FUNCTION pin_contest_problem_version();

CREATE TABLE contest_participants (
    contest_id UUID NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status     TEXT NOT NULL DEFAULT 'registered' CHECK (status IN ('registered', 'in_contest', 'finished')),
    PRIMARY KEY (contest_id, user_id)
);

CREATE TABLE contest_access (
    id BIGSERIAL PRIMARY KEY,
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
    contest_id UUID NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    group_id UUID,
    role TEXT NOT NULL CHECK (role IN ('editor','jury','observer','participant')),
    granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(user_id,group_id) = 1),
    FOREIGN KEY (domain_id,user_id) REFERENCES domain_members(domain_id,user_id) ON DELETE CASCADE,
    FOREIGN KEY (domain_id,group_id) REFERENCES domain_groups(domain_id,id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX contest_access_user ON contest_access(contest_id,user_id,role) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX contest_access_group ON contest_access(contest_id,group_id,role) WHERE group_id IS NOT NULL;

-- Read-only compatibility projection for submission/community read models.
-- It includes inherited grants only while the account and membership are active.
CREATE VIEW contest_staff AS
    SELECT a.contest_id,m.user_id,
           CASE WHEN bool_or(a.role='jury') THEN 'jury' ELSE 'observer' END AS role,
           min(a.created_at) AS created_at
    FROM contest_access a
    JOIN domain_members m ON m.domain_id=a.domain_id AND m.status='active'
    JOIN users u ON u.id=m.user_id AND u.disabled_at IS NULL
    WHERE a.role IN ('jury','observer') AND (a.user_id=m.user_id OR EXISTS (
        SELECT 1 FROM domain_group_members gm
        WHERE gm.domain_id=a.domain_id AND gm.group_id=a.group_id AND gm.user_id=m.user_id))
    GROUP BY a.contest_id,m.user_id;

-- DOMjudge scorecache 式增量积分格;rejudge 安全
CREATE TABLE contest_submission_cells (
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
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
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
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
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
    public_id BIGINT NOT NULL,
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    problem_id      UUID NOT NULL REFERENCES problems (id),
    problem_version INTEGER NOT NULL,
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

ALTER TABLE submissions ADD FOREIGN KEY(problem_id,problem_version) REFERENCES problem_versions(problem_id,version_no);
CREATE FUNCTION pin_submission_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.problem_version IS NULL THEN
        IF NEW.contest_id IS NULL THEN
            SELECT published_version INTO NEW.problem_version FROM problems WHERE id=NEW.problem_id;
        ELSE
            SELECT problem_version INTO NEW.problem_version FROM contest_problems WHERE contest_id=NEW.contest_id AND problem_id=NEW.problem_id;
        END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER submissions_version BEFORE INSERT ON submissions FOR EACH ROW EXECUTE FUNCTION pin_submission_version();

CREATE INDEX idx_submissions_user_time ON submissions (user_id, submitted_at DESC);
CREATE INDEX idx_submissions_domain_time ON submissions (domain_id, submitted_at DESC, id DESC);
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
    domain_id UUID NOT NULL REFERENCES domains(id),
    problem_id UUID NOT NULL REFERENCES problems(id),
    problem_version INTEGER NOT NULL,
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
ALTER TABLE judge_jobs ADD FOREIGN KEY(problem_id,problem_version) REFERENCES problem_versions(problem_id,version_no);
CREATE FUNCTION pin_judge_job_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    SELECT domain_id,problem_id,problem_version INTO NEW.domain_id,NEW.problem_id,NEW.problem_version FROM submissions WHERE id=NEW.submission_id;
    RETURN NEW;
END $$;
CREATE TRIGGER judge_jobs_version BEFORE INSERT ON judge_jobs FOR EACH ROW EXECUTE FUNCTION pin_judge_job_version();
CREATE FUNCTION protect_judge_job_input() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.submission_id,NEW.generation,NEW.domain_id,NEW.problem_id,NEW.problem_version) IS DISTINCT FROM (OLD.submission_id,OLD.generation,OLD.domain_id,OLD.problem_id,OLD.problem_version) THEN
        RAISE EXCEPTION 'judge generation inputs are immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER judge_jobs_input BEFORE UPDATE ON judge_jobs FOR EACH ROW EXECUTE FUNCTION protect_judge_job_input();

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
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
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
CREATE INDEX idx_rejudgings_created ON rejudgings (domain_id, created_at DESC);

CREATE TABLE rejudging_submissions (
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
    rejudging_id             UUID NOT NULL REFERENCES rejudgings (id) ON DELETE CASCADE,
    submission_id            UUID NOT NULL REFERENCES submissions (id) ON DELETE CASCADE,
    generation               INTEGER NOT NULL CHECK (generation > 0),
    prior_status             TEXT NOT NULL DEFAULT '',
    prior_problem_version    INTEGER NOT NULL,
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
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
    public_id BIGINT NOT NULL,
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    author_id   UUID REFERENCES users (id) ON DELETE SET NULL,
    owner_id    UUID NOT NULL REFERENCES users (id),
    visibility  TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_problem_sets_author ON problem_sets (author_id);
CREATE INDEX idx_problem_sets_owner ON problem_sets (domain_id, owner_id);
CREATE INDEX idx_problem_sets_visibility ON problem_sets (domain_id, visibility, created_at DESC);
CREATE INDEX idx_problem_sets_domain ON problem_sets (domain_id, created_at DESC);

CREATE TABLE problem_set_access (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    domain_id UUID NOT NULL REFERENCES domains(id),
    set_id UUID NOT NULL REFERENCES problem_sets(id) ON DELETE CASCADE,
    user_id UUID,
    group_id UUID,
    role TEXT NOT NULL CHECK(role IN ('reader','editor')),
    granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    CHECK(num_nonnulls(user_id,group_id)=1),
    FOREIGN KEY(domain_id,user_id) REFERENCES domain_members(domain_id,user_id) ON DELETE CASCADE,
    FOREIGN KEY(domain_id,group_id) REFERENCES domain_groups(domain_id,id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX problem_set_access_user ON problem_set_access(set_id,user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX problem_set_access_group ON problem_set_access(set_id,group_id) WHERE group_id IS NOT NULL;

CREATE TABLE problem_set_problems (
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
    set_id     UUID NOT NULL REFERENCES problem_sets (id) ON DELETE CASCADE,
    problem_id UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    note       TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (set_id, problem_id)
);

-- ---------- Editorials (题解) / Discussions ----------
CREATE TABLE editorials (
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
    public_id BIGINT NOT NULL,
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
CREATE INDEX idx_editorials_domain ON editorials (domain_id, created_at DESC, id DESC);
CREATE INDEX idx_editorials_popular ON editorials (problem_id, vote_count DESC, created_at DESC);

CREATE TABLE editorial_votes (
    editorial_id UUID NOT NULL REFERENCES editorials (id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (editorial_id, user_id)
);

CREATE INDEX idx_editorial_votes_user ON editorial_votes (user_id);

CREATE TABLE discussion_posts (
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
    id           BIGSERIAL PRIMARY KEY,
    problem_id   UUID REFERENCES problems (id) ON DELETE CASCADE,
    editorial_id UUID REFERENCES editorials (id) ON DELETE CASCADE,
    author_id    UUID REFERENCES users (id) ON DELETE SET NULL,
    content_md   TEXT NOT NULL,
    parent_id    BIGINT REFERENCES discussion_posts (id) ON DELETE CASCADE,  -- 线程评论
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(problem_id, editorial_id) = 1)
);

CREATE INDEX idx_discussion_problem ON discussion_posts (problem_id);
CREATE INDEX idx_discussion_editorial ON discussion_posts (editorial_id);

-- ---------- Announcements ----------
CREATE TABLE announcements (
    domain_id UUID NOT NULL DEFAULT '00000000-0000-4000-8000-000000000001' REFERENCES domains(id),
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
    ON announcements (domain_id, pinned DESC, created_at DESC)
    WHERE published;

-- Domain-local resource identities and cross-resource boundaries.
ALTER TABLE domain_groups ADD UNIQUE(domain_id,public_id);
CREATE TRIGGER domain_groups_number BEFORE INSERT ON domain_groups FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('groups','1');
CREATE TRIGGER domain_groups_identity BEFORE UPDATE OF id,domain_id,public_id ON domain_groups FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();
ALTER TABLE problems ADD UNIQUE(domain_id,public_id);
ALTER TABLE problems ADD UNIQUE(domain_id,id);
ALTER TABLE problem_access ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);
CREATE TRIGGER problems_number BEFORE INSERT ON problems FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('problems','1000');
CREATE TRIGGER problems_identity BEFORE UPDATE OF id,domain_id,public_id ON problems FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();
ALTER TABLE contests ADD UNIQUE(domain_id,public_id);
ALTER TABLE contests ADD UNIQUE(domain_id,id);
ALTER TABLE contest_access ADD FOREIGN KEY(domain_id,contest_id) REFERENCES contests(domain_id,id);
CREATE TRIGGER contests_number BEFORE INSERT ON contests FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('contests','1');
CREATE TRIGGER contests_identity BEFORE UPDATE OF id,domain_id,public_id ON contests FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();
ALTER TABLE submissions ADD UNIQUE(domain_id,public_id);
ALTER TABLE submissions ADD UNIQUE(domain_id,id);
CREATE TRIGGER submissions_number BEFORE INSERT ON submissions FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('submissions','1');
CREATE TRIGGER submissions_identity BEFORE UPDATE OF id,domain_id,public_id ON submissions FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();
ALTER TABLE problem_sets ADD UNIQUE(domain_id,public_id);
ALTER TABLE problem_sets ADD UNIQUE(domain_id,id);
ALTER TABLE problem_set_access ADD FOREIGN KEY(domain_id,set_id) REFERENCES problem_sets(domain_id,id);
CREATE TRIGGER problem_sets_number BEFORE INSERT ON problem_sets FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('problem_sets','1');
CREATE TRIGGER problem_sets_identity BEFORE UPDATE OF id,domain_id,public_id ON problem_sets FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();
ALTER TABLE editorials ADD UNIQUE(domain_id,public_id);
ALTER TABLE editorials ADD UNIQUE(domain_id,id);
CREATE TRIGGER editorials_number BEFORE INSERT ON editorials FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('editorials','1');
CREATE TRIGGER editorials_identity BEFORE UPDATE OF id,domain_id,public_id ON editorials FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();
ALTER TABLE discussion_posts ADD UNIQUE(domain_id,id);
ALTER TABLE tags ADD UNIQUE(domain_id,id);
ALTER TABLE rejudgings ADD UNIQUE(domain_id,id);
ALTER TABLE submissions ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);
ALTER TABLE submissions ADD FOREIGN KEY(domain_id,contest_id) REFERENCES contests(domain_id,id);
ALTER TABLE editorials ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);
ALTER TABLE discussion_posts ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);
ALTER TABLE discussion_posts ADD FOREIGN KEY(domain_id,editorial_id) REFERENCES editorials(domain_id,id);
ALTER TABLE discussion_posts ADD FOREIGN KEY(domain_id,parent_id) REFERENCES discussion_posts(domain_id,id);
ALTER TABLE discussion_posts ADD UNIQUE(problem_id,id);
ALTER TABLE discussion_posts ADD UNIQUE(editorial_id,id);
ALTER TABLE discussion_posts ADD FOREIGN KEY(problem_id,parent_id) REFERENCES discussion_posts(problem_id,id);
ALTER TABLE discussion_posts ADD FOREIGN KEY(editorial_id,parent_id) REFERENCES discussion_posts(editorial_id,id);
ALTER TABLE rejudgings ADD FOREIGN KEY(domain_id,contest_id) REFERENCES contests(domain_id,id);
ALTER TABLE rejudgings ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);
ALTER TABLE problem_tags ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);
ALTER TABLE problem_tags ADD FOREIGN KEY(domain_id,tag_id) REFERENCES tags(domain_id,id);
ALTER TABLE contest_problems ADD FOREIGN KEY(domain_id,contest_id) REFERENCES contests(domain_id,id);
ALTER TABLE contest_problems ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);
ALTER TABLE problem_set_problems ADD FOREIGN KEY(domain_id,set_id) REFERENCES problem_sets(domain_id,id);
ALTER TABLE problem_set_problems ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);
ALTER TABLE rejudging_submissions ADD FOREIGN KEY(domain_id,rejudging_id) REFERENCES rejudgings(domain_id,id);
ALTER TABLE rejudging_submissions ADD FOREIGN KEY(domain_id,submission_id) REFERENCES submissions(domain_id,id);
ALTER TABLE contest_submission_cells ADD FOREIGN KEY(domain_id,contest_id) REFERENCES contests(domain_id,id);
ALTER TABLE contest_submission_cells ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);
ALTER TABLE clarifications ADD UNIQUE(contest_id,id);
ALTER TABLE clarifications ADD FOREIGN KEY(domain_id,contest_id) REFERENCES contests(domain_id,id);
ALTER TABLE clarifications ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);
ALTER TABLE clarifications ADD FOREIGN KEY(contest_id,parent_id) REFERENCES clarifications(contest_id,id);
