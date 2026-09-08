# Vertex OJ — 部署与运维

## 1. 前提

- Linux 服务器(amd64/arm64),启用 **cgroup v2** 与 **Landlock ABI ≥ 1**(通常为 Linux 5.13+；发行版可能回移或关闭该功能)
- rootful Docker Engine ≥ 24 + Compose v2，或支持 privileged 容器的 Podman 与 Compose provider
- 可选:域名与 TLS(nginx 反代)

> 判题 worker 依赖 Docker 所在 Linux 内核，而不是客户端操作系统。Windows 可使用启用 cgroup v2 的 WSL2 Linux Docker；原生 Windows 容器无法运行。

启动前可检查 cgroup v2：

```bash
test -f /sys/fs/cgroup/cgroup.controllers
```

若旧版 WSL2 没有统一 cgroup v2，可在 Windows 用户目录 `.wslconfig` 中配置后执行 `wsl --shutdown`：

```ini
[wsl2]
kernelCommandLine = cgroup_no_v1=all
```

## 2. 部署步骤

> 当前仍处于首次发布前，schema 变更会直接维护 `000001_init`，旧开发数据库不会自动重放它。先备份需要保留的数据库与测试数据，再使用新的数据库/Compose project 验证新版本；旧卷可以保留。只有明确放弃全部旧开发数据时才使用 `docker compose down -v --remove-orphans`，该命令会永久删除 PostgreSQL、测试数据和缓存卷，不是无损升级步骤。

```bash
cp .env.example .env
# 编辑 .env:必须显式设置 JWT_SECRET、JUDGE_API_TOKEN、POSTGRES_PASSWORD、ADMIN_PASSWORD

docker compose up -d --build
# 首次启动自动执行迁移 + 创建默认管理员(ADMIN_USERNAME/ADMIN_PASSWORD)
```

上述密码和 token 均没有内置默认值；缺失或留空时 Compose/进程会直接拒绝启动。`JWT_SECRET` 与 `JUDGE_API_TOKEN` 至少为 32 个字符，建议分别使用 `openssl rand -hex 32` 生成。

验证:
```bash
curl http://<server>:8080/api/health/live    # → {"status":"ok"}
curl http://<server>:8080/api/health/ready   # → {"status":"ok"}
docker compose exec -T worker vertex-sandbox probe
docker compose exec -T worker /usr/local/libexec/vertex-sandbox-smoke-test
```

Worker 以 privileged 启动，entrypoint 从 `/proc/self/cgroup` 解析本容器的 cgroup，在其中创建 `vertex-manager` 管理进程组和 `vertex-jobs/instance-<SANDBOX_INSTANCE_ID>` 执行子树。它自动启用 cpu/memory/pids；无需宿主目录挂载、手工创建子树或 `cgroup: host`。任务子树保留在容器资源层级内，受容器总内存/CPU 配额约束。Compose 单机部署应按节点容量配置 Worker 的 `mem_limit`、`cpus` 和并发数；没有设置总预算不代表默认无限并发安全。

Judge sandbox policy 可通过 `.env` 覆盖：

| 变量 | 默认值 | 约束/用途 |
|---|---:|---|
| `SANDBOX_TIME_OVERSHOOT_MS` | `1000` | soft CPU/wall 到 hard kill 的 grace，非负整数 |
| `SANDBOX_WORKSPACE_BYTES` | `67108864` | 单次 workspace 逻辑字节上限，正整数 |
| `SANDBOX_WORKSPACE_INODES` | `4096` | 单次 workspace 目录项上限，正整数 |
| `SANDBOX_CPUSET` | 空 | 可选 Linux cpulist，例如 `0-3,6` |
| `SANDBOX_INSTANCE_ID` | 容器 hostname | 实例目录/cgroup 命名空间；1–64 位安全字符，scaled service 应保持为空 |
| `SANDBOX_BOX_ID` | `0` | 实例内起始 box id；不同实例可以复用同一范围 |

`SANDBOX_CPUSET` 为空时保持默认调度；显式配置时，entrypoint 启用 `+cpuset`，在任务父组与实例组中继承容器有效 CPU/memory node 集合，runner 再为每个 child cgroup 应用指定 CPU 集。不会修改 namespace 根的 CPU 限额。cpu/memory/pids controller 缺失、cpuset 不可用或配置无效时 Worker 失败关闭，而不是静默降级。

## 3. 安全加固检查清单

| 项 | 状态 |
|---|---|
| Worker 监督进程使用 privileged；不可信 child 降 UID、清空 capabilities、禁止提权 | 已内置/沙箱冒烟 |
| worker 容器 `read_only: true` + tmpfs `/tmp`、`/run` | 已内置 |
| 内层 seccomp 过滤网络与危险调用；AppArmor 不是启动依赖 | 已内置/沙箱冒烟 |
| 自动创建的任务 cgroup 留在容器资源层级内 | 已内置/CI 断言 |
| 目标代码由 Landlock 限制路径、内层 seccomp 断网 | 已内置 |
| workspace 逻辑字节/目录项 watchdog（默认 64 MiB / 4096） | 已内置 |
| Worker replica 的 sandbox、scratch、cgroup 与进程锁按 instance id 隔离 | 已内置/CI 断言 |
| `JWT_SECRET` / `JUDGE_API_TOKEN` / `POSTGRES_PASSWORD` / `ADMIN_PASSWORD` 必须显式配置 | `.env`，缺失时失败关闭 |
| Worker 容器不含 `DATABASE_URL`，只有 Server service token | 已内置/CI 断言 |
| 反代开启 TLS + `X-Forwarded-Proto` | 见 §5 |

## 4. 常用运维

```bash
docker compose logs -f worker     # 判题日志
docker compose logs -f server     # API 日志
docker compose exec postgres psql -U vertex -d vertex   # 数据库
docker compose restart worker     # 重启判题 worker
```

默认一个 Worker 容器内运行 `JUDGE_WORKERS=2` 个并发判题循环，每个循环自动获得不同 box id。Worker 通过 `JUDGE_API_URL/JUDGE_API_TOKEN/JUDGE_WORKER_ID` 长轮询 Server，不连接 PostgreSQL。

同一 Compose service 可以直接横向扩展：

```bash
docker compose up -d --scale worker=2
```

scaled service 必须让 `SANDBOX_INSTANCE_ID` 保持为空。Docker 为每个容器分配唯一 hostname，entrypoint 用它隔离 sandbox workspace、scratch 与 cgroup 实例根；cache 与只读 testdata 仍共享。box id 只需在单个实例内唯一，所以所有 replica 都可安全使用默认的 `SANDBOX_BOX_ID=0`。Go Worker 会在共享 sandbox volume 上为 instance id 持有进程生命周期文件锁，重复 ID 会失败关闭；显式 instance id 仅用于分别配置且能保证 ID 唯一的 Worker，不能给同一 scaled service 设置一个共享值。

entrypoint 最终以 `exec` 运行 Worker，容器停止信号直接触发 Go 进程已有的优雅 cleanup。实例根在重启后保留并复用，box 初始化会回收本实例同名 box 的残留 cgroup；不得手工让两个仍在运行的容器使用相同 instance id。

Worker 判题通信配置：

| 变量 | 默认值 | 说明 |
|---|---:|---|
| `JUDGE_API_TOKEN` | 必填，无默认值 | Server 与 Worker 共享的独立强 token，至少 32 个字符 |
| `JUDGE_WORKER_ID` | 容器 hostname/PID | 协议实例身份；自定义时必须保证每个 replica 唯一 |
| `JUDGE_WORKERS` | `2` | 单实例并发判题循环数 |
| `SANDBOX_INSTANCE_ID` | 容器 hostname | 文件/cgroup 实例命名空间；Compose scale 时留空 |
| `SANDBOX_BOX_ID` | `0` | 实例内 box 范围起点 |
| `JUDGE_LONG_POLL_TIMEOUT` | `25s` | claim 的服务端等待时间，必须为整秒 |
| `JUDGE_HTTP_TIMEOUT` | `40s` | HTTP 请求上限，必须大于长轮询时间 |
| `JUDGE_LEASE_TTL` | `45s` | Server 侧租约时长，必须大于长轮询时间 |

Server 认证配置：

| 变量 | 默认值 | 说明 |
|---|---:|---|
| `DATABASE_URL` | 必填，无默认值 | PostgreSQL 连接串 |
| `JWT_SECRET` | 必填，无默认值 | access JWT 签名密钥，至少 32 个字符 |
| `AUTH_ACCESS_TTL` | `15m` | 短期 access JWT |
| `AUTH_REFRESH_TTL` | `720h` | opaque refresh session |
| `AUTH_COOKIE_SECURE` | development 为 `false` | 生产必须配合 HTTPS |
| `AUTH_COOKIE_DOMAIN` | 空 | refresh cookie domain |
| `RATE_LIMIT_WINDOW` | `1m` | 登录、注册与比赛密码尝试的固定计数窗口 |
| `RATE_LIMIT_MAX_KEYS` | `10000` | 单个 Server 进程保留的活跃限流 key 硬上限 |
| `AUTH_LOGIN_RATE_LIMIT` | `10` | 每账号、每窗口的登录尝试上限 |
| `AUTH_LOGIN_CLIENT_RATE_LIMIT` | `120` | 每 TCP peer、每窗口的登录尝试上限 |
| `AUTH_REGISTER_RATE_LIMIT` | `20` | 每 TCP peer、每窗口的注册尝试上限 |
| `CONTEST_REGISTER_RATE_LIMIT` | `10` | 每用户/比赛、每窗口的报名密码尝试上限 |
| `CORS_ALLOWED_ORIGINS` | 本地 Vite origin | 逗号分隔 allowlist |
| `SWAGGER_ENABLED` | development 开启 | 生产默认关闭 |

应用内限流有硬内存上限且 key 只保存 SHA-256，不记录用户名、密码或原始地址。客户端维度只读取 TCP `RemoteAddr`，不会信任请求自行提供的 `X-Forwarded-For`；经过反代时它会自然退化为 ingress 级保护。多 Server 实例或需要真实客户端配额的部署，应在可信 ingress 再配置共享/分布式限流。

### 测试数据管理

- 上传:进入当前域出题工作台 → 题目详情 → 测试数据（ZIP 含 `1.in/1.out, 2.in/2.out, ...`）。导入只准备候选；审核并显式发布后才能评测。可见性与发布版本独立，私有发布版本可经授权用于比赛/题单。
- 存储位置:命名卷 `testdata`,内容寻址目录 `/<problemID>/<sha256>/`；旧版本保留到题目删除，避免覆盖运行中的 job 快照。
- 判题 worker 以只读挂载同一卷。

## 5. 可选:nginx 反代 + TLS

`docker compose --profile with-frontend up -d --build` 会使用 `ui/Dockerfile` 锁定的 pnpm 安装流程构建前端，并把对应的 `dist` 复制进 nginx 镜像；不依赖宿主预先生成或挂载 `ui/dist`。

生产建议在宿主 nginx/Let's Encrypt 终止 TLS,反代 `:8080`,并转发 `X-Forwarded-*` 头。

## 6. 端到端验证

仓库内置 GitHub Actions 工作流(`.github/workflows/e2e.yml`),在干净 Ubuntu runner 上:
1. 跑 Go 单测、vet，并在真实 PostgreSQL 上验证 Judge 并发/lease 事务
2. 重新生成 Swag/Orval 并检查产物漂移，再构建前端
3. 使用 `docker compose up -d --build` 启动与生产一致的完整服务
4. 启动两个 Worker replica，断言实例 workspace/scratch/cgroup/lock 唯一、cache/testdata 共享、只读 rootfs 与容器内 cgroup 层级正确，并运行内层降权、文件/网络隔离、资源限制和进程清理冒烟
5. 跑通判题、比赛、refresh 轮换/logout 和 Judge stale lease E2E
6. 验证 Worker 容器环境不存在 `DATABASE_URL`；失败时收集日志，最后销毁测试卷

本地 Linux Docker 环境可按 `.github/workflows/e2e.yml` 的相同顺序运行；README 的 curl 只检查健康端点，不代表完整端到端验收。

仅验证 API/数据库而不启动服务进程或开放端口时，可给 `TEST_DATABASE_URL` 指向独立测试库，然后运行：

```powershell
go -C server test ./e2e -run '^TestDomainAPIIntegration$' -count=1 -v
```

该测试通过进程内 HTTP transport 调用生产路由，使用真实 PostgreSQL 和临时测试数据目录，覆盖认证、站点治理、域/group/资源流程与内部任务协议。它会在独立测试库中重置 fixtures，不能指向开发或生产业务库；不启动 Worker，不执行提交或构建程序，因此不是容器部署/沙箱 E2E 的替代。

完整服务的 `TestEndToEndDomainWorkflow` 随正常 E2E 运行；`TestEndToEndDomainProtocol` 必须在 Worker 已停止且队列受测试独占时运行，放在旧 Judge fencing 场景之前，避免领取那个场景故意留下的重试任务。

## 7. 升级与备份

- 迁移：当前尚未实际部署，schema 直接合并在 `000001_init` 中。旧 schema 不会自动升级，按 §2 选择新库验证或在明确备份/弃用旧数据后重建；首次生产发布后才使用只追加 migration 的升级策略。
- 备份:卷 `pgdata`(全量)+ `testdata`(测试数据)。测试数据体积大,可与 DB 分开备份。
