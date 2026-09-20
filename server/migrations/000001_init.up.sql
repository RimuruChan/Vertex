-- Vertex OJ — 初始 Schema
-- PostgreSQL 16,使用 pgcrypto 的 gen_random_uuid() 生成 UUID
--
-- Fresh-install baseline. Tables, indexes and triggers are grouped by module.
-- Constraints belong to their tables; numbering triggers follow the table.
-- Cyclic and forward references remain explicit near the referenced table.
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

ALTER TABLE domain_groups ADD UNIQUE(domain_id,public_id);
CREATE TRIGGER domain_groups_number BEFORE INSERT ON domain_groups FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('groups','1');
CREATE TRIGGER domain_groups_identity BEFORE UPDATE OF id,domain_id,public_id ON domain_groups FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();

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

-- ---------- Problems ----------
CREATE TABLE problems (
    domain_id UUID NOT NULL REFERENCES domains(id),
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
    published_version   INTEGER,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE problems ADD UNIQUE(domain_id,public_id);
ALTER TABLE problems ADD UNIQUE(domain_id,id);
CREATE TRIGGER problems_number BEFORE INSERT ON problems FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('problems','1000');
CREATE TRIGGER problems_identity BEFORE UPDATE OF id,domain_id,public_id ON problems FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();

CREATE INDEX idx_problems_visibility ON problems (domain_id, visibility, created_at DESC, id DESC);
CREATE INDEX idx_problems_difficulty ON problems (domain_id, difficulty);
CREATE INDEX idx_problems_domain ON problems (domain_id, created_at DESC, id DESC);
CREATE INDEX idx_problems_author ON problems (author_id);
CREATE INDEX idx_problems_owner ON problems (domain_id, owner_id);

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

ALTER TABLE problem_access ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);
CREATE UNIQUE INDEX problem_access_user ON problem_access(problem_id, user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX problem_access_group ON problem_access(problem_id, group_id) WHERE group_id IS NOT NULL;

CREATE TABLE tags (
    domain_id UUID NOT NULL REFERENCES domains(id),
    id   BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    UNIQUE (domain_id, name),
    UNIQUE(domain_id,id)
);
CREATE TABLE problem_tags (
    domain_id UUID NOT NULL REFERENCES domains(id),
    problem_id UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    tag_id     BIGINT NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (problem_id, tag_id),
    FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id),
    FOREIGN KEY(domain_id,tag_id) REFERENCES tags(domain_id,id)
);
CREATE INDEX idx_problem_tags_tag ON problem_tags (tag_id);

-- Latest candidate only. Judge jobs consume immutable problem_versions instead.
CREATE TABLE problem_versions (
    id            BIGSERIAL PRIMARY KEY,
    problem_id    UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    version_no    INTEGER NOT NULL CHECK(version_no>0),
    source_revision BIGINT,
    source_tree_hash TEXT,
    check_id UUID,
    toolchain_key TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    statement_md  TEXT NOT NULL DEFAULT '',
    difficulty INTEGER NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    time_limit_ms INTEGER NOT NULL CHECK(time_limit_ms>0),
    memory_limit_kb INTEGER NOT NULL CHECK(memory_limit_kb>0),
    judge_type TEXT NOT NULL CHECK(judge_type IN ('normal','interactive')),
    statement_language TEXT NOT NULL,
    tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    config_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
    testdata_path TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    case_count INTEGER NOT NULL CHECK(case_count>0),
    checker TEXT NOT NULL CHECK(checker IN ('diff','spj','interactive','testlib','artifact')),
    spj_source TEXT NOT NULL DEFAULT '',
    created_by    UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (problem_id, version_no),
    CHECK ((source_revision IS NULL AND source_tree_hash IS NULL AND check_id IS NULL) OR
           (source_revision IS NOT NULL AND source_tree_hash IS NOT NULL AND check_id IS NOT NULL AND checker='artifact' AND toolchain_key ~ '^[a-f0-9]{64}$'))
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

-- Provenance is historical evidence, not a live cross-domain resource link.
-- Removing the source must not destroy or prevent use of an independent copy.
CREATE TABLE problem_origins (
    problem_id UUID PRIMARY KEY REFERENCES problems(id) ON DELETE CASCADE,
    source_domain_id UUID NOT NULL,
    source_domain_slug TEXT NOT NULL,
    source_problem_id UUID NOT NULL,
    source_problem_number BIGINT NOT NULL CHECK(source_problem_number>0),
    source_version INTEGER NOT NULL CHECK(source_version>0),
    source_title TEXT NOT NULL,
    source_sha256 TEXT NOT NULL,
    attribution TEXT NOT NULL CHECK(length(attribution)>0 AND octet_length(attribution)<=8192),
    copied_by UUID REFERENCES users(id) ON DELETE SET NULL,
    copied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE FUNCTION protect_problem_origin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (to_jsonb(NEW)-'copied_by') IS DISTINCT FROM (to_jsonb(OLD)-'copied_by') THEN
        RAISE EXCEPTION 'problem copy provenance is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER problem_origins_immutable BEFORE UPDATE ON problem_origins FOR EACH ROW EXECUTE FUNCTION protect_problem_origin();

-- ---------- Problem authoring ----------
-- Private working copies and explicit shared history. Content is immutable;
-- etags are concurrency tokens, never user-visible revision numbers.
CREATE TABLE problem_blobs (
    problem_id UUID NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    sha256 TEXT NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
    byte_size BIGINT NOT NULL CHECK(byte_size>=0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(problem_id,sha256)
);
-- Only explicitly approved files may be exposed through a published statement.
CREATE TABLE problem_version_files (
    problem_id UUID NOT NULL,
    version_no INTEGER NOT NULL,
    file_id TEXT NOT NULL CHECK(length(file_id) BETWEEN 1 AND 160),
    path TEXT NOT NULL,
    filename TEXT NOT NULL,
    media_type TEXT NOT NULL,
    purpose TEXT NOT NULL CHECK(purpose IN ('statement','asset','sample-input','sample-answer')),
    blob_sha256 TEXT NOT NULL,
    byte_size BIGINT NOT NULL CHECK(byte_size>=0),
    sample_index INTEGER NOT NULL DEFAULT 0,
    preview TEXT NOT NULL DEFAULT '' CHECK(octet_length(preview)<=8192),
    truncated BOOLEAN NOT NULL DEFAULT FALSE,
    is_binary BOOLEAN NOT NULL DEFAULT FALSE,
    embedded BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY(problem_id,version_no,file_id),
    CHECK ((purpose IN ('sample-input','sample-answer') AND sample_index>0) OR (purpose IN ('statement','asset') AND sample_index=0 AND preview='')),
    FOREIGN KEY(problem_id,version_no) REFERENCES problem_versions(problem_id,version_no) ON DELETE CASCADE,
    FOREIGN KEY(problem_id,blob_sha256) REFERENCES problem_blobs(problem_id,sha256) DEFERRABLE INITIALLY DEFERRED
);
CREATE TRIGGER problem_version_files_immutable BEFORE UPDATE ON problem_version_files FOR EACH ROW EXECUTE FUNCTION protect_problem_release();
CREATE INDEX problem_version_files_blob ON problem_version_files(problem_id,blob_sha256);

CREATE TABLE problem_blob_uploads (
    problem_id UUID NOT NULL,
    sha256 TEXT NOT NULL,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(problem_id,sha256,actor_id),
    FOREIGN KEY(problem_id,sha256) REFERENCES problem_blobs(problem_id,sha256) ON DELETE CASCADE
);
CREATE TABLE problem_content_trees (
    problem_id UUID NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    tree_hash TEXT NOT NULL CHECK(tree_hash ~ '^[a-f0-9]{64}$'),
    manifest JSONB NOT NULL CHECK(jsonb_typeof(manifest)='object' AND manifest ? 'entries' AND jsonb_typeof(manifest->'entries')='array'),
    summary JSONB NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(summary)='object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(problem_id,tree_hash)
);
-- Derived reference index, maintained with the immutable manifest transaction.
CREATE TABLE problem_tree_blobs (
    problem_id UUID NOT NULL,
    tree_hash TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    PRIMARY KEY(problem_id,tree_hash,sha256),
    FOREIGN KEY(problem_id,tree_hash) REFERENCES problem_content_trees(problem_id,tree_hash) ON DELETE CASCADE,
    FOREIGN KEY(problem_id,sha256) REFERENCES problem_blobs(problem_id,sha256) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX ix_problem_tree_blobs_blob ON problem_tree_blobs(problem_id,sha256);
CREATE TABLE problem_commits (
    problem_id UUID NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    revision BIGINT NOT NULL CHECK(revision>0),
    parent_revision BIGINT,
    tree_hash TEXT NOT NULL,
    author_id UUID REFERENCES users(id) ON DELETE SET NULL,
    message TEXT NOT NULL CHECK(length(btrim(message))>0 AND octet_length(message)<=2000),
    request_id TEXT NOT NULL,
    request_etag UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(problem_id,revision),
    UNIQUE(problem_id,revision,tree_hash),
    UNIQUE(problem_id,author_id,request_id),
    CHECK((revision=1 AND parent_revision IS NULL) OR (revision>1 AND parent_revision IS NOT NULL AND parent_revision=revision-1)),
    FOREIGN KEY(problem_id,parent_revision) REFERENCES problem_commits(problem_id,revision) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY(problem_id,tree_hash) REFERENCES problem_content_trees(problem_id,tree_hash) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE problem_authoring_heads (
    problem_id UUID PRIMARY KEY REFERENCES problems(id) ON DELETE CASCADE,
    revision BIGINT,
    tree_hash TEXT NOT NULL,
    initial_tree_hash TEXT NOT NULL,
    FOREIGN KEY(problem_id,revision,tree_hash) REFERENCES problem_commits(problem_id,revision,tree_hash) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY(problem_id,tree_hash) REFERENCES problem_content_trees(problem_id,tree_hash) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY(problem_id,initial_tree_hash) REFERENCES problem_content_trees(problem_id,tree_hash) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX ix_problem_commits_tree ON problem_commits(problem_id,tree_hash);
CREATE TABLE problem_working_copies (
    problem_id UUID NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    base_revision BIGINT,
    tree_hash TEXT NOT NULL,
    etag UUID NOT NULL DEFAULT gen_random_uuid(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(problem_id,actor_id),
    FOREIGN KEY(problem_id,base_revision) REFERENCES problem_commits(problem_id,revision) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY(problem_id,tree_hash) REFERENCES problem_content_trees(problem_id,tree_hash) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE problem_merge_sessions (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    problem_id UUID NOT NULL,
    actor_id UUID NOT NULL,
    etag UUID NOT NULL DEFAULT gen_random_uuid(),
    copy_etag UUID NOT NULL,
    base_revision BIGINT,
    remote_revision BIGINT,
    local_tree TEXT NOT NULL,
    remote_tree TEXT NOT NULL,
    result_json JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(problem_id,actor_id),
    UNIQUE(id),
    FOREIGN KEY(problem_id,actor_id) REFERENCES problem_working_copies(problem_id,actor_id) ON DELETE CASCADE,
    FOREIGN KEY(problem_id,base_revision) REFERENCES problem_commits(problem_id,revision) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY(problem_id,remote_revision) REFERENCES problem_commits(problem_id,revision) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY(problem_id,local_tree) REFERENCES problem_content_trees(problem_id,tree_hash) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY(problem_id,remote_tree) REFERENCES problem_content_trees(problem_id,tree_hash) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE problem_imports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    problem_id UUID NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    base_etag UUID NOT NULL,
    archive_hash TEXT NOT NULL,
    tree_hash TEXT NOT NULL,
    plan_json JSONB NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL DEFAULT now()+interval '1 hour',
    applied_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY(problem_id,archive_hash) REFERENCES problem_blobs(problem_id,sha256) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY(problem_id,tree_hash) REFERENCES problem_content_trees(problem_id,tree_hash) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX ix_problem_imports_actor ON problem_imports(problem_id,actor_id,created_at DESC);
CREATE FUNCTION protect_authoring_content() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_TABLE_NAME='problem_commits' THEN
        IF (to_jsonb(NEW)-'author_id') IS DISTINCT FROM (to_jsonb(OLD)-'author_id') THEN
            RAISE EXCEPTION 'authoring commits are immutable' USING ERRCODE='23514';
        END IF;
    ELSIF NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'authoring content is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER problem_commits_immutable BEFORE UPDATE ON problem_commits FOR EACH ROW EXECUTE FUNCTION protect_authoring_content();
CREATE TRIGGER problem_trees_immutable BEFORE UPDATE ON problem_content_trees FOR EACH ROW EXECUTE FUNCTION protect_authoring_content();
CREATE TRIGGER problem_blobs_immutable BEFORE UPDATE ON problem_blobs FOR EACH ROW EXECUTE FUNCTION protect_authoring_content();

-- Frozen authoring checks use the same lease fencing as judge jobs.
CREATE TABLE problem_build_jobs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    problem_id       UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    input_json       JSONB NOT NULL,
    source_tree_hash TEXT NOT NULL,
    source_revision BIGINT,
    data_hash TEXT NOT NULL,
    check_policy TEXT NOT NULL,
    toolchain_key TEXT NOT NULL DEFAULT '',
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
    validation_json  JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(validation_json) = 'array'),
    package_path     TEXT NOT NULL DEFAULT '',
    package_sha256   TEXT NOT NULL DEFAULT '',
    package_cases    INTEGER NOT NULL DEFAULT 0,
    package_manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ,
    UNIQUE(problem_id,id),
    CHECK (data_hash ~ '^[a-f0-9]{64}$' AND check_policy<>''),
    CHECK (state<>'succeeded' OR toolchain_key ~ '^[a-f0-9]{64}$'),
    FOREIGN KEY(problem_id,source_tree_hash) REFERENCES problem_content_trees(problem_id,tree_hash) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY(problem_id,source_revision,source_tree_hash) REFERENCES problem_commits(problem_id,revision,tree_hash) DEFERRABLE INITIALLY DEFERRED
);


CREATE UNIQUE INDEX ux_problem_checks_active
    ON problem_build_jobs(problem_id,source_tree_hash,created_by)
    WHERE state IN ('queued','running') AND source_tree_hash IS NOT NULL;

CREATE FUNCTION protect_problem_build_input() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.problem_id,NEW.input_json,NEW.source_tree_hash,NEW.source_revision,NEW.data_hash,NEW.check_policy)
        IS DISTINCT FROM
       (OLD.problem_id,OLD.input_json,OLD.source_tree_hash,OLD.source_revision,OLD.data_hash,OLD.check_policy) THEN
        RAISE EXCEPTION 'build inputs are immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER problem_build_input_immutable BEFORE UPDATE ON problem_build_jobs FOR EACH ROW EXECUTE FUNCTION protect_problem_build_input();
ALTER TABLE problem_versions ADD CONSTRAINT problem_versions_source_commit FOREIGN KEY(problem_id,source_revision,source_tree_hash) REFERENCES problem_commits(problem_id,revision,tree_hash) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE problem_versions ADD CONSTRAINT problem_versions_source_check FOREIGN KEY(problem_id,check_id) REFERENCES problem_build_jobs(problem_id,id) DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX ix_problem_versions_storage_path ON problem_versions(testdata_path text_pattern_ops) WHERE testdata_path<>'';
CREATE INDEX ix_problem_builds_storage_path ON problem_build_jobs(package_path text_pattern_ops) WHERE package_path<>'';

CREATE INDEX idx_problem_build_jobs_claim
    ON problem_build_jobs (state, priority DESC, available_at, created_at);

CREATE INDEX idx_problem_build_jobs_expired_lease
    ON problem_build_jobs (lease_expires_at)
    WHERE state = 'running';

CREATE INDEX idx_problem_build_jobs_problem
    ON problem_build_jobs (problem_id, created_at DESC);

-- ---------- Contests ----------
CREATE TABLE contests (
    domain_id UUID NOT NULL REFERENCES domains(id),
    public_id BIGINT NOT NULL,
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title              TEXT NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    rule               TEXT NOT NULL DEFAULT 'icpc' CHECK (rule IN ('icpc', 'ioi', 'oi', 'leduo', 'cf')),
    medal_mode         TEXT NOT NULL DEFAULT 'none' CHECK (medal_mode IN ('none', 'count', 'percentage')),
    medal_gold         INTEGER NOT NULL DEFAULT 0 CHECK (medal_gold BETWEEN 0 AND 100000),
    medal_silver       INTEGER NOT NULL DEFAULT 0 CHECK (medal_silver BETWEEN 0 AND 100000),
    medal_bronze       INTEGER NOT NULL DEFAULT 0 CHECK (medal_bronze BETWEEN 0 AND 100000),
    CONSTRAINT contests_medal_percentage_check CHECK (medal_mode <> 'percentage' OR medal_gold + medal_silver + medal_bronze <= 100),
    begin_at           TIMESTAMPTZ NOT NULL,
    end_at             TIMESTAMPTZ NOT NULL,
    freeze_at          TIMESTAMPTZ,          -- 封榜时间,NULL=不封榜
    unfreeze_at        TIMESTAMPTZ,
    visibility         TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private', 'password')),
    password_hash      TEXT NOT NULL DEFAULT '',
    rankboard_visible  BOOLEAN NOT NULL DEFAULT TRUE,
    show_problem_metadata BOOLEAN NOT NULL DEFAULT FALSE,
    submission_visibility TEXT NOT NULL DEFAULT 'own' CHECK (submission_visibility IN ('own', 'after_end', 'during')),
    source_code_visibility TEXT NOT NULL DEFAULT 'own' CHECK (source_code_visibility IN ('own', 'after_end')),
    frozen_submission_visibility TEXT NOT NULL DEFAULT 'pending' CHECK (frozen_submission_visibility IN ('hidden', 'pending')),
    penalty_minutes    INTEGER NOT NULL DEFAULT 20 CHECK (penalty_minutes >= 0 AND penalty_minutes <= 1440),
    penalize_compile_error BOOLEAN NOT NULL DEFAULT TRUE,
    feedback           TEXT NOT NULL DEFAULT 'full' CHECK (feedback IN ('full', 'summary', 'first_error', 'none')),
    owner_id           UUID NOT NULL REFERENCES users(id),
    admission          TEXT NOT NULL DEFAULT 'members' CHECK (admission IN ('members', 'restricted')),
    allow_self_registration BOOLEAN NOT NULL DEFAULT TRUE,
    allow_late_registration BOOLEAN NOT NULL DEFAULT TRUE,
    created_by         UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT contests_unfreeze_after_freeze
        CHECK (unfreeze_at IS NULL OR freeze_at IS NULL OR unfreeze_at >= freeze_at)
);

ALTER TABLE contests ADD UNIQUE(domain_id,public_id);
ALTER TABLE contests ADD UNIQUE(domain_id,id);
CREATE TRIGGER contests_number BEFORE INSERT ON contests FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('contests','1');
CREATE TRIGGER contests_identity BEFORE UPDATE OF id,domain_id,public_id ON contests FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();

CREATE INDEX idx_contests_begin ON contests (domain_id, begin_at DESC);

CREATE TABLE contest_problems (
    domain_id UUID NOT NULL REFERENCES domains(id),
    contest_id UUID NOT NULL REFERENCES contests (id) ON DELETE CASCADE,
    problem_id UUID NOT NULL REFERENCES problems (id),
    problem_version INTEGER NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    label      TEXT NOT NULL DEFAULT '',
    color      TEXT NOT NULL DEFAULT '',
    points     INTEGER NOT NULL DEFAULT 100 CHECK (points >= 0),
    PRIMARY KEY (contest_id, problem_id),
    FOREIGN KEY(domain_id,contest_id) REFERENCES contests(domain_id,id),
    FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id),
    FOREIGN KEY(problem_id,problem_version) REFERENCES problem_versions(problem_id,version_no)
);CREATE FUNCTION pin_contest_problem_version() RETURNS trigger LANGUAGE plpgsql AS $$
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
    domain_id UUID NOT NULL REFERENCES domains(id),
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

ALTER TABLE contest_access ADD FOREIGN KEY(domain_id,contest_id) REFERENCES contests(domain_id,id);
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
    domain_id UUID NOT NULL REFERENCES domains(id),
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
    PRIMARY KEY (contest_id, user_id, problem_id),
    FOREIGN KEY(domain_id,contest_id) REFERENCES contests(domain_id,id),
    FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id)
);
-- ---------- Contest clarifications ----------
CREATE TABLE clarifications (
    domain_id UUID NOT NULL REFERENCES domains(id),
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
    CHECK (parent_id IS NULL OR recipient_id IS NOT NULL OR from_jury),
    UNIQUE(contest_id,id),
    FOREIGN KEY(domain_id,contest_id) REFERENCES contests(domain_id,id),
    FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id),
    FOREIGN KEY(contest_id,parent_id) REFERENCES clarifications(contest_id,id)
);
CREATE INDEX idx_clarifications_contest ON clarifications (contest_id, created_at DESC);
CREATE INDEX idx_clarifications_thread ON clarifications (parent_id);
CREATE INDEX idx_clarifications_recipient ON clarifications (contest_id, recipient_id);
CREATE INDEX idx_clarifications_open
    ON clarifications (contest_id, answered)
    WHERE parent_id IS NULL AND NOT from_jury;

-- ---------- Submissions ----------
-- submissions 保存用户可见的判题状态；调度状态由 judge_jobs 独立维护。
-- Submission facts are separate from each immutable-input evaluation generation.
CREATE TABLE submissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id),
    public_id BIGINT NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    problem_id UUID NOT NULL REFERENCES problems(id),
    initial_problem_version INTEGER NOT NULL,
    contest_id UUID REFERENCES contests(id) ON DELETE CASCADE,
    language TEXT NOT NULL,
    source_code TEXT NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    judge_generation INTEGER NOT NULL DEFAULT 1 CHECK(judge_generation > 0),
    result_generation INTEGER NOT NULL DEFAULT 1 CHECK(result_generation > 0 AND result_generation <= judge_generation),
    UNIQUE(domain_id,public_id),
    UNIQUE(domain_id,id),
    UNIQUE(id,problem_id),
    FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id),
    FOREIGN KEY(domain_id,contest_id) REFERENCES contests(domain_id,id),
    FOREIGN KEY(problem_id,initial_problem_version) REFERENCES problem_versions(problem_id,version_no)
);
CREATE TRIGGER submissions_number BEFORE INSERT ON submissions FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('submissions','1');
CREATE TRIGGER submissions_identity BEFORE UPDATE OF id,domain_id,public_id ON submissions FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();

CREATE FUNCTION valid_judgement_cases(cases JSONB) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
    SELECT jsonb_typeof(cases)='array'
       AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(cases) c
                      WHERE c->>'caseIndex' IS NULL OR (c->>'caseIndex')::integer <= 0)
       AND jsonb_array_length(cases) = (SELECT count(DISTINCT (c->>'caseIndex')::integer) FROM jsonb_array_elements(cases) c);
$$;
CREATE TABLE judgements (
    submission_id UUID NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    generation INTEGER NOT NULL CHECK(generation > 0),
    problem_id UUID NOT NULL,
    problem_version INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'Pending' CHECK(status IN (
        'Pending','Judging','Accepted','Wrong Answer','Time Limit Exceeded','Memory Limit Exceeded',
        'Runtime Error','Compile Error','Output Limit Exceeded','System Error','Skipped')),
    score INTEGER NOT NULL DEFAULT 0,
    total_time_ms INTEGER NOT NULL DEFAULT 0,
    peak_memory_kb INTEGER NOT NULL DEFAULT 0,
    compile_result TEXT NOT NULL DEFAULT '',
    case_results JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(valid_judgement_cases(case_results)),
    judged_cases INTEGER NOT NULL DEFAULT 0 CHECK(judged_cases >= 0),
    total_cases INTEGER NOT NULL DEFAULT 0 CHECK(total_cases >= 0),
    judged_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(submission_id,generation),
    FOREIGN KEY(submission_id,problem_id) REFERENCES submissions(id,problem_id) ON DELETE CASCADE,
    FOREIGN KEY(problem_id,problem_version) REFERENCES problem_versions(problem_id,version_no)
);
ALTER TABLE submissions ADD CONSTRAINT submissions_result FOREIGN KEY(id,result_generation) REFERENCES judgements(submission_id,generation) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE submissions ADD CONSTRAINT submissions_generation FOREIGN KEY(id,judge_generation) REFERENCES judgements(submission_id,generation) DEFERRABLE INITIALLY DEFERRED;

CREATE FUNCTION protect_judgement_input() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.submission_id,NEW.generation,NEW.problem_id,NEW.problem_version) IS DISTINCT FROM
       (OLD.submission_id,OLD.generation,OLD.problem_id,OLD.problem_version) THEN
        RAISE EXCEPTION 'judgement inputs are immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER judgements_input BEFORE UPDATE ON judgements FOR EACH ROW EXECUTE FUNCTION protect_judgement_input();

-- Read projection: the adopted result may be an older generation after cancelling a rejudge.
CREATE VIEW submission_results AS
SELECT s.id,s.domain_id,s.public_id,s.user_id,s.problem_id,s.contest_id,s.language,s.source_code,
       s.submitted_at,s.judge_generation,s.result_generation,j.problem_version,j.status,j.score,
       j.total_time_ms,j.peak_memory_kb,j.compile_result,j.case_results,j.judged_cases,j.total_cases,j.judged_at
FROM submissions s JOIN judgements j ON j.submission_id=s.id AND j.generation=s.result_generation;

-- Shared row visibility policy for list, count, detail and progress.
-- It returns facts; field disclosure is owned by the application projection.
CREATE FUNCTION visible_submissions(scope_domain UUID, scope_viewer TEXT, scope_manager BOOLEAN,
                                   scope_member BOOLEAN, scope_submit BOOLEAN, observed_at TIMESTAMPTZ)
RETURNS SETOF submission_results LANGUAGE sql STABLE AS $$
 SELECT s.* FROM submission_results s JOIN problems p ON p.id=s.problem_id
 WHERE s.domain_id=scope_domain::uuid AND (
		scope_manager::boolean
		OR s.user_id = NULLIF(scope_viewer::text,'')::uuid
		OR (
			s.contest_id IS NULL
			AND ((p.visibility = 'public' AND p.published_version IS NOT NULL) OR (scope_member::boolean AND (p.owner_id = NULLIF(scope_viewer::text,'')::uuid OR EXISTS (
			 SELECT 1 FROM problem_access a WHERE a.problem_id=p.id AND (a.user_id=NULLIF(scope_viewer::text,'')::uuid OR a.group_id IN (
			 SELECT group_id FROM domain_group_members WHERE domain_id=p.domain_id AND user_id=NULLIF(scope_viewer::text,'')::uuid))))))
		)
		OR EXISTS (
			SELECT 1 FROM contests c
			WHERE c.id = s.contest_id
			  AND (
				(scope_member::boolean AND c.owner_id = NULLIF(scope_viewer::text,'')::uuid)
				OR EXISTS (
					SELECT 1 FROM contest_staff staff
					WHERE staff.contest_id = c.id AND staff.user_id = NULLIF(scope_viewer::text,'')::uuid
				)
				OR (
					(c.submission_visibility='during' AND c.begin_at<=observed_at::timestamptz OR c.submission_visibility='after_end' AND c.end_at<observed_at::timestamptz)
					AND NOT (
						c.rule <> 'oi' AND c.frozen_submission_visibility='hidden'
                        AND s.submitted_at >= c.freeze_at
                        AND c.freeze_at IS NOT NULL
						AND observed_at::timestamptz > c.freeze_at
						AND (c.unfreeze_at IS NULL OR observed_at::timestamptz < c.unfreeze_at)
					)
					AND (
						(c.visibility = 'public' AND (
							p.visibility = 'public'
							OR EXISTS (
								SELECT 1 FROM contest_participants participant
								WHERE participant.contest_id = c.id
								  AND participant.user_id = NULLIF(scope_viewer::text,'')::uuid
							)
						))
						OR (c.visibility IN ('password','private') AND (c.visibility='password' OR (scope_submit::boolean AND scope_member::boolean AND (c.admission='members' OR EXISTS(SELECT 1 FROM contest_access a WHERE a.contest_id=c.id AND a.role='participant' AND (a.user_id=NULLIF(scope_viewer::text,'')::uuid OR a.group_id IN(SELECT group_id FROM domain_group_members WHERE domain_id=c.domain_id AND user_id=NULLIF(scope_viewer::text,'')::uuid)))))) AND EXISTS (
							SELECT 1 FROM contest_participants participant
							WHERE participant.contest_id = c.id
							  AND participant.user_id = NULLIF(scope_viewer::text,'')::uuid
						))
					)
				)
			  )
		)
	);
$$;

CREATE INDEX idx_submissions_user_time ON submissions(user_id,submitted_at DESC);
CREATE INDEX idx_submissions_user_problem ON submissions(user_id,problem_id,submitted_at DESC);
CREATE INDEX idx_submissions_domain_time ON submissions(domain_id,submitted_at DESC,id DESC);
CREATE INDEX idx_submissions_problem ON submissions(problem_id);
CREATE INDEX idx_submissions_contest ON submissions(contest_id);
CREATE INDEX idx_submissions_contest_cell ON submissions(contest_id,user_id,problem_id,submitted_at) WHERE contest_id IS NOT NULL;
CREATE INDEX idx_judgements_status ON judgements(status);

CREATE TABLE judge_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id UUID NOT NULL,
    generation INTEGER NOT NULL CHECK(generation > 0),
    state TEXT NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','completed','cancelled','dead')),
    priority INTEGER NOT NULL DEFAULT 0,
    attempt INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    worker_id TEXT,
    lease_token UUID,
    lease_expires_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    UNIQUE(submission_id,generation),
    FOREIGN KEY(submission_id,generation) REFERENCES judgements(submission_id,generation) ON DELETE CASCADE
);
CREATE FUNCTION protect_judge_job_input() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.submission_id,NEW.generation) IS DISTINCT FROM (OLD.submission_id,OLD.generation) THEN
        RAISE EXCEPTION 'judge job target is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER judge_jobs_input BEFORE UPDATE ON judge_jobs FOR EACH ROW EXECUTE FUNCTION protect_judge_job_input();
CREATE INDEX idx_judge_jobs_claim ON judge_jobs(state,priority DESC,available_at,created_at);
CREATE INDEX idx_judge_jobs_expired_lease ON judge_jobs(lease_expires_at) WHERE state='running';

-- ---------- Rejudging ----------
CREATE TABLE rejudgings (
    domain_id UUID NOT NULL REFERENCES domains(id),
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

ALTER TABLE rejudgings ADD UNIQUE(domain_id,id);
ALTER TABLE rejudgings ADD FOREIGN KEY(domain_id,contest_id) REFERENCES contests(domain_id,id);
ALTER TABLE rejudgings ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);

CREATE INDEX idx_rejudgings_contest ON rejudgings (contest_id, created_at DESC);
CREATE INDEX idx_rejudgings_created ON rejudgings (domain_id, created_at DESC);

CREATE TABLE rejudging_submissions (
    domain_id UUID NOT NULL REFERENCES domains(id),
    rejudging_id             UUID NOT NULL REFERENCES rejudgings (id) ON DELETE CASCADE,
    submission_id            UUID NOT NULL REFERENCES submissions (id) ON DELETE CASCADE,
    generation               INTEGER NOT NULL CHECK (generation > 0),
    prior_generation INTEGER NOT NULL CHECK(prior_generation > 0),
    PRIMARY KEY (rejudging_id, submission_id),
    FOREIGN KEY(submission_id,generation) REFERENCES judgements(submission_id,generation),
    FOREIGN KEY(submission_id,prior_generation) REFERENCES judgements(submission_id,generation),
    FOREIGN KEY(domain_id,rejudging_id) REFERENCES rejudgings(domain_id,id),
    FOREIGN KEY(domain_id,submission_id) REFERENCES submissions(domain_id,id)
);
CREATE INDEX idx_rejudging_submissions_submission
    ON rejudging_submissions (submission_id);

-- ---------- Problem sets ----------
CREATE TABLE problem_sets (
    domain_id UUID NOT NULL REFERENCES domains(id),
    public_id BIGINT NOT NULL,
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    author_id   UUID REFERENCES users (id) ON DELETE SET NULL,
    owner_id    UUID NOT NULL REFERENCES users (id),
    visibility  TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(domain_id,public_id),
    UNIQUE(domain_id,id)
);

CREATE TRIGGER problem_sets_number BEFORE INSERT ON problem_sets FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('problem_sets','1');
CREATE TRIGGER problem_sets_identity BEFORE UPDATE OF id,domain_id,public_id ON problem_sets FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();

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
    FOREIGN KEY(domain_id,group_id) REFERENCES domain_groups(domain_id,id) ON DELETE CASCADE,
    FOREIGN KEY(domain_id,set_id) REFERENCES problem_sets(domain_id,id)
);CREATE UNIQUE INDEX problem_set_access_user ON problem_set_access(set_id,user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX problem_set_access_group ON problem_set_access(set_id,group_id) WHERE group_id IS NOT NULL;

CREATE TABLE problem_set_problems (
    domain_id UUID NOT NULL REFERENCES domains(id),
    set_id     UUID NOT NULL REFERENCES problem_sets (id) ON DELETE CASCADE,
    problem_id UUID NOT NULL REFERENCES problems (id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    note       TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (set_id, problem_id),
    FOREIGN KEY(domain_id,set_id) REFERENCES problem_sets(domain_id,id),
    FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id)
);
-- ---------- Editorials (题解) / Discussions ----------
CREATE TABLE editorials (
    domain_id UUID NOT NULL REFERENCES domains(id),
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
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(domain_id,public_id),
    UNIQUE(domain_id,id),
    FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id)
);

CREATE TRIGGER editorials_number BEFORE INSERT ON editorials FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('editorials','1');
CREATE TRIGGER editorials_identity BEFORE UPDATE OF id,domain_id,public_id ON editorials FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();

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
    domain_id UUID NOT NULL REFERENCES domains(id),
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

ALTER TABLE discussion_posts ADD UNIQUE(domain_id,id);
ALTER TABLE discussion_posts ADD FOREIGN KEY(domain_id,problem_id) REFERENCES problems(domain_id,id);
ALTER TABLE discussion_posts ADD FOREIGN KEY(domain_id,editorial_id) REFERENCES editorials(domain_id,id);
ALTER TABLE discussion_posts ADD FOREIGN KEY(domain_id,parent_id) REFERENCES discussion_posts(domain_id,id);
ALTER TABLE discussion_posts ADD UNIQUE(problem_id,id);
ALTER TABLE discussion_posts ADD UNIQUE(editorial_id,id);
ALTER TABLE discussion_posts ADD FOREIGN KEY(problem_id,parent_id) REFERENCES discussion_posts(problem_id,id);
ALTER TABLE discussion_posts ADD FOREIGN KEY(editorial_id,parent_id) REFERENCES discussion_posts(editorial_id,id);

CREATE INDEX idx_discussion_problem ON discussion_posts (problem_id);
CREATE INDEX idx_discussion_editorial ON discussion_posts (editorial_id);

-- ---------- Announcements ----------
CREATE TABLE announcements (
    domain_id UUID NOT NULL REFERENCES domains(id),
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    public_id   BIGINT NOT NULL,
    title       TEXT NOT NULL,
    content_md  TEXT NOT NULL DEFAULT '',
    pinned      BOOLEAN NOT NULL DEFAULT FALSE,
    pinned_until TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    published   BOOLEAN NOT NULL DEFAULT TRUE,
    created_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(domain_id,public_id)
);

CREATE TRIGGER announcements_number BEFORE INSERT ON announcements FOR EACH ROW EXECUTE FUNCTION allocate_domain_number('announcements','1');
CREATE TRIGGER announcements_identity BEFORE UPDATE OF id,domain_id,public_id ON announcements FOR EACH ROW EXECUTE FUNCTION protect_resource_identity();

CREATE INDEX idx_announcements_feed
    ON announcements (domain_id, published_at DESC, id DESC)
    WHERE published;

-- ---------- Bootstrap: official domain and built-in roles ----------
INSERT INTO domains(id, slug, name, is_official, visibility, join_policy)
VALUES ('00000000-0000-4000-8000-000000000001', 'official', '官方', TRUE, 'public', 'open');
INSERT INTO domain_roles(domain_id, key, name, permissions, builtin) VALUES
('00000000-0000-4000-8000-000000000001','admin','域管理员','["domain.settings.manage","domain.members.manage","domain.roles.manage","domain.groups.manage","domain.resources.manage","problem.create","contest.create","problem_set.create","submission.create","content.create"]',TRUE),
('00000000-0000-4000-8000-000000000001','author','出题人','["problem.create","contest.create","problem_set.create","submission.create","content.create"]',TRUE),
('00000000-0000-4000-8000-000000000001','member','成员','["problem_set.create","submission.create","content.create"]',TRUE),
('00000000-0000-4000-8000-000000000001','viewer','只读成员','[]',TRUE);
