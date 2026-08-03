# Vertex Online Judge

[![CI](https://github.com/RimuruChan/Vertex/actions/workflows/e2e.yml/badge.svg)](https://github.com/RimuruChan/Vertex/actions/workflows/e2e.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Vertex 是一个可自托管、container-first 的在线判题平台，覆盖题目管理、代码评测、ACM/ICPC 比赛、榜单、题解与讨论。项目由 Go Web API、独立 Judge Worker、原生 C++ 沙箱、React 前端和 PostgreSQL 组成。

> Vertex 仍处于早期开发阶段，尚未承诺 API 与数据库结构稳定性。运行不受信任代码具有固有风险；生产部署前请完整阅读[沙箱安全模型](docs/02-judge-sandbox.md)与[部署清单](docs/06-deployment.md)。

## 功能

- C、C++、Python 编译与评测，支持逐测试点结果和 AC/WA/TLE/MLE/RE/CE/OLE/SE 判定。
- Markdown + LaTeX 题面、测试数据版本化上传、题目可见性和管理后台。
- ACM/ICPC 比赛、报名、实时榜单、封榜、解榜与赛后练习。
- 题解和题目讨论，前端使用 Markdown、KaTeX 渲染。
- 短期 Access JWT、opaque Refresh Token 轮换、单会话/全会话吊销与 user/admin 角色。
- Judge 通过内部 HTTP 长轮询 Web，不持有数据库凭据，也不依赖 Kafka 或 Redis。
- 自研 `vertex-sandbox` 使用 Landlock、seccomp、cgroup v2 和 rlimit 约束不受信任进程。

## 架构

```text
Browser ── HTTP / polling ──> Web API ── sqlx ──> PostgreSQL
                                  ▲
                                  │ claim / heartbeat / result
                                  │
                              Judge Worker
                                  │
                         compile → sandbox → checker
```

PostgreSQL 是业务和 Judge job 的唯一事实源。`LISTEN/NOTIFY` 仅用于唤醒等待中的 Web 请求；job 领取使用 `FOR UPDATE SKIP LOCKED`，租约结果通过 generation、worker identity 和 lease token fencing。

| 项目 | 技术 | 职责 | 开发文档 |
|---|---|---|---|
| Web API | Go、Gin、sqlx | 领域逻辑、认证、持久化、Judge 调度协议、OpenAPI | [`web/README.md`](web/README.md) |
| Judge | Go、C++ | 长轮询任务、编译、隔离执行、checker、结果回传 | [`judge/README.md`](judge/README.md) |
| Web UI | React、TypeScript、Vite、Ant Design | 用户界面、会话恢复、生成式 API 客户端 | [`webui/README.md`](webui/README.md) |
| Database | PostgreSQL 16 | 用户、比赛、提交、session 与 Judge job/lease | [`docs/03-database.md`](docs/03-database.md) |

## 快速开始

### 环境要求

- x86_64 Linux，启用 cgroup v2 与 Landlock ABI 1 或更高版本（通常需要 Linux 5.13+）。
- rootful Docker Engine 24+ 与 Docker Compose v2。
- Windows/macOS 开发者可以编辑和运行普通单元测试，但完整 Judge 必须运行在满足上述条件的 Linux 或 WSL2 Linux Docker 中。

### 启动 API、Judge 与 PostgreSQL

```bash
cp .env.example .env
```

编辑 `.env`，至少显式设置以下四项；不要把该文件提交到 Git：

```dotenv
POSTGRES_PASSWORD=<random-password>
JWT_SECRET=<at-least-32-random-characters>
JUDGE_API_TOKEN=<at-least-32-random-characters>
ADMIN_PASSWORD=<random-admin-password>
```

可以使用 `openssl rand -hex 32` 分别生成随机值。随后启动服务：

```bash
docker compose up -d --build --wait --wait-timeout 120
curl http://localhost:8080/api/health/ready
```

默认 Compose 栈启动 PostgreSQL、Web API 和 Judge，不构建 Web UI。本地使用前端时另开终端：

```bash
cd webui
pnpm install --frozen-lockfile
pnpm run dev
```

浏览器访问 <http://localhost:5173>。默认管理员由 `.env` 中的 `ADMIN_USERNAME` 和 `ADMIN_PASSWORD` 在首次启动时创建。

停止并删除本地服务：

```bash
docker compose down
```

添加 `-v` 会同时删除本地数据库、测试数据和缓存卷，请只在确定不再需要这些数据时使用。

## 开发与验证

```bash
# Web API
go -C web vet ./...
go -C web test ./...

# Judge 的 Go 部分
go -C judge vet ./...
go -C judge test ./...

# 重新生成 OpenAPI 与前端客户端
go -C web generate .
pnpm --dir webui run api:generate

# 前端
pnpm --dir webui install --frozen-lockfile
pnpm --dir webui run build
```

在 Linux 上启动 Compose 后可运行原生沙箱冒烟测试：

```bash
docker compose exec -T judge /usr/local/libexec/vertex-sandbox-smoke-test
```

GitHub Actions 会执行 Go vet/test、OpenAPI 生成一致性检查、前端构建、完整 Compose 启动、沙箱安全边界检查和 API/Judge E2E。

## 仓库结构

```text
web/       Go Web API，领域优先的模块化单体
judge/     Go Judge Worker 与 C++ vertex-sandbox
webui/     React 前端与 Orval 生成的 API 客户端
docs/      架构、数据库、API、沙箱、榜单与部署文档
deploy/    可选 nginx 反向代理配置
.github/   CI 工作流
```

## 文档

- [设计文档索引](docs/README.md)
- [系统架构](docs/01-architecture.md)
- [Judge 沙箱安全模型](docs/02-judge-sandbox.md)
- [数据库设计](docs/03-database.md)
- [API 设计](docs/04-api.md)
- [比赛榜单语义](docs/05-contest-rankboard.md)
- [部署与运维](docs/06-deployment.md)
- [路线图](docs/07-roadmap.md)

## 参与贡献

Issue、缺陷复现和范围清晰的 Pull Request 都欢迎。提交前请：

1. 保持修改聚焦，避免夹带无关重构。
2. 为行为变化补充或更新测试。
3. 运行受影响项目的测试与构建。
4. 如果修改了 HTTP handler 或 DTO，重新生成并提交 `web/docs` 与 `webui/src/generated/api`。
5. 不要提交 `.env`、凭据、测试数据或构建产物。

安全问题不应公开披露具体利用细节；请通过仓库所有者的 GitHub 联系方式进行私下报告。

## License

Vertex 使用 [MIT License](LICENSE) 开源。
