# Vertex OJ — 部署与运维

## 1. 前提

- Linux 服务器(x86_64),启用 **cgroup v2** 与 **Landlock ABI ≥ 1**(通常为 Linux 5.13+；发行版可能回移或关闭该功能)
- rootful Docker Engine ≥ 24 + Compose v2(rootless 模式无法提供专属 cgroup 子树)
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

Compose 会创建 `/sys/fs/cgroup/vertex-<project>` 并只把该项目子树绑定到 Judge 的 `/vertex-cgroup`。不要手工把整个宿主 cgroupfs 改成可写。

Judge sandbox policy 可通过 `.env` 覆盖：

| 变量 | 默认值 | 约束/用途 |
|---|---:|---|
| `SANDBOX_TIME_OVERSHOOT_MS` | `1000` | soft CPU/wall 到 hard kill 的 grace，非负整数 |
| `SANDBOX_WORKSPACE_BYTES` | `67108864` | 单次 workspace 逻辑字节上限，正整数 |
| `SANDBOX_WORKSPACE_INODES` | `4096` | 单次 workspace 目录项上限，正整数 |
| `SANDBOX_CPUSET` | 空 | 可选 Linux cpulist，例如 `0-3,6` |

`SANDBOX_CPUSET` 为空时保持默认调度；显式配置时，entrypoint 会启用 `+cpuset`，runner 为每个 child cgroup 初始化 `cpuset.mems`。宿主没有委派 cpuset 或配置无效时 Judge 失败关闭，而不是静默忽略绑定。

## 3. 安全加固检查清单

| 项 | 状态 |
|---|---|
| worker 容器无 `--privileged`，capability 白名单不含 `SYS_ADMIN`/`NET_ADMIN` | 已内置 |
| worker 容器 `read_only: true` + tmpfs `/tmp`、`/run` | 已内置 |
| 保留 Docker 默认 seccomp 与 AppArmor，不使用 `apparmor=unconfined` | 已内置/CI 断言 |
| 宿主 cgroupfs 只读，仅项目专属子树 rw | 已内置 |
| 目标代码由 Landlock 限制路径、内层 seccomp 断网 | 已内置 |
| workspace 逻辑字节/目录项 watchdog（默认 64 MiB / 4096） | 已内置 |
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

默认一个 Worker 容器内运行 `JUDGE_WORKERS=2` 个并发判题循环，每个循环自动获得不同 box id。Worker 通过 `JUDGE_API_URL/JUDGE_API_TOKEN/JUDGE_WORKER_ID` 长轮询 Server，不连接 PostgreSQL。横向扩展时每个实例必须使用唯一 worker identity，并分配不重叠的 sandbox box id 范围和独立 cgroup 子树。

Worker 判题通信配置：

| 变量 | 默认值 | 说明 |
|---|---:|---|
| `JUDGE_API_TOKEN` | 必填，无默认值 | Server 与 Worker 共享的独立强 token，至少 32 个字符 |
| `JUDGE_WORKER_ID` | 容器 hostname/PID | 实例身份；横向扩展时应显式设为唯一值 |
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
| `CORS_ALLOWED_ORIGINS` | 本地 Vite origin | 逗号分隔 allowlist |
| `SWAGGER_ENABLED` | development 开启 | 生产默认关闭 |

### 测试数据管理

- 上传:管理后台 → 题目 → 数据(zip 含 `1.in/1.out, 2.in/2.out, ...`)。
- 存储位置:命名卷 `testdata`,内容寻址目录 `/<problemID>/<sha256>/`；旧版本保留到题目删除，避免覆盖运行中的 job 快照。
- 判题 worker 以只读挂载同一卷。

## 5. 可选:nginx 反代 + TLS

`docker compose --profile with-frontend up`(需先构建前端到 `ui/dist` 并挂载)。

生产建议在宿主 nginx/Let's Encrypt 终止 TLS,反代 `:8080`,并转发 `X-Forwarded-*` 头。

## 6. 端到端验证

仓库内置 GitHub Actions 工作流(`.github/workflows/e2e.yml`),在干净 Ubuntu runner 上:
1. 跑 Go 单测、vet，并在真实 PostgreSQL 上验证 Judge 并发/lease 事务
2. 重新生成 Swag/Orval 并检查产物漂移，再构建前端
3. 使用 `docker compose up -d --build` 启动与生产一致的完整服务
4. 断言默认 AppArmor、只读 rootfs、capability/cgroup 边界并运行沙箱安全冒烟
5. 跑通判题、比赛、refresh 轮换/logout 和 Judge stale lease E2E
6. 验证 Worker 容器环境不存在 `DATABASE_URL`；失败时收集日志，最后销毁测试卷

本地(需 Linux)也可手动跑通 README 中的 curl 脚本。

## 7. 升级与备份

- 迁移：当前尚未实际部署，schema 直接合并在 `000001_init` 中。首次生产发布后才开始使用只追加 migration 的升级策略。
- 备份:卷 `pgdata`(全量)+ `testdata`(测试数据)。测试数据体积大,可与 DB 分开备份。
