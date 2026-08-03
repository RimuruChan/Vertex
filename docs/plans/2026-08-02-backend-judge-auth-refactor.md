# Vertex 后端、鉴权与 Judge 通信重构计划

## 1. Goal objective

在不改变现有 Judge 沙箱安全架构和公开业务能力的前提下，完成以下目标：

1. Judge 不再直连 PostgreSQL，改为通过受认证的 Web 内部 API 长轮询领取任务、续租和回传结果。
2. 将 `submissions` 的用户状态与判题调度状态拆开，引入显式 job、generation、lease 和 fencing 语义。
3. 将 Web 后端重组为领域优先的模块化单体：Identity、Problem、Contest、Submission、Judge、Content 作为 `internal` 下的一级目录，领域实体、service 和 sqlx store 直接放在领域根包，领域内仅为 HTTP DTO 与 handler 建立子包；HTTP handler 不再直接操作数据库 store，也不再直接暴露数据库实体。
4. 引入短期 Access JWT、可轮换 Refresh Token、会话吊销和退出登录；吊销后 Access JWT 立即失效。
5. 新增或重构的后端单元测试使用 Ginkgo v2 + Gomega，并继续允许 `go test ./...` 统一执行。
6. 使用稳定版 `swaggo/swag` 从 Go 注解生成 Swagger/OpenAPI 2.0 契约，并用 Orval 从该契约生成前端类型和 API 客户端。
7. 保持 Docker 默认 seccomp/AppArmor、只读 rootfs、现有 capability 白名单、自研 C++ runner 和 Judge sandbox policy 不变。

本 Goal 应实现、测试、更新文档并完成一次最终 diff 审计。不要推送远端；提交前确认用户是否仍要求签名提交。

## 2. 已确认的现状

- Web 与 Judge 目前都持有 `DATABASE_URL`，Judge 每个执行循环每 500ms 使用 `SKIP LOCKED` 直接领取 `submissions`。
- `submissions.status` 同时承担用户状态、队列状态和粗粒度租约，Judge 结果可直接覆盖 submission。
- Web 已预留 `notify` 回调但路由传入 `nil`；Redis 当前不参与判题通信。
- Web 与 Judge 通过 `/testdata` Docker volume 共享测试数据，Judge 只读。
- JWT 当前有效期为 7 天，没有 `jti`、session、吊销和 refresh token。
- Gin handler 直接依赖 `store`，请求/响应 DTO 与数据库 model 混用；`Router` 内部自行构造所有依赖。
- 前端手写 Axios API 和接口类型，并把 JWT 存在 `localStorage`。
- `swaggo/swag` 稳定版生成的是 Swagger/OpenAPI 2.0，不要在文档中误称为 OpenAPI 3；Orval 可直接消费 Swagger 2.0。

## 3. 范围约束

### 3.1 本次必须完成

- Auth、Submission/Judge 两条核心链路完成领域模块化和测试。
- Problem、Contest、Editorial、Discussion 的 handler 只依赖对应领域 service，并为公开 API 使用显式 DTO。
- Judge HTTP 长轮询协议、租约、heartbeat、幂等结果回写和 stale result fencing。
- Access/Refresh token、刷新轮换、单会话退出、会话吊销。
- Swag 文档、Swagger UI、Orval 生成客户端和 CI 漂移检查。
- 前端认证状态重构和现有页面迁移到生成客户端。
- 将最终数据库结构直接收敛到 `000001_init`、更新文档、单元测试和 E2E。

### 3.2 明确不做

- Kafka、Redis Streams 或其他外部任务队列。
- 对象存储迁移；本次继续使用只读 testdata volume，但 job payload 必须包含测试数据版本/哈希。
- WebSocket/SSE 判题结果推送；前端结果轮询暂时保留。
- 替换 Gin、sqlx、自研 C++ runner 或现有 sandbox。
- 新增数据库 verdict 或修改判定分类学。
- 为了“统一风格”重写已经稳定且与目标无关的 SQL。
- 创建泛型 BaseRepository、全局 Service Locator 或反射式依赖注入框架。

## 4. 目标包结构

目标结构采用领域优先的扁平模块。每个领域根包直接容纳实体、service、repository interface、sqlx store、错误和领域内辅助逻辑；只有 DTO 和 Gin handler 因依赖方向与协议职责不同而成为领域子包。这里不追求将 domain/application/adapter 拆成大量小 package，但领域实体仍不得携带 JSON 或数据库序列化职责：

```text
web/
  cmd/server/                    # composition root，只负责配置与组装
  docs/                          # swag 生成物：docs.go/swagger.json/swagger.yaml
  internal/
    config/                      # 环境变量解析、默认值和验证
    database/                    # sqlx 连接、migration；不放领域 SQL
    httpx/                       # 通用 HTTP 错误/状态响应和协议辅助
    middleware/                  # auth、admin、judge service auth、CORS
    identity/
      domain.go                  # User、Session、role 和领域错误
      service.go                 # 注册、登录、刷新、吊销、当前用户
      store.go                   # sqlx user/session store 与事务
      security.go                # bcrypt、JWT、refresh token 生成/哈希
      dto/
        auth.go                  # 同一业务类型的 request/response/mapper
        user.go
        session.go
      handler/
        auth.go                  # Gin handler + swag annotations
        router.go                # 注册 Identity 路由
    problem/
      domain.go
      service.go
      store.go
      testdata.go
      dto/
        problem.go
        testdata.go
      handler/
        problem.go
        admin.go
        router.go
    contest/
      domain.go
      service.go
      store.go
      rankboard.go
      dto/
        contest.go
        registration.go
        rankboard.go
      handler/
        contest.go
        router.go
    submission/
      domain.go                  # Submission、CaseResult、公开状态
      service.go                 # 提交、查询、重判
      store.go
      limiter.go
      dto/
        submission.go
        case_result.go
      handler/
        submission.go
        router.go
    judge/
      domain.go                  # Job、Lease、JobSpec、Result
      service.go                 # claim、heartbeat、finish
      store.go                   # job/lease/fencing 和结果事务
      dispatcher.go
      notify.go
      dto/
        job.go
        lease.go
        result.go
      handler/
        judge.go
        router.go
    content/
      domain.go                  # Editorial、Discussion
      service.go
      store.go
      dto/
        editorial.go
        discussion.go
      handler/
        editorial.go
        discussion.go
        router.go
```

Judge 侧目标结构：

```text
judge/
  cmd/worker/                    # composition root
  internal/
    config/
    client/                     # Web Judge API client 和 wire DTO
    scheduler/                  # 领取、heartbeat、编译、执行、结果回传
    compile/
    executor/
    run/
    verdict/
```

迁移规则：

- 领域根包使用领域名作为 package；`domain.go`、`service.go`、`store.go` 等文件都直接位于领域目录，不再建立 `domain/application/adapter` 子包。
- `service.go` 定义它真正需要的窄 repository interface；同一领域根包中的 `store.go` 提供默认 sqlx 实现，service 依赖接口而不是具体 `*SQLStore`。
- 每个领域拥有自己的 `dto` 和 `handler` 子包；DTO 子包可以依赖领域根包，handler 可以依赖本领域 service 和 DTO，领域根包不得反向依赖 DTO 或 handler。
- DTO 文件按 `problem`、`user`、`job`、`rankboard` 等业务类型组织，不按 `request.go`/`response.go` 横切；同一业务类型的 request、response 和 mapper 放在同一文件。
- 每个领域的 `handler/router.go` 暴露路由注册函数；外层 HTTP 组合代码显式调用各领域 handler 注册路由，router 不得在内部创建 store 或 service。
- middleware 统一放在 `internal/middleware`，构造完成后注入路由注册；领域根包不依赖 middleware。
- 领域实体不包含 `json` tag；HTTP 输出只使用所属领域的 DTO。通用错误响应等纯协议类型放在 `internal/httpx`，不得建立全局业务 DTO 包。
- 跨表、跨领域原子操作由触发该用例的领域 store 完整实现。例如 Judge 结果回写事务归 `judge/store.go`，不得在 service 中串联多个具体 store 伪装成事务。
- `cmd/server` 显式构造 config、pool、SQL stores、services、middleware、handlers 并调用各领域路由注册。
- 不创建泛型 BaseRepository、全局 Service Locator、反射式依赖注入或仅为目录对称而存在的 package/interface。

## 5. 数据库设计

项目尚未实际部署，本次不追加迁移版本。将 sessions、judge jobs 和 generation 直接并入 `000001_init.up.sql`，并同步维护 `000001_init.down.sql`；删除已被最终结构取代的增量 migration。所有 Web 数据库访问统一通过 `sqlx`，领域 SQL 和事务只放在对应领域根包的 store 文件中，不在 handler、DTO 或 service 中直接执行 SQL。

### 5.1 `auth_sessions`

```text
auth_sessions
  id                  UUID PK
  user_id             UUID FK users(id) ON DELETE CASCADE
  refresh_token_hash  BYTEA UNIQUE NOT NULL
  expires_at          TIMESTAMPTZ NOT NULL
  revoked_at          TIMESTAMPTZ NULL
  last_used_at        TIMESTAMPTZ NULL
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
```

索引：

- `(user_id) WHERE revoked_at IS NULL`
- `(expires_at)` 用于后续清理
- refresh hash 使用唯一约束保证并发轮换只有一个请求成功

数据库只存 refresh token 的 SHA-256，不存明文。

### 5.2 `judge_jobs`

```text
judge_jobs
  id                UUID PK
  submission_id     UUID FK submissions(id) ON DELETE CASCADE
  generation        INTEGER NOT NULL
  state             TEXT queued/running/completed/cancelled/dead
  priority          INTEGER NOT NULL DEFAULT 0
  attempt           INTEGER NOT NULL DEFAULT 0
  available_at      TIMESTAMPTZ NOT NULL DEFAULT now()
  worker_id         TEXT NULL
  lease_token       UUID NULL
  lease_expires_at  TIMESTAMPTZ NULL
  last_error        TEXT NOT NULL DEFAULT ''
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
  started_at        TIMESTAMPTZ NULL
  finished_at       TIMESTAMPTZ NULL
  UNIQUE(submission_id, generation)
```

同时在 `submissions` 增加：

```text
judge_generation INTEGER NOT NULL DEFAULT 1
```

索引至少覆盖：

```text
(state, priority DESC, available_at, created_at)
(lease_expires_at) WHERE state = 'running'
```

初始化时 submission 使用 generation=1；新建 submission 必须在同一事务中创建对应 queued job。最终 init schema 不再包含旧 `judge_started_at` 字段。

## 6. Auth 设计

### 6.1 Token 模型

- Access JWT 默认 TTL：15 分钟，可由 `AUTH_ACCESS_TTL` 配置。
- Refresh session 默认 TTL：30 天，可由 `AUTH_REFRESH_TTL` 配置。
- JWT 固定 HS256，验证 issuer、audience、expiry、not-before 和允许算法。
- Claims 至少包含：`sub`、`sid`、`jti`、`username`、`role`、`type=access`。
- `DATABASE_URL`、`JWT_SECRET` 与 `JUDGE_API_TOKEN` 在所有环境都必须显式提供；两个 token 均满足至少 32 个字符，缺失时进程失败关闭。
- Refresh token 使用 32 字节密码学随机数并进行 base64url 编码，不使用可长期离线验证的 JWT refresh token。

### 6.2 Cookie 与前端存储

- Refresh token 只放 `HttpOnly`、`SameSite=Lax` cookie；生产启用 `Secure`。
- Access token 只保存在前端内存，不再写 `localStorage`。
- 页面刷新后调用 `/api/auth/refresh` 恢复会话。
- CORS 改为 allowlist，不再无条件回显任意 Origin；所有认证请求使用 credentials。

### 6.3 API

保留现有路径并新增：

```text
POST /api/auth/register
POST /api/auth/login
POST /api/auth/refresh
POST /api/auth/logout
POST /api/auth/logout-all
GET  /api/auth/me
```

认证响应 DTO：

```json
{
  "accessToken": "...",
  "token": "...",
  "expiresIn": 900,
  "user": {"id":"...","username":"...","email":"...","role":"user"}
}
```

- `token` 暂时作为兼容别名保留并在 Swagger 描述中标记 deprecated；前端和 E2E 迁移到 `accessToken`。
- refresh 每次成功后轮换 refresh token；旧 token 的并发重放只能有一个成功。
- logout 吊销当前 `sid` 并清除 cookie。
- logout-all 吊销该用户所有活动 session。
- RequireAuth 解析 JWT 后必须检查 session 未吊销且未过期；使用数据库作为权威状态，确保立即吊销。
- middleware 使用数据库中当前 user role，不盲信长期存在于 JWT 中的旧 role。

### 6.4 错误 DTO

统一公开错误格式，同时保留现有 `error` 字符串兼容：

```json
{"code":"auth.invalid_credentials","error":"invalid username or password"}
```

应用层返回 typed/sentinel errors，HTTP 层统一映射状态码；禁止 handler 用字符串匹配数据库错误。

## 7. Judge 长轮询协议

### 7.1 配置

Web：

```text
JUDGE_API_TOKEN
JUDGE_LONG_POLL_TIMEOUT=25s
JUDGE_LEASE_TTL=45s
```

Judge：

```text
JUDGE_API_URL=http://web:8080/internal/judge/v1
JUDGE_API_TOKEN
JUDGE_WORKER_ID                 # 默认 hostname + process identity
JUDGE_HTTP_TIMEOUT              # 必须大于 long poll timeout
```

Judge 容器删除 `DATABASE_URL`；`judge/go.mod` 删除 pgx 依赖。

### 7.2 内部 API

所有接口使用独立 Judge service token，使用常量时间比较，不复用用户 JWT：

```text
POST /internal/judge/v1/jobs/claim
POST /internal/judge/v1/jobs/{jobId}/heartbeat
PUT  /internal/judge/v1/jobs/{jobId}/result
```

Claim request：

```json
{
  "workerId": "judge-host-1/worker-0",
  "waitSeconds": 25,
  "capabilities": ["c", "cpp", "python"]
}
```

响应：

- `200`：返回 job。
- `204`：长轮询到期且无任务。
- `401/403`：service token 错误。
- `503`：后端暂时不可领取任务。

Job response 必须是不可变快照：

```json
{
  "jobId": "...",
  "submissionId": "...",
  "generation": 2,
  "attempt": 1,
  "leaseToken": "...",
  "leaseExpiresAt": "...",
  "language": "cpp",
  "sourceCode": "...",
  "problemId": "...",
  "contestId": null,
  "timeLimitMs": 1000,
  "memoryLimitKb": 262144,
  "testdata": {
    "storagePath": "...",
    "dataVersion": 3,
    "sha256": "...",
    "caseCount": 10,
    "checker": "diff"
  }
}
```

Heartbeat request 包含 `generation` 和 `leaseToken`。Result request 包含：

- `generation`
- `leaseToken`
- 最终 verdict/score/time/memory/compile result
- 全部 case result DTO

### 7.3 Claim 与长轮询实现

1. Handler 先调用 Judge 领域 service 原子 claim。
2. 无任务时在 dispatcher 上等待；不要让每个 HTTP 请求各自每 500ms 查询数据库。
3. Submission/rejudge 事务提交后发出 wake signal。
4. 增加 PostgreSQL `NOTIFY vertex_judge_jobs`，每个 Web 实例只维护一个 LISTEN connection；通知仅作唤醒提示。
5. LISTEN 断线或漏通知时，以 5 秒低频扫描兜底。
6. 一个 Web 实例收到通知时只唤醒有限 waiter，避免广播惊群；最终正确性仍由 `SKIP LOCKED` 保证。
7. Judge 在 `204` 后可以立即发起下一次长轮询；仅在网络/5xx 错误时指数退避并加入 jitter。

### 7.4 Lease、heartbeat 与 fencing

- claim 使用单条事务性 `UPDATE ... FOR UPDATE SKIP LOCKED` 将 job 改为 running，并生成新的 lease token。
- Judge scheduler 在执行任务期间按 `leaseTTL/3` heartbeat。
- heartbeat 只在 `job_id + generation + lease_token + state=running` 全部匹配时续租。
- result 在同一事务中：
  1. 锁定并验证 job lease。
  2. 幂等写 submission 和 submission cases。
  3. 重算 problem counters 和 contest cell。
  4. 将 job 标记 completed。
- 条件不匹配返回 `409 stale_lease`，Judge 必须丢弃迟到结果。
- “结果已由同一 generation 完成”返回幂等成功，Judge 不重复报错。
- worker 崩溃后 lease 到期，job 回到可领取状态并增加 attempt。
- 超过最大 attempt 后标记 dead，并将 submission 写为现有 `System Error`，不新增 verdict。
- rejudge 取消旧 queued/running job、递增 submission generation、创建新 queued job；旧 generation 的结果永远无法覆盖新结果。

## 8. 后端领域模块重构细则

### 8.1 Router 与 composition root

- `cmd/server` 加载并验证完整 config。
- `cmd/server` 构造 sqlx stores、security provider、领域 services、middleware 和 handlers。
- 每个领域的 `handler/router.go` 只注册本领域路由；外层 HTTP 组合代码依次调用这些注册函数。
- 路由注册接受显式 service/handler 和已经构造完成的 middleware，不得在内部 `NewSQLStore`、`NewService` 或读取环境变量。
- 将 server timeout 与 Judge long poll 协调：`WriteTimeout` 必须大于最大 claim wait。
- `/api/health` 区分 liveness/readiness；readiness 至少检查数据库和 migration 已完成。

### 8.2 DTO 与 mapper

- 每个公开接口有命名 request/response DTO，不在 handler 内用匿名 `gin.H` 表达成功响应。
- DTO 位于所属领域的 `dto` 子包，按业务类型分文件；例如 `problem/dto/problem.go` 同时包含 Problem request、response 和 mapper，不建立全局业务 DTO 包，也不拆成通用 `request.go`/`response.go`。
- 列表使用 `internal/httpx` 提供的统一 `ListResponse[T]`，业务字段仍由所属领域 DTO 定义。
- 领域实体不直接序列化。
- 管理端 DTO 与公开 DTO 分开，避免误暴露 source/password/internal path。
- `sourceCode` 是否返回由领域 service 的授权逻辑决定，不在序列化前临时清空数据库 entity。
- 输入校验放在 DTO validation + service/domain invariant；数据库约束作为最后防线。

### 8.3 领域边界

- Identity：用户、密码、session、role。
- Problem：题目、标签、测试数据元信息和管理工作流。
- Contest：比赛、报名、题集、榜单。
- Submission：用户提交、查询、rejudge 请求。
- Judge：调度 job、lease、运行结果。
- Content：题解和讨论。

Submission 与 Judge 可以共享 submission ID，但 `judge` 根包不得依赖 Submission/Judge 的 Gin DTO；跨领域协作通过明确的领域输入类型和窄接口完成。Judge 结果回写等跨领域事务由用例所属领域的 store 原子实现。

### 8.4 注释和命名

- 注释解释责任、约束、并发或安全原因，不重复函数名和 HTTP method。
- 删除“v1 再做”“MVP 后扩展”等已经失真的路线图注释；路线图只放 `docs/07-roadmap.md`。
- 导出符号满足 Go doc 规范；内部简单 getter 不强行加注释。
- 中文注释统一 UTF-8 和中文标点；协议字段、错误 code、日志 key 使用英文。
- 日志始终包含稳定的 `submission_id`、`job_id`、`generation`、`worker_id`，绝不记录源码、JWT、refresh token 或 service token。

## 9. Ginkgo v2 + Gomega 测试计划

### 9.1 依赖与执行

- 在 `web/go.mod` 固定 `github.com/onsi/ginkgo/v2` 和 `github.com/onsi/gomega` 版本。
- 每个新增测试 package 放一个 `*_suite_test.go`，通过 `RunSpecs` 注册。
- CI 继续执行 `go -C web test ./...`；Ginkgo CLI 仅作为本地增强，不作为唯一执行方式。
- 不为已有稳定 stdlib test 做纯风格重写；本次触及的测试迁移到 Ginkgo/Gomega。
- fake repository 手写最小实现，不引入通用 mocking 框架。

### 9.2 Auth specs

- HS256 之外算法被拒绝。
- issuer/audience/type/sid/jti/expiry 校验。
- 注册和登录创建 session。
- refresh 成功轮换 token。
- 同一旧 refresh token 并发刷新仅一个成功。
- logout 后原 Access JWT 立即被拒绝。
- logout-all 吊销全部 session。
- 过期、吊销、hash 不匹配返回统一 unauthorized。
- cookie 属性在开发/生产配置下正确。
- handler DTO 校验和错误映射。

### 9.3 Judge specs

- 多 worker 并发 claim 不领取同一 job。
- 长轮询在提交后被唤醒。
- 无任务按超时返回 204，不忙等。
- heartbeat 续租。
- 错 token/generation heartbeat 返回 stale lease。
- worker 崩溃后过期 job 可被重新领取，attempt 增加。
- 旧 worker 的迟到 result 被拒绝。
- 同一 result 重试幂等。
- rejudge 后旧 generation 不能覆盖新 generation。
- result 事务失败时 submission/job/cases/counters 全部回滚。
- Judge HTTP client 正确处理 200/204/401/409/5xx、context cancellation 和退避。

### 9.4 API 与契约 specs

- 使用 `httptest` 测试公开 Auth 和内部 Judge API。
- Swagger JSON 可解析且包含所有现有公开路由、认证路由和内部 Judge 路由。
- 安全定义包含 user bearer auth 与独立 judge service auth。
- DTO 中不得出现 `password_hash`、refresh hash、lease token（公开 API）等敏感字段。

### 9.5 E2E 增量

现有 E2E 保留并增加：

- access token 刷新后继续请求。
- logout 后旧 access token 返回 401。
- rejudge generation fencing。
- 两个 Judge worker 并发领取和完成。
- Judge 容器环境中不存在 `DATABASE_URL`。
- Web 不可用时 Judge 安全退避；恢复后继续领取。

## 10. Swag 与前端 OpenAPI 生成

### 10.1 后端生成

- 使用稳定版 `swaggo/swag`，通过 Go tool dependency 固定版本，禁止 CI 使用浮动 `@latest`。
- 在 server API 入口添加全局注解：title、version、base path、security definitions。
- 每个 handler 添加 `@Summary`、`@Tags`、`@Accept`、`@Produce`、`@Param`、`@Success`、`@Failure`、`@Router`、`@Security`。
- 生成：

```text
web/docs/docs.go
web/docs/swagger.json
web/docs/swagger.yaml
```

- 暴露 Swagger UI，例如 `/swagger/index.html`；生产是否启用由配置控制。
- 增加仓库命令（脚本或 Make target）统一执行 `swag fmt` 与 `swag init --parseInternal`。
- CI 重新生成后执行 `git diff --exit-code -- web/docs`，防止注解和契约漂移。

### 10.2 Orval

- 前端包管理器统一使用 pnpm；在 `webui/package.json` 的 `packageManager` 字段固定 pnpm 版本，以 `webui/pnpm-lock.yaml` 作为唯一依赖锁文件并删除 `webui/package-lock.json`。
- 本地文档、CI 和仓库脚本统一通过 pnpm 安装依赖和运行命令；不得混用 npm 生成或更新第二份锁文件。
- 在 `webui` 固定 Orval 版本；Orval 支持 Swagger 2.0 和 OpenAPI 3。
- 新增 `orval.config.ts`，输入 `../web/docs/swagger.json`。
- 输出目录：`webui/src/generated/api/`。
- 使用 Axios client 和自定义 mutator，使生成请求复用统一的 base URL、credentials、access token 和 refresh interceptor。
- 生成的 model 和 request function 是 API 类型的唯一事实源；删除 `webui/src/api/types.ts` 中与后端契约重复的类型。
- `webui/src/api/client.ts` 仅保留：
  - Axios 实例/Orval mutator。
  - Access token 内存状态。
  - 单飞 refresh（多个 401 只发一个 refresh 请求）。
  - logout 和兼容性小包装；不得继续手写 URL、请求 body 或 response interface。
- 新增或调整 pnpm scripts：

```text
api:generate
api:check
build
```

- CI 使用固定版本 pnpm 和 pnpm store cache，以 `pnpm install --frozen-lockfile` 安装依赖；在前端 build 前执行生成，并对 `webui/src/generated/api` 做 git diff 检查。

### 10.3 前端认证状态

- 新增轻量 `AuthProvider`/hook，不引入额外全局状态库。
- 应用启动先通过 refresh cookie 恢复 session，再渲染需要认证的路由。
- 401 interceptor 只对非 refresh 请求尝试一次刷新，防止递归和请求风暴。
- refresh 失败统一清空内存用户并跳转登录。
- logout 必须等待后端吊销成功或明确处理网络失败，然后清理本地状态。
- 删除 `vertex_token` localStorage；可在迁移代码中主动清除旧 key。

## 11. 配置与部署调整

### 11.1 Web 配置

集中解析并验证：

```text
APP_ENV
DATABASE_URL
PORT
JWT_SECRET
AUTH_ACCESS_TTL
AUTH_REFRESH_TTL
AUTH_COOKIE_SECURE
AUTH_COOKIE_DOMAIN
CORS_ALLOWED_ORIGINS
JUDGE_API_TOKEN
JUDGE_LONG_POLL_TIMEOUT
JUDGE_LEASE_TTL
TESTDATA_ROOT
MIGRATIONS_DIR
```

### 11.2 Compose

- Judge `depends_on` 改为 Web healthy。
- Judge 删除 `DATABASE_URL`，新增 `JUDGE_API_URL`、`JUDGE_API_TOKEN`、`JUDGE_WORKER_ID`。
- Web 和 Judge 使用同一个 Judge service token，但不得将 token 写入日志或文档示例真实值。
- 保持 `/testdata` 在 Web 可写、Judge 只读。
- 不改变 Judge 的 capability、AppArmor、seccomp、cgroup、read-only rootfs 和 tmpfs 配置。

### 11.3 HTTP server timeout

- `ReadHeaderTimeout`、普通 request timeout 和 long-poll timeout 分开考虑。
- `WriteTimeout` 必须大于 Judge long poll timeout并留网络余量。
- Internal Judge API 对 request body 设置严格大小上限；result case 数量与 payload 大小必须受限。

## 12. 实施顺序与阶段验收

### Phase 0：基线和保护

1. 记录 HEAD、分支和现有未提交修改；工作区不要求干净，但不得丢弃、覆盖或混淆已有修改。
2. 跑现有 Web/Judge test、vet、frontend build。
3. 记录现有公开路由和 E2E 行为。

验收：基线全绿；无现有修改被丢弃。

### Phase 1：测试/配置/领域模块骨架

1. 引入 Ginkgo v2/Gomega。
2. 新增 config package 与纯函数测试。
3. 建立 Identity/Problem/Contest/Submission/Judge/Content 一级领域目录，在领域根包直接放置 domain/service/store 文件，并在各领域下建立 dto/handler 子包。
4. 为各领域 handler 增加 `router.go`，由外层 HTTP 组合代码注册路由；迁移依赖注入但不改变业务行为。

验收：所有旧 API/E2E 行为不变；领域根包不依赖本领域 DTO/handler；路由注册不再自行创建 store/service。

### Phase 2：Auth session

1. 在 `000001_init` 中加入 `auth_sessions`。
2. 在 Identity 根包实现 token manager、session repository 和 auth service。
3. 实现 refresh/logout/logout-all 和 session-aware middleware。
4. 使用 DTO 和统一错误响应。
5. 完成 Ginkgo specs。

验收：刷新轮换、立即吊销、并发旧 token 复用测试全部通过；兼容 `token` 字段仍可用。

### Phase 3：Submission/Judge 领域与长轮询

1. 在 `000001_init` 中加入 `judge_jobs` 和 submission generation。
2. 将 submission 创建/rejudge 改为事务性创建 generation job。
3. 实现 claim dispatcher、PostgreSQL NOTIFY listener 和低频兜底。
4. 实现 internal Judge handlers、DTO、service token middleware。
5. 实现 lease/heartbeat/result fencing 和事务性结果写入。
6. 完成并发、过期、迟到结果和幂等 specs。

验收：Web 数据库仍是唯一事实源；空闲 worker 不产生 500ms HTTP/DB 短轮询；stale result 永远不能覆盖新结果。

### Phase 4：Judge worker HTTP client

1. 删除 Judge PostgreSQL store 和 pgx 依赖。
2. 添加长轮询 client、heartbeat 和 result retry。
3. 更新 scheduler 接口和取消语义。
4. 更新 Compose/Docker 配置。

验收：Judge 容器没有数据库凭据；Web/Judge 断连和恢复测试通过；sandbox smoke 不回退。

### Phase 5：其余 Web 领域重构

按 Problem → Contest → Content 顺序迁移：

1. 各领域根包的 service 接管业务规则和授权，并依赖窄 repository interface。
2. 各领域根包的 store 使用 sqlx 负责持久化和完整事务边界，handler/service 不直接执行 SQL。
3. 每个领域的 handler 使用本领域 DTO 和统一错误 mapper；DTO 按业务类型组织。
4. 删除此次迁移产生的旧 model/store 重复代码。

验收：handler 中没有直接 SQL/store 业务编排；公开响应无数据库 entity 泄漏；现有 E2E 全绿。

### Phase 6：Swag、Orval 与前端认证

1. 为所有路由添加 swag annotations 并生成规范。
2. 暴露 Swagger UI。
3. 将前端包管理迁移到 pnpm，生成并提交唯一的 `pnpm-lock.yaml`，删除 `package-lock.json`，同步 CI 与开发文档。
4. 配置 Orval 并生成 Axios client/types。
5. 迁移前端 API 调用和 AuthProvider。
6. 增加生成物漂移检查。

验收：删除手写重复 API 类型和 URL；frontend build 全绿；生成后工作区无 diff。

### Phase 7：文档、CI、E2E 和最终审计

更新：

- `docs/01-architecture.md`
- `docs/03-database.md`
- `docs/04-api.md`
- `docs/06-deployment.md`
- `docs/07-roadmap.md`
- `.github/workflows/e2e.yml`

执行完整验证并检查 diff；不要用 AppArmor 放宽配置修复 WSL 的空 profile。

## 13. 最终验证顺序

```text
1. gofmt 所有修改过的 Go 文件
2. go -C web mod tidy
3. go -C judge mod tidy
4. go -C web vet ./...
5. go -C web test ./...
6. go -C judge vet ./...
7. go -C judge test ./...
8. 重新运行 swag 生成，确认 web/docs 无漂移
9. pnpm --dir webui install --frozen-lockfile
10. pnpm --dir webui run api:generate
11. 确认 webui/src/generated/api 无漂移
12. pnpm --dir webui run build
13. docker compose up -d --build --wait --wait-timeout 120
14. 容器安全边界断言
15. docker compose exec -T judge /usr/local/libexec/vertex-sandbox-smoke-test
16. go -C web test ./e2e/ -v -timeout 20m
17. 验证 Judge 容器环境无 DATABASE_URL
18. 验证 refresh/logout/stale lease E2E
19. git diff --check
20. 完整审阅 git diff 与 `000001_init` up/down
```

Windows/WSL 环境继续使用 `D:\dev\Vertex` 作为唯一 workspace；不要重新加入 WSL UNC writable root。WSL 本地 AppArmor profile 可能为空，只记录环境限制，不能配置 `apparmor=unconfined`。

## 14. 完成标准

以下条件全部满足才可认为 Goal 完成：

- Judge 不再 import pgx、不再读取 `DATABASE_URL`、不再直接写数据库。
- Judge 空闲时使用 Web 长轮询，不产生固定 500ms 的无效 HTTP/DB 请求。
- claim、heartbeat、result 都有 service auth、generation 和 lease token。
- stale/duplicate result 有自动测试且不会覆盖当前 generation。
- Access JWT 短期有效、Refresh Token 轮换且仅存 hash、logout 可立即吊销。
- 前端不在 localStorage 保存 access/refresh token。
- Gin handler 只调用所属领域 service，不直接执行 SQL 或依赖具体 SQLStore；所有公开请求/响应使用所属领域 DTO。
- Identity/Problem/Contest/Submission/Judge/Content 是 `internal` 下的一级领域目录；领域实体、service 和 store 直接位于领域根包，DTO 与 handler 是领域子包。
- DTO 文件按业务类型而不是 request/response 方向组织；领域实体与 HTTP DTO 分离，领域根包不反向依赖 DTO/handler。
- 每个领域 handler 包由 `router.go` 暴露路由注册，外层 HTTP 组合代码只组装依赖并调用注册函数。
- 新增/重构后端单元测试使用 Ginkgo v2/Gomega，并可由 `go test ./...` 执行。
- Swag 规范覆盖全部现有路由并在 CI 检查漂移。
- 前端 API 类型和请求函数由 Orval 生成，手写层只处理横切关注点。
- 前端安装、生成、检查和构建统一使用固定版本 pnpm；仓库只保留 `webui/pnpm-lock.yaml`，不存在 `package-lock.json` 或 npm-only 脚本。
- 现有业务 E2E、Judge sandbox smoke、Docker 安全边界和前端 build 全部通过。
- `000001_init` up/down 可审阅、可从空数据库执行。
- 最终工作区不存在临时脚本、凭据、生成缓存或无关格式化修改。

## 15. 关键风险与处理

| 风险 | 处理 |
|---|---|
| 大规模搬包导致 diff 难审 | 按 Phase 逐领域迁移；每次先移动一个领域的根包、DTO、handler 并跑测试，再删除对应旧目录 |
| 扁平领域根包混入传输职责 | DTO 和 Gin handler 必须留在领域子包；根包实体不带 JSON tag，service 只依赖窄接口 |
| 跨领域事务被拆成多个 store 调用 | 事务归触发用例的领域 store，由单个 sqlx transaction 完成全部 fencing 和派生更新 |
| Web 与 Kafka/Redis 双写 | 本次不引入外部队列；PostgreSQL 是唯一事实源 |
| 长轮询耗尽 DB 连接 | Web 每实例只保留一个 LISTEN connection；waiter 不持有 DB transaction |
| 旧 Judge 迟到结果覆盖 rejudge | generation + lease token 条件更新和事务 fencing |
| refresh token 泄漏 | HttpOnly cookie、数据库只存 SHA-256、每次刷新轮换 |
| logout 后 JWT 仍可用 | RequireAuth 查询权威 session 状态 |
| 多个 401 触发刷新风暴 | 前端 single-flight refresh promise |
| npm/pnpm 双锁文件导致依赖漂移 | 固定 pnpm 版本，只提交 `pnpm-lock.yaml`；CI 使用 `--frozen-lockfile` 并禁止 npm 更新锁文件 |
| Swag 注解与真实 API 漂移 | CI 重新生成并 `git diff --exit-code` |
| Swagger 2.0 被误当成 OpenAPI 3 | 文档明确版本；Orval 直接消费 v2；不手工伪造 v3 |
| WSL AppArmor 为空 | CI Ubuntu 继续断言 `docker-default`，本地不放宽安全配置 |
