# Vertex OJ — 架构概览

本文描述当前仓库的实际架构。Vertex 采用 container-first 部署：Server 是业务和持久化边界，Worker 只通过内部 HTTP 协议领取任务、续租并回传结果。

## 组件

| 组件 | 技术 | 责任 |
|---|---|---|
| UI | React 19、TypeScript、Vite、Ant Design | 页面、内存 access token、refresh cookie 会话恢复 |
| Server | Go、Gin、sqlx | 业务规则、认证、Judge 调度协议、唯一数据库访问入口 |
| PostgreSQL | PostgreSQL 16 | 唯一事实源；session、业务数据、Judge job/lease |
| Worker | Go | 领取后台任务；当前负责判题编排且不持有数据库凭据 |
| Sandbox | C++、Landlock、seccomp、cgroup v2 | 隔离执行、资源限制与运行统计 |
| 测试数据 | Docker 共享卷 | Server 写入内容寻址版本，Worker 只读不可变快照 |

```text
Browser ── /api/* ──────────────────────> Server ── sqlx/LISTEN ──> PostgreSQL
Worker ── long poll / heartbeat / result ────┘
  │
  └── compile → vertex-sandbox → checker
```

不引入 Kafka、Redis 或第二份队列状态。`NOTIFY vertex_judge_jobs` 只负责唤醒，`judge_jobs` 表始终是权威状态；每个 Server 实例只有一个 5 秒 fallback ticker，每次只唤醒一个 waiter 检查数据库，不随等待中的 worker 数增长。

## Server 领域模块

Server 使用领域优先的模块化单体。每个领域根包直接包含实体、service、窄 repository interface 和默认 sqlx store；HTTP DTO 与 Gin handler 是该领域的子包：

```text
cmd/server
  └── internal/transport/httpapi   外层 Gin 组合、CORS、health、Swagger UI
internal/
  ├── identity/               用户、session、JWT/refresh、认证 handler
  ├── problem/                题目与测试数据
  ├── contest/                比赛、报名与榜单
  ├── submission/             提交、限流与 rejudge
  ├── judge/                  job、lease、fencing、NOTIFY 与内部 API
  ├── content/                题解与讨论
  ├── database/               sqlx pool 与 migration（不放领域 SQL）
  ├── middleware/             用户/admin/Judge service 认证
  ├── httpx/                  通用 HTTP 协议响应
  └── config/                 环境配置解析与验证
```

- `cmd/server` 是 composition root，负责加载配置并注入具体实现。
- 每个领域的 `handler/router.go` 只注册本领域路由，外层 router 依次调用这些注册函数。
- handler 只依赖本领域 service 与 DTO，不直接依赖 PostgreSQL；领域根包不反向依赖 handler/DTO。
- 领域实体不参与 HTTP 序列化；DTO 按业务类型组织，同一文件包含对应 request、response 与 mapper。
- PostgreSQL 访问统一使用 `sqlx`，领域 SQL 与完整事务边界留在触发用例所属领域的 store。

## 提交与判题生命周期

1. `POST /api/submissions` 在同一事务创建 `submissions` 与 generation 1 的 `judge_jobs`。
2. 事务提交时发送 PostgreSQL notification；每个 Server 实例只有一个专用 LISTEN connection。
3. Worker 对 `/internal/judge/v1/jobs/claim` 发起最长 25 秒长轮询。Server 使用 `FOR UPDATE SKIP LOCKED` 原子生成 worker、lease token 和到期时间。
4. Server 返回源码、资源限制和测试数据版本/哈希/checker 的不可变快照。
5. Worker 编译后逐测试点运行自研 C++ runner，并按 `leaseTTL/3` heartbeat。
6. result 使用 `job + generation + worker + lease token` fencing；同一事务写逐点结果、提交快照、题目计数和比赛积分格。
7. 同一 lease 的重复 result 幂等成功；迟到 lease 或旧 generation 永远返回冲突。
8. rejudge 取消旧 job、递增 generation 并创建新 job。

Worker 对 `204` 立即开启下一次长轮询；仅网络错误和 5xx 使用带 jitter 的指数退避。

## 认证生命周期

- Access JWT 默认 15 分钟，包含 `sub/sid/jti/type`，固定 HS256 并验证 issuer、audience、expiry、not-before 和算法。
- Refresh token 是 32 字节随机 opaque credential，只以 SHA-256 hash 存入 `auth_sessions`。
- refresh token 仅放在 `HttpOnly`、`SameSite=Lax` cookie；每次 refresh 原子轮换。
- 前端只在内存保存 access token，启动时通过 refresh cookie 恢复会话。
- 每次受保护请求都查询 session 与当前用户角色，因此 logout/logout-all 可立即吊销 access JWT。

## 代码生成

- Swag 从 handler annotation 生成 `server/docs/swagger.json|yaml`，开发环境在 `/swagger/index.html` 提供 UI。
- Orval 从该规范生成 `ui/src/generated/api`。
- 手写前端 HTTP 层只处理 credentials、Authorization、401 单飞刷新和会话状态，不重复维护 URL/DTO。
- CI 重新生成两端产物并以 `git diff --exit-code` 检查漂移。
