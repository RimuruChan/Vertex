# Vertex OJ — 部署与运维

## 1. 前提

- Linux 服务器(x86_64),内核 ≥ 4.15(**cgroup v2 必需**)
- Docker Engine ≥ 24 + Compose v2
- 可选:域名与 TLS(nginx 反代)

> 判题沙箱 `ioi/isolate` 依赖 Linux 命名空间 + cgroup v2。**Windows/macOS 无法运行判题 worker。**

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
```

## 3. 安全加固检查清单

| 项 | 状态 |
|---|---|
| judge 容器无 `--privileged`,仅 `cap_add: SYS_ADMIN` | 已内置 |
| judge 容器 `read_only: true` + tmpfs /tmp | 已内置 |
| judge 挂载 cgroupfs 为 rw(供 isolate `--cg`) | 已内置 |
| 目标代码断网(isolate 默认 netns) | 已内置 |
| 生产必须换 `JWT_SECRET` / `POSTGRES_PASSWORD` | `.env` |
| 反代开启 TLS + `X-Forwarded-Proto` | 见 §5 |

## 4. 常用运维

```bash
docker compose logs -f judge      # 判题日志
docker compose logs -f web        # API 日志
docker compose exec postgres psql -U vertex -d vertex   # 数据库
docker compose restart judge      # 重启判题 worker
```

### 测试数据管理

- 上传:管理后台 → 题目 → 数据(zip 含 `1.in/1.out, 2.in/2.out, ...`)。
- 存储位置:命名卷 `testdata`,目录 `/<problemID>/`。
- 判题 worker 以只读挂载同一卷。

## 5. 可选:nginx 反代 + TLS

`docker compose --profile with-frontend up`(需先构建前端到 `webui/dist` 并挂载)。

生产建议在宿主 nginx/Let's Encrypt 终止 TLS,反代 `:8080`,并转发 `X-Forwarded-*` 头。

## 6. 端到端验证

仓库内置 GitHub Actions 工作流(`.github/workflows/e2e.yml`),在干净 Ubuntu runner 上:
1. 安装 isolate + 工具链
2. 起 Postgres/Redis 服务
3. 原生启动 web + judge
4. 跑 `web/e2e`:AC(多语言)/WA/TLE/CE + 比赛榜单

本地(需 Linux)也可手动跑通 README 中的 curl 脚本。

## 7. 升级与备份

- 迁移:启动时 golang-migrate 自动执行 `web/migrations/`,向后追加版本即可。
- 备份:卷 `pgdata`(全量)+ `testdata`(测试数据)。测试数据体积大,可与 DB 分开备份。
