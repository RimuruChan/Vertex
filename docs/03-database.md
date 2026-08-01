# Vertex OJ — 数据库设计

> PostgreSQL 16。迁移位于 `web/migrations/`(golang-migrate,版本化 SQL)。

## 1. Schema 总览

```
users ──< submissions ──> problems ──< tags (problem_tags)
  │            │              │
  │            └──< contest_id │
  │                            │
contests ──< contest_problems ─┘
   │  │
   │  └──< contest_submission_cells
   │
contest_participants
problems ──1:1── problem_testdata ──< problem_versions
problems ──< editorials ──< discussion_posts
problem_sets ──< problem_set_problems
```

## 2. 核心表

### users
`id`(UUID PK)、`username` UNIQUE、`email` UNIQUE、`password_hash`(bcrypt)、`role`(user/admin)、`rating`(占位)。

### problems
`id`、`title`、`statement_md`、`difficulty`(1-10)、`source`、`time_limit_ms`、`memory_limit_kb`、`visibility`(draft/private/public)、`author_id`、`submission_count`/`accepted_count`/`solved_user_count`(计数器)、`judge_type`(normal/interactive,SPJ 预留)。

### problem_testdata
**测试数据文件永不进 DB**。此表只存元信息:`data_version`、`storage_path`、`sha256`(数据目录哈希)、`case_count`、`checker`(diff/spj/interactive)、`config_json`(每测试点限覆盖/batched 依赖声明)。文件实体在共享卷 `TESTDATA_ROOT/<problemID>/1.in, 1.out, ...`。

### submissions(**状态列兼作判题队列**)
`id`、`user_id`、`problem_id`、`language`、`source_code`、`status`(CHECK 约束 11 态)、`score`、`total_time_ms`、`peak_memory_kb`、`compile_result`、`case_results`(JSONB 快照)、**`contest_id`**(NULL=练习)、`submitted_at`、`judged_at`。

**队列机制**:worker 用
`UPDATE submissions SET status='Judging' WHERE id=(SELECT id FROM submissions WHERE status='Pending' ORDER BY submitted_at FOR UPDATE SKIP LOCKED LIMIT 1)`
原子领取。行即队列,无外部 broker;rejudge 只需重置回 Pending。

### submission_cases
`submission_id`、`case_index`、`verdict`、`time_ms`、`memory_kb`、`exit_status`、`checker_output`,UNIQUE(submission_id, case_index)。判题结束全量重写(幂等)。

### contests
`id`、`title`、`description`、`rule`(acm/ioi)、`begin_at`/`end_at`/`freeze_at`(封榜)、`visibility`(public/private/password)、`password_hash`、`rankboard_visible`、`created_by`。

### contest_submission_cells(DOMjudge scorecache 式)
`(contest_id, user_id, problem_id)` 联合主键,`attempts`、`penalty_sec`、`solved_at`、`score`(IOI)、`pending_count`(封榜期)。**增量更新,rejudge 安全**。

### editorials / discussion_posts
题解与评论。`discussion_posts` 通过 `problem_id`/`editorial_id`/`contest_id` 三选一关联(CHECK 约束 `num_nonnulls=1`),`parent_id` 支持线程回复。

### problem_sets / problem_set_problems
题单(表已建模,v1 启用)。

## 3. 关键索引

| 表 | 索引 | 用途 |
|---|---|---|
| submissions | (user_id, submitted_at DESC) | 用户提交历史 |
| submissions | (problem_id, status) | 题目维度筛选 |
| submissions | (status) | 队列领取 |
| submissions | (contest_id) | 比赛榜单查询 |
| submission_cases | (submission_id) | 逐测试点读取 |
| problems | (visibility) | 公开列表 |
| problems | (difficulty) | 难度筛选 |
| contest_submission_cells | (contest_id, user_id, problem_id) PK | 榜单计算 |
| discussion_posts | (problem_id) / (editorial_id) | 评论列表 |

## 4. 设计决策

1. **测试数据文件外置**:DB 只存路径与哈希,避免大对象拖垮备份与查询。
2. **状态列 = 队列**:`SKIP LOCKED` 原子领取,崩溃安全(事务回滚,Pending 不变)。
3. **`contest_id` 行内字段**:比赛绑定、榜单查询、队列优先级都从一个字段出发。
4. **JSONB 快照 + 规范化并行**:`submissions.case_results` 是前端读取的快照,`submission_cases` 是规范化事实源;两者同事务写入。
5. **积分格幂等**:只认首次 AC;rejudge 重复调用不污染榜单。
6. **计数精确**:`submission_count` 仅从非终态→终态递增一次,`accepted_count` 仅非 AC→AC 递增一次。
