# Vertex OJ — 数据库设计

PostgreSQL 16 是唯一事实源。数据库操作使用 `sqlx`；项目尚未实际部署，完整 schema 直接维护在 `server/migrations/000001_init.up.sql` 与对应 down 文件，不累积过渡 migration。首次正式发布后再切换为只追加的升级策略。

> 开发期修改 `000001_init` 不会升级已经记录 migration version 的旧数据库。拉取包含 init schema rebase 的版本后，必须先备份需要的数据，再从仓库根目录执行 `docker compose down -v --remove-orphans` 并重新启动。该操作会永久删除 Compose 管理的数据库、测试数据与缓存卷。

## 领域关系

题目、比赛、提交、题解、题单和 group 保留 UUID 主键，公开编号使用 `public_id BIGINT` 与 `(domain_id, public_id)` 唯一约束。`domain_number_counters` 按域及资源种类计数，插入触发器在同一事务内分配编号；题目从 1000 开始，其他资源从 1 开始。因此两个域可以分别拥有自己的 1000 号题。并发插入通过 counter 行锁串行化；删除已提交资源后不复用编号。触发器拒绝修改公开身份及资源域。HTTP DTO 将编号序列化为字符串 `publicId`，避免 JavaScript 大整数精度问题。

外键、Worker 协议和请求体中的关联 ID 仍使用 UUID。HTTP 路由在认证及域解析之后，只在当前域把公开编号解析为 UUID，再执行资源权限检查；比赛题号由比赛领域在验证报名、时间和赛务身份后解析。编号可枚举，不作为访问控制手段。没有域前缀的旧资源接口明确绑定官方域，不会按 UUID 搜索其他域。

```text
users ──< auth_sessions
  └──< submissions ──< submission_cases
          ├──< judge_jobs
          ├──> problems ──1 problem_testdata
          └──> contests ──< contest_submission_cells

problems ──< tags / problem_versions / editorials / discussion_posts
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

Store 的资源查询、分页 count、更新及删除显式使用 typed domain context。旧的无 scope 持久层调用只访问官方域；不能用于跨域 Worker 查询。全站账号治理及系统队列统计是明确例外，只由站点管理员接口提供。完整资源 owner/协作者策略和多域 UI 尚未完成，不能把数据库作用域底座当作完整多租户产品。

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

文件内容不进入数据库。`problem_testdata` 只保存 `storage_path/data_version/sha256/case_count/checker` 等元信息；Web 将每次上传或构建产物写入 `/<problemID>/<sha256>/` 内容寻址目录，Judge 只读。旧目录保留到题目删除，因此 claim 返回的路径、版本和哈希在运行期间构成不可变快照。

`checker` 取 `diff`（内置比较）或 `testlib`（快照内自带 `checker.cpp`，判题节点用自己的工具链现编）。`config_json` 保存构建写入的逐测试点元数据（分组、分值、是否样例）。

## 题目包

出题侧的源材料与已发布数据分成两层，详见[出题设计](08-problem-authoring.md)：

- `problem_statements`(problem_id, language) 保存分段题面；`problems.statement_language` 指定渲染进 `statement_md` 的那一份。
- `problem_files` 保存 checker / validator / generator / solution / interactor 源码。部分唯一索引 `ux_problem_files_active` 保证每题每类至多一个 `is_active`（checker/validator/interactor 的启用项，以及作为标程的解）。
- `problem_tests` 保存测试点计划：`manual` 存输入文本，`generator` 存一条生成命令。`test_index` 在 `(problem_id)` 内连续，删除后由 store 顺延。
- 任何包内容变更都在同一事务里自增 `problems.package_revision`；`built_revision` 记录最后一次成功构建的版本，二者不等即数据过期。
- `problem_build_jobs` 与 `judge_jobs` 同构（generation 换成 revision），复用 `FOR UPDATE SKIP LOCKED` + lease token 围栏。部分唯一索引 `ux_problem_build_jobs_active` 保证一道题同时只有一个未完成构建。
- 构建产物的发布只发生在 `Complete(success=true)` 的事务里：写 `problem_testdata`、`problems.built_revision` 和重渲染的 `statement_md`，读者不会看到数据与题面不一致的中间态。

## 通知

提交事务调用 `pg_notify('vertex_judge_jobs', submission_id)`，构建入队事务调用 `pg_notify('vertex_problem_builds', build_id)`。notification 只提示 Web dispatcher 唤醒有限 waiter，不承担持久化或投递保证；断线重连与 5 秒 fallback scan 保证最终可领取。

## 赛制与赛务

详见[赛制与榜单设计](05-contest-rankboard.md)。

- `contests.rule` 取 `icpc`/`ioi`/`oi`,历史值 `acm` 在读路径归一化为 `icpc`。`penalty_minutes`、`penalize_compile_error`、`feedback`、`unfreeze_at` 都是每场可配的赛务设置。
- `contest_problems` 增加 `label`(A/B/C)、`color`(气球色)与 `points`(IOI/OI 满分)。
- `contest_submission_cells` 一行同时保存裁判视图(`attempts`/`penalty_sec`/`score`/`solved_at`)与封榜视图(`public_*`)以及 `pending_count`,榜单读取因此与参赛人数无关地只需两条查询。
- `contest_staff` 把 jury/observer 权限下放给具体用户,不必授予系统管理员。
- `rejudgings` + `rejudging_submissions` 记录批量重测;成员行保存展开时的 `generation` 与重测前判定,进度和「改判了哪些」都由 `submissions` 当前状态推导,worker 不上报任何批次状态。
- `clarifications` 是提问/回答/公告共用的话题表;选手可见性(自己的话题、发给自己的回复、全场公告)在 SQL 中过滤。

## 社区与后台

详见[社区与后台管理](09-community-admin.md)。

- `problem_sets` / `problem_set_problems` 从第一版就存在但一直没有实现,本轮补上策展元信息(`updated_at`、每题 `note`)并接上读写路径。题单进度是读模型,由 `submissions` 现算。
- `editorials` 增加 `solved_only`(防剧透)与冗余的 `vote_count`;`editorial_votes` 一人一票,计数每次由投票表重算,重复提交不会漂移。
- `discussion_posts` 增加 `updated_at`,与 `created_at` 拉开距离即表示「已编辑」。
- `announcements` 是域内公告，支持置顶与草稿；旧接口对应官方域。
- `users.disabled_at` / `disabled_reason` 用于封禁:不删账号,只阻止登录。封禁时同时吊销该用户的全部 `auth_sessions`,登录/刷新/access token 校验三条路径各自复查这一列。

## 计数和榜单

- `submissions.case_results` 服务详情读取，`submission_cases` 服务规范化查询，二者同事务更新。
- problem counters 从当前 submission 事实重算，并用事务 advisory lock 串行化同题更新。
- contest cell 由 `contest.ScoreCell` 从当前比赛提交事实整体重算(纯函数,可单测),rejudge 与改判都收敛到同一结果。
