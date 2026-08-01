# Vertex OJ — 比赛榜单设计

> ACM/ICPC 赛制是 MVP 的唯一赛制(IOI 留 v1)。本页说明积分模型、封榜语义与 rejudge 安全性。

## 1. ACM 积分模型

对每个用户 × 每道题维护一个积分格(`contest_submission_cells`):

| 字段 | 语义 |
|---|---|
| `attempts` | 已提交次数(仅未 AC 时递增) |
| `solved_at` | 首次 AC 时间(NULL = 未通过) |
| `penalty_sec` | 该题罚时(秒)= 解题耗时 + 未过尝试 × 20 分钟 |
| `pending_count` | 封榜期间提交数(榜单显示 `?`) |

**榜单汇总**:`solved` = 已 AC 题数,`penalty` = Σ 各题罚时。
**排序**:`solved` 降序 → `penalty` 升序 → 用户名升序(并列同 rank)。

## 2. 积分更新(幂等,rejudge 安全)

存储函数 `record_contest_submission()`(`migrations/000002`)由判题 worker 在比赛提交判定完成后调用:

```sql
IF p_accepted THEN
  INSERT ... VALUES(..., 1, solve_sec, submitted_at)
  ON CONFLICT DO UPDATE SET
    attempts = CASE WHEN solved_at IS NULL THEN attempts+1 ELSE attempts END,
    penalty_sec = CASE WHEN solved_at IS NULL THEN solve_sec + attempts*1200 ELSE penalty_sec END,
    solved_at = CASE WHEN solved_at IS NULL THEN EXCLUDED.solved_at ELSE solved_at END;
ELSE
  ... attempts 仅当未 AC 时 +1
```

**关键性质**:
- **只认首次 AC**:已 AC 后任何提交(包括 rejudge 变 AC/变 WA)都不改 `solved_at`/`penalty_sec`。
- **重复调用幂等**:判题 worker 崩溃重试、rejudge、同题多提交都不会污染积分。
- **时间窗**:仅比赛时间 `[begin_at, end_at]` 内的提交计入。

## 3. 封榜(Freeze)

- `contests.freeze_at` 为空 = 不封榜。
- `frozen` 判定:`now() > freeze_at`(或查询参数强制)。
- **封榜视图**:`freeze_at` 之后的 AC **隐藏为 pending**——`Rankboard()` 查询该用户该题在冻结时间后提交的 AC,把对应 cell 的 `solved_at` 置空、`pending_count+1`。前端渲染为 `?`。
- 解榜:重新计算(不冻结),真实数据恢复。

> 注意:MVP 的封榜把冻结后 AC 隐藏为 `?`,而非 Codeforces 式的"提交事件隐藏"。v1 可增强为完整事件级 freeze。

## 4. 为什么榜单是派生的

榜单**不存快照**,每次查询由 `contest_submission_cells` 实时计算。这带来:
- **rejudge 正确**:积分格幂等,重判后榜单自然一致。
- **无需缓存失效**:改判/封榜切换都是纯查询。
- **可回溯**:原始提交事件永久保留在 `submissions`,可随时重算历史榜单。

## 5. 前端渲染

榜单表格每列对应一道题(按 `problemIds` 顺序,标 A/B/C...),单元格三态:

| 状态 | 渲染 |
|---|---|
| `solvedAt` 有值 | 绿色标签,显示 `penaltySec/60` 分钟 |
| `attempts > 0` | 红色 `-attempts` |
| `pendingCount > 0` | 金色 `?`(封榜中) |
| 其他 | `·` |

实时性:MVP 前端 5s 轮询;v1 换 socket.io 增量推送。
