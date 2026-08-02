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
# 编辑 .env:务必修改 JWT_SECRET、POSTGRES_PASSWORD、ADMIN_PASSWORD

docker compose up -d --build
# 首次启动自动执行迁移 + 创建默认管理员(ADMIN_USERNAME/ADMIN_PASSWORD)
```

验证:
```bash
curl http://<server>:8080/api/health   # → {"status":"ok"}
docker compose exec -T judge vertex-sandbox probe
docker compose exec -T judge /usr/local/libexec/vertex-sandbox-smoke-test
```

Compose 会创建 `/sys/fs/cgroup/vertex-<project>` 并只把该项目子树绑定到 Judge 的 `/vertex-cgroup`。不要手工把整个宿主 cgroupfs 改成可写。

## 3. 安全加固检查清单

| 项 | 状态 |
|---|---|
| judge 容器无 `--privileged`，capability 白名单不含 `SYS_ADMIN`/`NET_ADMIN` | 已内置 |
| judge 容器 `read_only: true` + tmpfs `/tmp`、`/run` | 已内置 |
| 保留 Docker 默认 seccomp 与 AppArmor，不使用 `apparmor=unconfined` | 已内置/CI 断言 |
| 宿主 cgroupfs 只读，仅项目专属子树 rw | 已内置 |
| 目标代码由 Landlock 限制路径、内层 seccomp 断网 | 已内置 |
| 生产必须换 `JWT_SECRET` / `POSTGRES_PASSWORD` | `.env` |
| 反代开启 TLS + `X-Forwarded-Proto` | 见 §5 |

## 4. 常用运维

```bash
docker compose logs -f judge      # 判题日志
docker compose logs -f web        # API 日志
docker compose exec postgres psql -U vertex -d vertex   # 数据库
docker compose restart judge      # 重启判题 worker
```

默认一个 Judge 容器内运行 `JUDGE_WORKERS=2` 个并发循环，每个循环自动获得不同 box id。不要直接用 `docker compose --scale judge` 横向复制默认配置；多容器部署需给每个实例分配不重叠的 `SANDBOX_BOX_ID` 范围和独立 cgroup 子树。

### 测试数据管理

- 上传:管理后台 → 题目 → 数据(zip 含 `1.in/1.out, 2.in/2.out, ...`)。
- 存储位置:命名卷 `testdata`,目录 `/<problemID>/`。
- 判题 worker 以只读挂载同一卷。

## 5. 可选:nginx 反代 + TLS

`docker compose --profile with-frontend up`(需先构建前端到 `webui/dist` 并挂载)。

生产建议在宿主 nginx/Let's Encrypt 终止 TLS,反代 `:8080`,并转发 `X-Forwarded-*` 头。

## 6. 端到端验证

仓库内置 GitHub Actions 工作流(`.github/workflows/e2e.yml`),在干净 Ubuntu runner 上:
1. 跑 Go 单测、vet 与前端构建
2. 使用 `docker compose up -d --build` 启动与生产一致的完整服务
3. 断言默认 AppArmor、只读 rootfs、capability/cgroup 边界并运行沙箱安全冒烟
4. 跑 `web/e2e`:AC(多语言)/WA/TLE/CE/OLE + 比赛榜单
5. 失败时输出所有容器状态和日志,结束后销毁测试卷

本地(需 Linux)也可手动跑通 README 中的 curl 脚本。

## 7. 升级与备份

- 迁移:启动时 golang-migrate 自动执行 `web/migrations/`,向后追加版本即可。
- 备份:卷 `pgdata`(全量)+ `testdata`(测试数据)。测试数据体积大,可与 DB 分开备份。
