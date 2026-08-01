# Vertex OJ — 设计概览

> 状态:已实现(M0–M5 全部完成)。本文描述当前代码库的实际架构。

## 1. 系统定位

Vertex 是一个自托管的 Online Judge:用户提交代码,系统在隔离沙箱中编译、运行并判定结果,支持题目练习、出题管理、比赛与社区交流。

## 2. 技术选型

| 层 | 选择 | 理由 |
|---|---|---|
| 前端 | React 19 + TypeScript + Vite | SPA,页面多但无复杂状态需求 |
| UI 库 | Ant Design 5 | 表单/表格强,适合出题管理后台 |
| 后端 | Go + Gin | goroutine 适合判题推送与榜单并发;单二进制部署 |
| 数据库 | PostgreSQL 16 | 唯一事实源;提交队列用 `SKIP LOCKED`;JSONB 存灵活字段 |
| 缓存 | Redis 7 | 缓存、会话、判题 wake-up 信号(非权威存储) |
| 判题沙箱 | ioi/isolate(`--cg` cgroup v2),运行于 unprivileged Docker 容器 | IOI 久经考验;unprivileged 容器隔离逃逸影响域 |
| 测试数据 | 本地 bind-mount 卷,web 与 judge 共享 | 单机唯一消费方;多机换 MinIO/S3 |
| 前端渲染 | markdown-it + KaTeX(自定义 dollarmath)+ DOMPurify | 题面/评论是用户内容,必须消毒 |

## 3. 组件与部署形态

```
┌─────────────────────────────────────────────┐
│  Frontend SPA (React 19 + Ant Design)        │
│  题库 / 题目 / 提交 / 比赛 / 管理后台          │
└───────────────┬─────────────────────────────┘
                │  HTTP /api/*   (token 认证)
                ▼
┌─────────────────────────────────────────────┐
│  Web API (Go/Gin)  web/                      │
│  认证 · 题目 · 提交 · 比赛 · 题解 · 评论       │
│  插入 submissions(Pending)→ 队列             │
└──────┬──────────────────────────┬───────────┘
       │ SQL                       │ 轮询领取
       ▼                           ▼
┌───────────────┐      ┌──────────────────────┐
│ PostgreSQL 16 │      │ Judge Worker (Go)     │
│  status 行=队列 │      │  judge/               │
│  SKIP LOCKED  │      │  compile → isolate    │
└───────────────┘      │  → 逐测试点判定 → 写回  │
       ▲               └──────────────────────┘
       │  Redis 7(wake-up 信号,非权威)
       └────────────────────────────────────────
```

**服务清单**(docker-compose):`web`(Go API)、`judge`(判题 worker,1–2 副本)、`postgres`、`redis`,可选 `nginx` 反代 + 前端静态。

## 4. 一次提交的完整生命周期

1. **提交**:`POST /api/submissions`。校验登录、题目可见性、语言、限流(10 次/分钟/用户)。
2. **落库入队**:插入 `submissions` 行 `status='Pending'`(比赛内提交带 `contest_id`);可选推 Redis 信号;立即返回 `202 + id`。
3. **领取**:worker 执行 `UPDATE ... WHERE id = (SELECT ... FOR UPDATE SKIP LOCKED)` 原子领取,置 `Judging`。
4. **准备**:读题目限值与测试数据目录。
5. **编译(沙箱内)**:按 `(语言, sha256(源码))` 缓存产物;未命中在 isolate 内编译(带自身时间/内存/输出上限)。
6. **逐测试点运行**:`copyIn` → `isolate --run`(cgroup v2 内存/进程、断网、CPU+墙钟双限)→ 读 meta 映射判定。
7. **写结果**:同一事务写 `submission_cases` 行 + 提交行 `case_results` JSONB 快照 + 更新题目计数。
8. **比赛积分**:若 `contest_id` 存在,调 `record_contest_submission()` 更新积分格(幂等)。
9. **通知**:前端轮询提交详情兜底(WS 预留)。

## 5. 目录结构

```
Vertex/
├── docker-compose.yml          # web + judge + postgres + redis + nginx(可选)
├── web/                        # Go + Gin API
│   ├── cmd/server/             # 入口(自动迁移 + bootstrap admin)
│   ├── internal/
│   │   ├── api/                # HTTP 处理器与中间件(含限流器)
│   │   ├── auth/               # JWT 认证
│   │   ├── model/              # 数据模型
│   │   ├── store/              # PostgreSQL 访问(SKIP LOCKED 队列)
│   │   └── ws/                 # (预留)WebSocket 推送
│   ├── migrations/             # golang-migrate 版本化 SQL
│   └── e2e/                    # 端到端测试(CI 驱动)
├── judge/                      # Go 判题 worker
│   ├── cmd/worker/             # 入口(自检 isolate)
│   └── internal/
│       ├── run/                # ioi/isolate CLI 封装
│       ├── compile/            # 语言编译 + 哈希缓存
│       ├── executor/           # 逐测试点执行与判定
│       ├── checker/            # diff checker(SPJ 接口预留)
│       ├── verdict/            # 判定分类学(信号映射)
│       ├── scheduler/          # 主循环(并发领取/判题)
│       └── store/              # 判题数据访问
├── webui/                      # React 19 + Vite + Ant Design
│   └── src/{pages,components,api,hooks}/
├── docs/                       # 本文档目录
└── deploy/                     # nginx 配置(可选)
```

## 6. 跨服务约定

- **测试数据路径**:`TESTDATA_ROOT/<problemID>/1.in, 1.out, ...`,web 写、judge 读。
- **判定状态**:与 `submissions.status` 的 CHECK 约束一致(见数据库文档)。
- **判题 worker 与 web 不直接通信**:经 Postgres 队列 + 共享卷;Redis 仅作可选信号。
