# Vertex OJ — 数据库设计

PostgreSQL 16 是唯一事实源。数据库访问正迁移到各业务上下文内部的 `sqlc` repository，分层设计与迁移进度见[后端包结构与持久化边界](14-backend-architecture.md)。项目尚未实际部署，完整 schema 直接维护在 `server/migrations/000001_init.up.sql` 与对应 down 文件，不累积过渡 migration。首次正式发布后再切换为只追加的升级策略。

> 开发期修改 `000001_init` 不会升级已经记录 migration version 的旧数据库。优先用新的隔离数据库验证新 schema，并备份需要保留的数据与文件。只有明确决定弃用旧开发数据时才删除对应卷；`docker compose down -v` 会永久删除 Compose 管理的数据库、测试数据与缓存卷，不是普通更新步骤。参见[部署说明](06-deployment.md)。

## 领域关系

题目、比赛、提交、题解、题单和 group 保留 UUID 主键，公开编号使用 `public_id BIGINT` 与 `(domain_id, public_id)` 唯一约束。`domain_number_counters` 按域及资源种类计数，插入触发器在同一事务内分配编号；题目从 1000 开始，其他资源从 1 开始。因此两个域可以分别拥有自己的 1000 号题。并发插入通过 counter 行锁串行化；删除已提交资源后不复用编号。触发器拒绝修改公开身份及资源域。HTTP DTO 将编号序列化为字符串 `publicId`，避免 JavaScript 大整数精度问题。

外键、Worker 协议和请求体中的关联 ID 仍使用 UUID。HTTP 路由在认证及域解析之后，只在当前域把公开编号解析为 UUID，再执行资源权限检查；比赛题号由比赛领域在验证报名、时间和赛务身份后解析。编号可枚举，不作为访问控制手段。没有域前缀的旧资源接口明确绑定官方域，不会按 UUID 搜索其他域。

```text
users ──< auth_sessions
  └──< submissions ──< submission_cases
          ├──< judge_jobs
          ├──> problems ──1 problem_testdata (候选)
          └──> contests ──< contest_submission_cells

problems ──1 problem_workspaces (可变元信息)
         ├──< tags / problem_versions / editorials / discussion_posts
         ├──< problem_statements / problem_files / problem_tests   ← 题目包(源材料)
         └──< problem_build_jobs                                    ← 构建队列
contests ──< contest_problems / contest_participants / contest_staff
         ├──< contest_submission_cells                             ← 积分格(裁判+封榜两套视图)
         └──< clarifications                                       ← 答疑
submissions ──< rejudging_submissions >── rejudgings               ← 重测批次
```

## 域与资源作用域

域底座包含 `domains`、`domain_roles`、`domain_members`、`domain_groups`、`domain_group_members` 与 `domain_audit_events`。官方域由 init 创建；账号注册在同一事务加入官方域。普通域 owner 必须有同域 membership（延迟检查的复合 FK），组与组成员也用同域复合 FK。

资源表、标签及多父关联表已有 `domain_id`。题目标签、比赛题目、题单条目、比赛提交、题解、讨论父节点、重测成员、积分格和澄清使用复合 FK 限制同域，澄清回复还必须属于同一比赛。题面、源程序、测试计划和构建等单父子记录继承题目的域：公开操作校验父题目的域，修改包源材料时在事务内锁定父题目；内部构建领取从受 lease 保护的任务取得父题目，不受官方域默认值限制。

Store 的资源查询、分页 count、更新及删除显式使用 typed domain context。旧的无 scope 持久层调用只访问官方域；不能用于跨域 Worker 查询。全站账号治理及系统队列统计是明确例外，只由站点管理员接口提供。资源 owner、用户/group 协作者、域治理能力及多域 UI 已接入；授权、关联和统计的具体检查见[域与协作](10-domains-and-access.md)及[读模型审计](11-read-policy-audit.md)。

题目已接入独立的 `owner_id NOT NULL`（引用站点账号）与 `problem_access`。授权行只能选择一个用户或 group，分别引用同域成员或同域群组，并以复合 FK 绑定题目域。所有权变化不重写 `author_id` 创建记录。资源写事务取得域共享锁，域角色/成员/组管理取得同一域的排他锁，因此一次已经授权的写入与一次撤权有确定的提交顺序；撤权完成后，新的写入重新计算权限并被拒绝。

## 认证 session

`auth_sessions` 保存：session UUID、user UUID、refresh token hash、有效期、吊销时间、最后使用时间。数据库从不保存可复用 refresh token。轮换使用带旧 hash 条件的单条 `UPDATE`，并发复用同一旧 token 时只有一个请求成功。

Access JWT 的 `sid` 在每次认证时与 active session 联查；角色从 `users` 当前行读取。

## Submission 与 Judge job

`submissions` 保存用户可见结果，并以 `judge_generation` 标识当前评测代次。调度状态独立存入 `judge_jobs`：

- 唯一约束：`(submission_id, generation)`。
- 状态：`queued/running/completed/cancelled/dead`。
- lease 字段：`worker_id/lease_token/lease_expires_at/attempt`。
- claim 索引：`(state, priority DESC, available_at, created_at)`。
- running lease 到期索引：`lease_expires_at WHERE state='running'`。

新建提交、rejudge 和对应 job 始终在同一事务。claim 使用 `FOR UPDATE SKIP LOCKED`；result 锁定 job/submission 并验证 generation 与 lease 后，再原子更新：

1. `judge_jobs` 完成状态；
2. `submissions` 结果 JSONB 快照；
3. `submission_cases` 规范化逐点结果；
4. problem counters；
5. contest score cell。

任一步失败会回滚全部写入。超过最大 attempt 的 job 进入 `dead`，提交使用现有 `System Error` verdict。

`submissions.judged_cases / total_cases` 只服务前端进度显示：claim 时写入 `total_cases` 并把 `judged_cases` 清零，heartbeat 以 `GREATEST` 推进 `judged_cases`（乱序心跳不回退），result 用最终用例数覆盖两者，rejudge 清零。两列不参与判定，也不参与榜单计算。

## 查看者读模型

题库的「已通过 / 尝试过 / 未尝试」和个人主页统计都由 `submissions` 实时推导，不建冗余表——rejudge 与比赛重算会改变既有提交的状态，任何物化副本都需要额外的双写不变量。支撑索引：

- `(user_id, problem_id, status)`：题库逐题状态判定与 `status` 过滤。
- `(user_id, problem_id) WHERE status='Accepted'`：已通过集合，服务个人主页与难度分布。

## 测试数据

测试数据文件不进入数据库。`problem_testdata` 保存当前候选，发布时复制元信息到不可变 `problem_versions`；Server 将每次上传或构建产物写入 `/<problemID>/<sha256>/` 内容寻址目录，Worker 只读。比赛、提交及 judge generation 关联具体发布版本；被引用的题目不允许硬删除，因此不能删除仍在使用的数据目录。

`checker` 取 `diff`（内置比较）或 `testlib`（快照内自带 `checker.cpp`，判题节点用自己的工具链现编）。`config_json` 保存构建写入的逐测试点元数据（分组、分值、是否样例）。

## 题目包

出题侧分为工作材料、候选数据和发布快照，详见[出题设计](08-problem-authoring.md)：

- `problem_statements`(problem_id, language) 保存分段工作题面；`problem_workspaces.statement_language` 是工作副本默认语言，发布时显式选择的语言写入 `problem_versions` 与公共 `problems.statement_language`。
- `problem_files` 保存 checker / validator / generator / solution / interactor 源码。部分唯一索引 `ux_problem_files_active` 保证每题每类至多一个 `is_active`（checker/validator/interactor 的启用项，以及作为标程的解）。
- `problem_tests` 保存测试点计划：`manual` 存输入文本，`generator` 存一条生成命令。`test_index` 在 `(problem_id)` 内连续，删除后由 store 顺延。
- 包内容变更自增 `package_revision`，判题材料变更还自增 `data_revision`；候选的数据 revision 不匹配时必须重建或导入。工作元信息放在 `problem_workspaces`，公共读路径不读取可变源材料。
- `problem_build_jobs` 与 `judge_jobs` 同构（generation 换成 revision），复用 `FOR UPDATE SKIP LOCKED` + lease token 围栏。部分唯一索引 `ux_problem_build_jobs_active` 保证一道题同时只有一个未完成构建。
- `problem_build_jobs.input_json` 在排队时封存；完成只更新匹配当前数据 revision 的候选。显式发布在一个事务里创建不可变版本并切换公共投影；`judge_jobs` 的域、题目、generation 与发布版本不可修改。取消批量重测恢复 prior 版本和 prior 结果。
- 发布快照还保存全部源文件（包括未激活项）与候选样例，供复制所选版本使用。`problem_origins` 保存复制时的来源域、题号、版本、哈希与说明，只在目标包权限下读取；源引用是历史事实，不通过外键阻止源题删除。目标题目的数据另存于自己的目录，来源记录除账号删除的审计主体置空外不可修改。

## 通知

提交事务调用 `pg_notify('vertex_judge_jobs', submission_id)`，构建入队事务调用 `pg_notify('vertex_problem_builds', build_id)`。notification 只提示 Web dispatcher 唤醒有限 waiter，不承担持久化或投递保证；断线重连与 5 秒 fallback scan 保证最终可领取。

## 赛制与赛务

详见[赛制与榜单设计](05-contest-rankboard.md)。

- `contests.rule` 取 `icpc`/`ioi`/`oi`/`leduo`/`cf`，默认值为 `icpc`。赛制与 `full`/`summary`/`first_error`/`none` 反馈约束直接维护在 `000001_init`。OI 固定赛中不反馈；其他赛制的 `feedback` 可配置。
- `contest_problems` 增加 `label`(A/B/C)、`color`(气球色)与 `points`(IOI/OI 满分)。
- `contest_submission_cells` 一行同时保存裁判视图(`attempts`/`penalty_sec`/`score`/`solved_at`)与封榜视图(`public_*`)以及 `pending_count`,榜单读取因此与参赛人数无关地只需两条查询。
- `contests.owner_id` 保留当前所有权，`created_by` 保留创建记录。`contest_access` 以同域用户或 group 为主体，支持 editor/jury/observer/participant 多角色授权。`contest_staff` 是按当前有效成员计算的只读赛务视图，不再直接写入。
- `contests.admission` 区分域成员资格与显式 participant 授权；报名仍是独立的 `contest_participants` 记录，撤销资格后即使报名记录存在，也不能继续提交。
- `contests.allow_self_registration` 默认 true，`allow_late_registration` 默认 false；新增报名在比赛行锁内同时验证两项配置和结束时间。配置更新的缺省字段保留锁内当前值，不使用锁前快照覆盖并发修改，关闭报名不删除已有 participant 行。
- `rejudgings` + `rejudging_submissions` 记录批量重测;成员行保存展开时的 `generation` 与重测前判定,进度和「改判了哪些」都由 `submissions` 当前状态推导,worker 不上报任何批次状态。
- `clarifications` 是提问/回答/公告共用的话题表;选手可见性(自己的话题、发给自己的回复、全场公告)在 SQL 中过滤。

## 社区与后台

详见[社区与后台管理](09-community-admin.md)。

- `problem_sets` 保存域、公开编号、必填 `owner_id` 与不可变创建记录 `author_id`；`problem_set_access` 以用户或同域 group 授予 reader/editor，复合外键拒绝跨域关系。`problem_set_problems` 保存同域题目、顺序与备注，删除题单不删除题目。题单条目及进度按查看者的当前题目权限过滤，仅计算练习提交。
- `editorials` 增加 `solved_only`(防剧透)与冗余的 `vote_count`;`editorial_votes` 一人一票,计数每次由投票表重算,重复提交不会漂移。
- `discussion_posts` 只关联题目或题解，比赛交流使用 `clarifications`。域复合 FK 保证同域，`(problem_id,parent_id)` / `(editorial_id,parent_id)` 的自引用复合 FK 再保证同线程；编辑不移动作用域。`updated_at` 用于标记已编辑。
- `announcements` 是域内公告，支持置顶与草稿；`public_id` 按域分配并受身份保护触发器约束，旧接口对应官方域。公开读取不混入草稿，治理读写需要当前域的资源管理能力。
- 标签治理同步当前 `problem_tags` 和可变 `problem_workspaces.tags_json`，增加工作副本 metadata revision，但不改历史 `problem_versions.tags_json`。目录独占 guard 与题目写入的共享 guard 协调发布，所有治理写入在域锁内重新授权并记录审计。
- `users.disabled_at` / `disabled_reason` 用于封禁:不删账号,只阻止登录。封禁时同时吊销该用户的全部 `auth_sessions`,登录/刷新/access token 校验三条路径各自复查这一列。

## 计数和榜单

- `submissions.case_results` 服务详情读取，`submission_cases` 服务规范化查询，二者同事务更新。
- problem counters 从当前 submission 事实重算，并用事务 advisory lock 串行化同题更新。
- contest cell 由 `contest.ScoreCell` 从当前比赛提交事实整体重算(纯函数,可单测),rejudge 与改判都收敛到同一结果。
