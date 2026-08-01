# Vertex Online Judge

自托管的在线判题平台。支持题目、在线评测、比赛(ACM/ICPC 实时榜单)、题解、题目评论与出题管理。

## 技术栈

| 层 | 选择 |
|---|---|
| 前端 | React 19 + TypeScript + Vite + Ant Design 5 |
| 后端 | Go + Gin |
| 数据库 | PostgreSQL 16(提交队列用 `SKIP LOCKED`) |
| 缓存 | Redis 7(缓存与 wake-up 信号,非权威) |
| 判题沙箱 | ioi/isolate(cgroup v2),运行于 unprivileged Docker 容器 |
| 实时 | 前端轮询兜底(WS 预留) |

## 功能

- **在线判题**:C / C++ / Python,隔离沙箱评测,逐测试点反馈(AC/WA/TLE/MLE/RE/CE/OLE/SE)
- **出题管理**:题面 Markdown+LaTeX、测试数据 zip 上传(1.in/1.out...)、可见性控制
- **比赛**:ACM/ICPC 赛制、实时榜单、封榜/解榜、赛后练习
- **题解与评论**:Markdown + KaTeX 渲染,题目评论、题解发布
- **账号**:注册/登录、user/admin 两角色

## 快速开始(需要 Linux 服务器 + Docker)

```bash
# 1. 配置环境变量
cp .env.example .env
# 编辑 .env:设置 JWT_SECRET / POSTGRES_PASSWORD / ADMIN_PASSWORD

# 2. 启动(web + judge + postgres + redis)
docker compose up -d --build

# 3. 访问 http://<服务器>:8080/api/health 确认 web 就绪
#    默认管理员由 .env 的 ADMIN_USERNAME/ADMIN_PASSWORD 创建
```

### 验证一次端到端评测

```bash
# 1. 用 admin 登录获取 token
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"<ADMIN_PASSWORD>"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")

# 2. 创建一道题目
PROBLEM=$(curl -s -X POST http://localhost:8080/api/admin/problems \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"A+B","statementMd":"输入两个整数,输出它们的和。","difficulty":1,"timeLimitMs":1000,"memoryLimitKb":262144,"visibility":"public"}')
PROBLEM_ID=$(echo $PROBLEM | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")

# 3. 上传测试数据(构造 zip:1.in="1 2\n", 1.out="3\n")
mkdir -p /tmp/td && printf "1 2\n" > /tmp/td/1.in && printf "3\n" > /tmp/td/1.out
cd /tmp/td && zip -q ../td.zip 1.in 1.out
curl -s -X POST "http://localhost:8080/api/admin/problems/$PROBLEM_ID/testdata" \
  -H "Authorization: Bearer $TOKEN" -F "file=@/tmp/td.zip"

# 4. 提交 C++ 代码
curl -s -X POST http://localhost:8080/api/submissions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"problemId\":\"$PROBLEM_ID\",\"language\":\"cpp\",\"sourceCode\":\"#include <bits/stdc++.h>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<a+b;}\"}"

# 5. 轮询提交详情直到判定完成,应显示 Accepted
```

## 目录结构

```
web/            Go web API(Gin + pgx)
  internal/api        HTTP 处理器与中间件
  internal/store      PostgreSQL 数据访问
  internal/auth       JWT 认证
  migrations          golang-migrate 版本化 SQL
  e2e/                端到端测试(CI 驱动)
judge/          Go 判题 worker(独立进程)
  internal/run        ioi/isolate 沙箱封装
  internal/compile    语言编译(按源码哈希缓存)
  internal/executor   逐测试点执行与判定
  internal/verdict    判定分类学(信号映射)
  internal/store      判题数据访问(SKIP LOCKED 队列)
webui/          React 前端(Vite + Ant Design)
docs/           设计文档(架构/沙箱/数据库/API/榜单/部署/路线图)
deploy/         nginx 反代配置(可选)
.github/workflows/  GitHub Actions(E2E 测试)
```

## 设计文档

完整设计文档见 [`docs/`](docs/README.md),涵盖架构、判题沙箱安全模型、数据库 Schema、API、比赛榜单、部署与路线图。

## 判题沙箱安全模型

- Worker 容器 **非特权**:只读 rootfs、无 `--privileged`、仅 `CAP_SYS_ADMIN`(isolate 建 mount namespace 所需)
- 不可信代码由 isolate 在独立 mount/PID/net namespace + cgroup v2 内以低权限 box UID 运行
- 资源五限:CPU 时间、墙钟、内存(cgroup)、输出大小、进程数;断网
- 判定映射严格:OOM-kill→MLE、超时→TLE、信号→RE、非零退出→RE

## 开发(Windows)

```bash
# 后端 API(需本地 Postgres,或仅改前端)
cd web && go run ./cmd/server

# 前端
cd webui && npm install && npm run dev   # http://localhost:5173

# 判题 worker 必须在 Linux 上运行(isolate 依赖),见 docker-compose
```

## 路线图(v1+)

- 题单(表已建模)、SPJ 自定义评测机、IOI 子任务赛制、积分 rating、虚拟比赛
- 题解投稿审核工作流、socket.io 实时推送、Rejudge UX
