# Vertex OJ — 架构概览

本文描述当前仓库的实际架构。Vertex 采用 container-first 部署：Server 是业务和持久化边界，Worker 只通过内部 HTTP 协议领取任务、续租并回传结果。

## 组件

| 组件 | 技术 | 责任 |
|---|---|---|
| UI | React 19、TypeScript、Vite、Tailwind CSS、Radix UI | 页面、内存 access token、refresh cookie 会话恢复 |
| Server | Go、Gin、sqlc（迁移中） | 业务规则、认证、Judge 调度协议、唯一数据库访问入口 |
| PostgreSQL | PostgreSQL 16 | 唯一事实源；session、业务数据、Judge job/lease |
| Worker | Go | 领取判题与题目包构建任务，不持有数据库凭据 |
| Sandbox | C++、Landlock、seccomp、cgroup v2 | 隔离执行、资源限制与运行统计 |
| 测试数据 | Docker 共享卷 | Server 写入内容寻址版本，Worker 只读不可变快照 |

```text
Browser ── /api/* ──────────────────────> Server ── SQL/LISTEN ──> PostgreSQL
Worker ── long poll / heartbeat / result ────┘
  │
  └── compile → vertex-sandbox → checker
```

不引入 Kafka、Redis 或第二份队列状态。`NOTIFY vertex_judge_jobs` 只负责唤醒，`judge_jobs` 表始终是权威状态；每个 Server 实例只有一个 5 秒 fallback ticker，每次只唤醒一个 waiter 检查数据库，不随等待中的 worker 数增长。

## Server 领域模块

Server 使用按业务上下文分层的模块化单体。领域模型与 repository 契约、应用服务、PostgreSQL/sqlc 实现和 HTTP 适配分别归属 `domain`、`application`、`infrastructure/postgres` 与 `transport/http`。具体结构和依赖约束见[后端包结构与持久化边界](14-backend-architecture.md)。

```text
cmd/server
  └── internal/transport/http   外层 Gin 组合、CORS、health、Swagger UI
internal/
  ├── modules/                按业务上下文分层的模块
  │   ├── identity/           用户、session 与认证
  │   ├── tenancy/            域、成员、角色与群组
  │   ├── problem/            题目与公开读模型
  │   ├── authoring/          题目工作区、构建与发布
  │   ├── problemset/         题单与个人进度
  │   ├── contest/            比赛、赛务与榜单
  │   ├── submission/         提交、反馈与重测
  │   ├── judge/              评测任务与租约
  │   ├── content/            题解与讨论
  │   ├── profile/            用户公开统计
  │   ├── console/            站点及域内治理
  │   └── publicid/           公开编号解析
  ├── platform/               config、database、ratelimit
  └── transport/http/         全局路由、middleware、httpx
```

- `cmd/server` 是 composition root，负责加载配置并注入具体实现。
- 每个上下文的 HTTP 适配只注册自身路由，外层 router 依次调用这些注册函数。
- HTTP 适配依赖应用服务、领域类型与 DTO，不直接依赖 PostgreSQL；领域和应用层不反向依赖 HTTP 适配。
- 领域实体不参与 HTTP 序列化；DTO 按业务类型组织，同一文件包含对应 request、response 与 mapper。
- PostgreSQL 查询迁移到各上下文的私有 sqlc 包，完整事务边界由所属 repository 的基础设施实现维护。最终共享 database 包只保留连接池与 migration，不持有业务查询。

## 提交与判题生命周期

域目录位于 `/api/domains`，资源与治理接口位于 `/api/domains/{domain}`；旧无域资源接口只绑定官方域。题库、题单、比赛、提交、社区、统计及出题读写都按域与当前资源权限过滤。模型见[域与协作](10-domains-and-access.md)，验收与环境限制见[实施清单](plans/2026-09-07-domain-redesign.md)。

1. `POST /api/domains/{domain}/submissions` 在同一事务重新验证域、题目/比赛/参与关系并锁定目标，然后创建 `submissions` 与 generation 1 的 `judge_jobs`。
2. 事务提交时发送 PostgreSQL notification；每个 Server 实例只有一个专用 LISTEN connection。
3. Worker 对 `/internal/judge/v1/jobs/claim` 发起最长 25 秒长轮询。Server 使用 `FOR UPDATE SKIP LOCKED` 原子生成 worker、lease token 和到期时间。
4. Server 返回域、发布版本、源码、资源限制和测试数据版本/哈希/checker 的不可变快照。
5. Worker 编译后逐测试点运行自研 C++ runner，并按 `leaseTTL/3` heartbeat。
6. result 使用 `job + generation + worker + lease token` fencing；同一事务写逐点结果、提交快照、题目计数和比赛积分格。
7. 同一 lease 的重复 result 幂等成功；迟到 lease 或旧 generation 永远返回冲突。
8. rejudge 取消旧 job、递增 generation 并创建新 job；取消批次时，尚未领取的成员原子恢复重测前的完整结果快照。

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
- CI 重新生成两端产物并用 `git status --porcelain` 同时检查 tracked 与 untracked 漂移。
