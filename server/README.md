# Vertex Server

`server` 是 Vertex 的业务与持久化边界。它提供面向浏览器的 REST API、面向 Worker 的内部长轮询协议、认证会话、数据库迁移和 Swagger/OpenAPI 文档。

Go module：`github.com/RimuruChan/Vertex/server`

## 设计

- 领域优先的模块化单体：`identity`、`domain`、`problem`、`authoring`、`problemset`、`contest`、`submission`、`judge`、`content`、`profile`、`console`。
- 每个领域根包包含 domain、service、repository interface 与默认 sqlx store。
- 每个领域的 `dto/` 与 `handler/` 负责 HTTP 边界，外层 router 只负责组合。
- PostgreSQL 是唯一数据源；所有数据库访问统一使用 `sqlx`。
- Access JWT 默认有效 15 分钟，Refresh Token 只以哈希形式持久化并在每次刷新时轮换。
- JSON 写入口统一限长；登录、注册和比赛密码入口使用有界的进程内滥用控制，反代/多实例配额由可信 ingress 补充。
- Worker 使用 service token 调用 `/internal/judge/v1`（判题与题目包构建共用），不直接连接数据库。
- `contest` 拥有三种赛制的计分(纯函数 `ScoreCell`)、封榜双视图、裁判角色与答疑;`submission` 拥有批量重测批次与比赛反馈屏蔽。
- `problemset` 拥有策展题单与个人进度读模型；`content` 拥有题解（投票、草稿、防剧透）与讨论；`console` 提供站点统计/账号治理，以及分开的域内标签和公告治理入口。
- `domain` 拥有官方域、成员/角色/group 与域审计。资源读写、统计和公开编号解析绑定路由域，owner/用户/group 授权在关键写事务中复核；站点管理不绕过资源作用域。
- `authoring` 拥有题目工作副本、结构化材料、封存输入的构建队列和显式发布；构建/导入只准备候选，owner 确认后创建不可变版本，比赛与判题 generation 固定版本。

更完整的设计说明见[系统架构](../docs/01-architecture.md)、[数据库设计](../docs/03-database.md)、[API 设计](../docs/04-api.md)和[出题设计](../docs/08-problem-authoring.md)。

## 环境要求

- Go 1.25.6 或兼容的 Go 1.25 工具链。
- PostgreSQL 16。

本地运行至少需要：

| 环境变量 | 说明 |
|---|---|
| `DATABASE_URL` | PostgreSQL 连接串 |
| `JWT_SECRET` | Access JWT 签名密钥，至少 32 个字符 |
| `JUDGE_API_TOKEN` | Web/Judge 共享 service token，至少 32 个字符 |
| `ADMIN_PASSWORD` | 首次启动时创建管理员所用密码 |

其他配置及默认值见根目录 [`.env.example`](../.env.example) 和[部署文档](../docs/06-deployment.md)。

## 本地运行

最简单的方式是从仓库根目录通过 Compose 启动 PostgreSQL，再向当前 shell 导出配置：

```bash
docker compose up -d postgres

cd server

export DATABASE_URL='postgres://vertex:<password>@localhost:5432/vertex?sslmode=disable'
export JWT_SECRET='<at-least-32-random-characters>'
export JUDGE_API_TOKEN='<at-least-32-random-characters>'
export ADMIN_PASSWORD='<admin-password>'

go run ./cmd/server
```

以上 Go 命令在 `server/` 目录执行。服务默认监听 `:8080`：

- Liveness：<http://localhost:8080/api/health/live>
- Readiness：<http://localhost:8080/api/health/ready>
- Swagger UI：开发环境默认位于 <http://localhost:8080/swagger/index.html>

启动时会应用 `migrations/` 中的 schema，并按配置创建或校验初始管理员。

## 测试

```bash
go vet ./...
go test ./...
```

多个领域（包括 domain、identity、problem、judge、contest、authoring、problemset、content、console、submission）有真实 PostgreSQL 集成测试，由 `TEST_DATABASE_URL` 启用。只使用可重置的独立测试库：套件自动应用 migrations 并清空其测试数据；未设置变量时相关用例 skip，不代表数据库验证通过。

```bash
export TEST_DATABASE_URL='postgres://vertex_test:vertex_test@localhost:5432/vertex_test?sslmode=disable'
go test ./...
```

这些套件共用同一个库并各自 `TRUNCATE`，因此每个套件在 `BeforeSuite` 里取一把 session 级
advisory lock（`internal/platform/database/dbtest`），让 `go test` 并行跑包时互相排队，而不是互相清空
对方的数据。

外部 API E2E 依赖已启动的完整 Compose 栈，通常由根目录 GitHub Actions 执行。没有服务进程或端口时，`go test ./e2e -run '^TestDomainAPIIntegration$' -count=1 -v` 可在上述独立 PostgreSQL 库运行生产路由的进程内 API 集成；它不执行真实程序，不能替代 Docker/Worker E2E。见[协议验证边界](../docs/12-worker-protocol-verification.md)。

## OpenAPI 生成

handler 注解是 API 文档的来源：

```bash
go generate .
```

该命令更新 `docs/docs.go`、`docs/swagger.json` 和 `docs/swagger.yaml`。修改 handler、路由或 DTO 后必须提交生成结果；UI 再从 `swagger.json` 生成客户端。

## 目录结构

```text
cmd/server/                  进程入口与依赖组装
internal/<domain>/           domain、service、sqlx store
internal/<domain>/dto/       按业务类型组织的 HTTP DTO
internal/<domain>/handler/   Gin handler 与本领域 router.go
internal/transport/http/  外层 router、health、CORS、Swagger
internal/platform/database/           sqlx 连接与 migration runner
internal/transport/http/middleware/         用户、管理员与 Judge 认证
internal/transport/http/httpx/              通用 HTTP 响应协议
internal/platform/ratelimit/          有界进程内限流
migrations/                  初始 schema 与后续追加的升级迁移
docs/                        Swag 生成的 API 规范
e2e/                         API/Judge 端到端测试
```
