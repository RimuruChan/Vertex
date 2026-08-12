# Vertex OJ — 数据库设计

PostgreSQL 16 是唯一事实源。数据库操作使用 `sqlx`；项目尚未实际部署，最终 schema 直接维护在 `server/migrations/000001_init.up.sql` 与对应 down 文件，不累积过渡 migration。

## 领域关系

```text
users ──< auth_sessions
  └──< submissions ──< submission_cases
          ├──< judge_jobs
          ├──> problems ──1 problem_testdata
          └──> contests ──< contest_submission_cells

problems ──< tags / problem_versions / editorials / discussion_posts
contests ──< contest_problems / contest_participants
```

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

文件内容不进入数据库。`problem_testdata` 只保存 `storage_path/data_version/sha256/case_count/checker` 等元信息；Web 将每次上传写入 `/<problemID>/<sha256>/` 内容寻址目录，Judge 只读。旧目录保留到题目删除，因此 claim 返回的路径、版本和哈希在运行期间构成不可变快照。

## 通知

提交事务调用 `pg_notify('vertex_judge_jobs', submission_id)`。notification 只提示 Web dispatcher 唤醒有限 waiter，不承担持久化或投递保证；断线重连与 5 秒 fallback scan 保证最终可领取。

## 计数和榜单

- `submissions.case_results` 服务详情读取，`submission_cases` 服务规范化查询，二者同事务更新。
- problem counters 从当前 submission 事实重算，并用事务 advisory lock 串行化同题更新。
- contest cell 从当前比赛提交事实重建，rejudge 不会重复计次。
