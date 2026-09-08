-- name: GetDomainScopeByID :one
SELECT d.id,d.slug,d.name,d.description,d.owner_id,COALESCE(owner.username,'') AS owner_name,
    d.is_official,d.visibility,d.join_policy,d.archived,d.created_at,d.updated_at,
    COALESCE(m.role_key,'') AS member_role,COALESCE(m.status,'') AS member_status,COALESCE(r.permissions,'[]'::jsonb) AS role_permissions FROM domains d LEFT JOIN users owner ON owner.id=d.owner_id
    LEFT JOIN domain_members m ON m.domain_id=d.id AND m.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
    LEFT JOIN domain_roles r ON r.domain_id=m.domain_id AND r.key=m.role_key WHERE d.id=sqlc.arg(domain_id)::uuid;

-- name: LockDomainForResourceWrite :one
SELECT 1 FROM domains WHERE id=sqlc.arg(domain_id)::uuid FOR SHARE;

-- name: GetDomainScopeBySlug :one
SELECT d.id,d.slug,d.name,d.description,d.owner_id,COALESCE(owner.username,'') AS owner_name,
    d.is_official,d.visibility,d.join_policy,d.archived,d.created_at,d.updated_at,
    COALESCE(m.role_key,'') AS member_role,COALESCE(m.status,'') AS member_status,COALESCE(r.permissions,'[]'::jsonb) AS role_permissions FROM domains d LEFT JOIN users owner ON owner.id=d.owner_id
    LEFT JOIN domain_members m ON m.domain_id=d.id AND m.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
    LEFT JOIN domain_roles r ON r.domain_id=m.domain_id AND r.key=m.role_key WHERE d.slug=sqlc.arg(slug)::text;

-- name: CountAccessibleDomains :one
SELECT count(*) FROM domains d LEFT JOIN users owner ON owner.id=d.owner_id
    LEFT JOIN domain_members m ON m.domain_id=d.id AND m.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
    LEFT JOIN domain_roles r ON r.domain_id=m.domain_id AND r.key=m.role_key WHERE (sqlc.arg(is_admin)::boolean OR d.visibility='public' OR m.status IN ('active','invited','pending'))
        AND (NOT d.archived OR sqlc.arg(is_admin)::boolean OR m.status='active') AND (d.name ILIKE sqlc.arg(keyword)::text OR d.slug ILIKE sqlc.arg(keyword)::text);

-- name: ListAccessibleDomains :many
SELECT d.id,d.slug,d.name,d.description,d.owner_id,COALESCE(owner.username,'') AS owner_name,
    d.is_official,d.visibility,d.join_policy,d.archived,d.created_at,d.updated_at,
    COALESCE(m.role_key,'') AS member_role,COALESCE(m.status,'') AS member_status,COALESCE(r.permissions,'[]'::jsonb) AS role_permissions FROM domains d LEFT JOIN users owner ON owner.id=d.owner_id
    LEFT JOIN domain_members m ON m.domain_id=d.id AND m.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid
    LEFT JOIN domain_roles r ON r.domain_id=m.domain_id AND r.key=m.role_key WHERE (sqlc.arg(is_admin)::boolean OR d.visibility='public' OR m.status IN ('active','invited','pending'))
        AND (NOT d.archived OR sqlc.arg(is_admin)::boolean OR m.status='active') AND (d.name ILIKE sqlc.arg(keyword)::text OR d.slug ILIKE sqlc.arg(keyword)::text) ORDER BY d.is_official DESC,d.created_at,d.id LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: LockDomainForGovernance :one
SELECT id FROM domains WHERE slug=sqlc.arg(slug)::text FOR UPDATE;

-- name: RecordDomainAudit :exec
INSERT INTO domain_audit_events(domain_id,actor_id,action,target) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(actor_id)::uuid,sqlc.arg(action)::text,sqlc.arg(target)::text);

-- name: SeedDomainRole :exec
INSERT INTO domain_roles(domain_id,key,name,permissions,builtin) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(key)::text,sqlc.arg(name)::text,sqlc.arg(permissions)::jsonb,TRUE) ON CONFLICT(domain_id,key) DO NOTHING;

-- name: EnsureOfficialDomain :exec
INSERT INTO domains(id,slug,name,is_official,visibility,join_policy) VALUES(sqlc.arg(domain_id)::uuid,'official','官方',TRUE,'public','open') ON CONFLICT(id) DO NOTHING;

-- name: CreateDomain :one
INSERT INTO domains(slug,name,description,owner_id,visibility,join_policy)
        VALUES(sqlc.arg(slug)::text,sqlc.arg(name)::text,sqlc.arg(description)::text,sqlc.arg(owner_id)::uuid,sqlc.arg(visibility)::text,sqlc.arg(join_policy)::text) RETURNING id;

-- name: AddDomainOwner :exec
INSERT INTO domain_members(domain_id,user_id,role_key,status) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(user_id)::uuid,'admin','active');

-- name: GetDomainRole :one
SELECT key,name,permissions,builtin FROM domain_roles WHERE domain_id=sqlc.arg(domain_id)::uuid AND key=sqlc.arg(key)::text;

-- name: ListDomainRoles :many
SELECT key,name,permissions,builtin FROM domain_roles WHERE domain_id=sqlc.arg(domain_id)::uuid ORDER BY builtin DESC,key;

-- name: LookupDomainMember :one
SELECT u.id AS user_id,u.username,COALESCE(m.role_key,'') AS role_key,COALESCE(m.status,'') AS status,COALESCE(m.joined_at,u.created_at)::timestamptz AS joined_at
        FROM users u LEFT JOIN domain_members m ON m.user_id=u.id AND m.domain_id=sqlc.arg(domain_id)::uuid
        WHERE u.username=sqlc.arg(username)::text AND (NOT sqlc.arg(active_account_only)::boolean OR u.disabled_at IS NULL) FOR SHARE OF u;

-- name: UpsertDomainMember :exec
INSERT INTO domain_members(domain_id,user_id,role_key,status) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(user_id)::uuid,sqlc.arg(role_key)::text,sqlc.arg(status)::text)
        ON CONFLICT(domain_id,user_id) DO UPDATE SET role_key=EXCLUDED.role_key,status=EXCLUDED.status,updated_at=now();

-- name: CountDomainMembers :one
SELECT count(*) FROM domain_members m JOIN users u ON u.id=m.user_id WHERE m.domain_id=sqlc.arg(domain_id)::uuid AND u.username ILIKE sqlc.arg(keyword)::text;

-- name: ListDomainMembers :many
SELECT m.user_id,u.username,m.role_key,m.status,m.joined_at FROM domain_members m JOIN users u ON u.id=m.user_id
        WHERE m.domain_id=sqlc.arg(domain_id)::uuid AND u.username ILIKE sqlc.arg(keyword)::text ORDER BY m.joined_at,m.user_id LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: CountDomainGroups :one
SELECT count(*) FROM domain_groups WHERE domain_id=sqlc.arg(domain_id)::uuid AND name ILIKE sqlc.arg(keyword)::text;

-- name: ListDomainGroups :many
SELECT g.id,g.public_id,g.domain_id,g.name,g.description,g.owner_id,u.username AS owner_name,g.created_at,
    (SELECT count(*) FROM domain_group_members allgm JOIN domain_members dm ON dm.domain_id=allgm.domain_id AND dm.user_id=allgm.user_id WHERE allgm.group_id=g.id AND dm.status='active')::integer AS member_count,COALESCE(gm.role,'') AS viewer_role FROM domain_groups g JOIN users u ON u.id=g.owner_id
    LEFT JOIN domain_group_members gm ON gm.group_id=g.id AND gm.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid WHERE g.domain_id=sqlc.arg(domain_id)::uuid AND g.name ILIKE sqlc.arg(keyword)::text ORDER BY g.name,g.id LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: CountGroupMembers :one
SELECT count(*) FROM domain_group_members gm JOIN domain_members dm ON dm.domain_id=gm.domain_id AND dm.user_id=gm.user_id
        JOIN users u ON u.id=gm.user_id WHERE gm.domain_id=sqlc.arg(domain_id)::uuid AND gm.group_id=sqlc.arg(group_id)::uuid AND dm.status='active' AND u.username ILIKE sqlc.arg(keyword)::text;

-- name: ListGroupMembers :many
SELECT gm.user_id,u.username,gm.role FROM domain_group_members gm
        JOIN domain_members dm ON dm.domain_id=gm.domain_id AND dm.user_id=gm.user_id
        JOIN users u ON u.id=gm.user_id WHERE gm.domain_id=sqlc.arg(domain_id)::uuid AND gm.group_id=sqlc.arg(group_id)::uuid AND dm.status='active' AND u.username ILIKE sqlc.arg(keyword)::text ORDER BY u.username LIMIT sqlc.arg(page_limit)::integer OFFSET sqlc.arg(page_offset)::integer;

-- name: UpdateDomain :exec
UPDATE domains SET name=sqlc.arg(name)::text,description=sqlc.arg(description)::text,visibility=sqlc.arg(visibility)::text,join_policy=sqlc.arg(join_policy)::text,updated_at=now() WHERE id=sqlc.arg(domain_id)::uuid;

-- name: ArchiveDomain :exec
UPDATE domains SET archived=sqlc.arg(archived)::boolean,updated_at=now() WHERE id=sqlc.arg(domain_id)::uuid;

-- name: PromoteNewDomainOwner :exec
UPDATE domain_members SET role_key='admin',updated_at=now() WHERE domain_id=sqlc.arg(domain_id)::uuid AND user_id=sqlc.arg(user_id)::uuid;

-- name: TransferDomainOwner :exec
UPDATE domains SET owner_id=sqlc.arg(owner_id)::uuid,updated_at=now() WHERE id=sqlc.arg(domain_id)::uuid;

-- name: UpsertDomainRole :exec
INSERT INTO domain_roles(domain_id,key,name,permissions) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(key)::text,sqlc.arg(name)::text,sqlc.arg(permissions)::jsonb)
        ON CONFLICT(domain_id,key) DO UPDATE SET name=EXCLUDED.name,permissions=EXCLUDED.permissions WHERE NOT domain_roles.builtin;

-- name: DeleteDomainRole :exec
DELETE FROM domain_roles WHERE domain_id=sqlc.arg(domain_id)::uuid AND key=sqlc.arg(key)::text AND NOT builtin;

-- name: CreateDomainGroup :one
INSERT INTO domain_groups(domain_id,name,description,owner_id) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(name)::text,sqlc.arg(description)::text,sqlc.arg(owner_id)::uuid) RETURNING id;

-- name: UpdateDomainGroup :exec
UPDATE domain_groups SET name=sqlc.arg(name)::text,description=sqlc.arg(description)::text,updated_at=now() WHERE domain_id=sqlc.arg(domain_id)::uuid AND id=sqlc.arg(group_id)::uuid;

-- name: TransferGroupOwner :exec
UPDATE domain_groups SET owner_id=sqlc.arg(owner_id)::uuid,updated_at=now() WHERE domain_id=sqlc.arg(domain_id)::uuid AND id=sqlc.arg(group_id)::uuid;

-- name: DeleteGroupMember :exec
DELETE FROM domain_group_members WHERE domain_id=sqlc.arg(domain_id)::uuid AND group_id=sqlc.arg(group_id)::uuid AND user_id=sqlc.arg(user_id)::uuid;

-- name: UpsertGroupMember :exec
INSERT INTO domain_group_members(domain_id,group_id,user_id,role) VALUES(sqlc.arg(domain_id)::uuid,sqlc.arg(group_id)::uuid,sqlc.arg(user_id)::uuid,sqlc.arg(role)::text)
        ON CONFLICT(group_id,user_id) DO UPDATE SET role=EXCLUDED.role;

-- name: DeleteDomainGroup :exec
DELETE FROM domain_groups WHERE domain_id=sqlc.arg(domain_id)::uuid AND id=sqlc.arg(group_id)::uuid;

-- name: GetDomainGroupByID :one
SELECT g.id,g.public_id,g.domain_id,g.name,g.description,g.owner_id,u.username AS owner_name,g.created_at,
    (SELECT count(*) FROM domain_group_members allgm JOIN domain_members dm ON dm.domain_id=allgm.domain_id AND dm.user_id=allgm.user_id WHERE allgm.group_id=g.id AND dm.status='active')::integer AS member_count,COALESCE(gm.role,'') AS viewer_role FROM domain_groups g JOIN users u ON u.id=g.owner_id
    LEFT JOIN domain_group_members gm ON gm.group_id=g.id AND gm.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid WHERE g.domain_id=sqlc.arg(domain_id)::uuid AND g.id=sqlc.arg(group_id)::uuid;

-- name: GetDomainGroupByNumber :one
SELECT g.id,g.public_id,g.domain_id,g.name,g.description,g.owner_id,u.username AS owner_name,g.created_at,
    (SELECT count(*) FROM domain_group_members allgm JOIN domain_members dm ON dm.domain_id=allgm.domain_id AND dm.user_id=allgm.user_id WHERE allgm.group_id=g.id AND dm.status='active')::integer AS member_count,COALESCE(gm.role,'') AS viewer_role FROM domain_groups g JOIN users u ON u.id=g.owner_id
    LEFT JOIN domain_group_members gm ON gm.group_id=g.id AND gm.user_id=NULLIF(sqlc.arg(viewer_id)::text,'')::uuid WHERE g.domain_id=sqlc.arg(domain_id)::uuid AND g.public_id=sqlc.arg(public_id)::bigint;

-- name: GetGovernanceActor :one
SELECT id,username,role='admin' AS is_admin FROM users WHERE id=sqlc.arg(user_id)::uuid AND disabled_at IS NULL;

-- name: LockGovernanceActor :one
SELECT id,username,role='admin' AS is_admin FROM users WHERE id=sqlc.arg(user_id)::uuid AND disabled_at IS NULL FOR SHARE;

-- name: LockResourceShared :exec
SELECT pg_advisory_xact_lock_shared(hashtextextended(sqlc.arg(resource_key)::text,0));

-- name: LockResourceExclusive :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(resource_key)::text,0));
