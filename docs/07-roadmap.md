# Vertex OJ — 路线图

## MVP(已交付,2026-08-01)

**已实现**:
- 判题核心:C/C++/Python,isolate 沙箱,cgroup v2 内存/进程限制,五限 + 断网,判定分类学,编译缓存
- 出题:题目 CRUD、测试数据 zip 上传、可见性控制
- 练习:题库筛选分页、题目详情(CodeMirror 提交)、提交记录/详情(逐测试点)
- 比赛:ACM/ICPC、实时榜单、封榜/解榜、赛后练习
- 社区:题解发布、题目评论(线程回复)
- 账号:注册/登录、user/admin 两角色
- 基础设施:docker-compose、GitHub Actions E2E、docs

**明确排除(含原因与预留接口)**:

| 功能 | 阶段 | 原因 | 预留接口 |
|---|---|---|---|
| 题单 | v1 | 纯内容聚合 | `problem_sets` 表已建模 |
| SPJ 自定义评测机 | v1 | 需二次构建 RCE 加固的 checker 执行路径 | `checker` 字段 + 统一 checker 接口 |
| 交互式题目 | v2 | 单题成本最高(interactor + 管道 + 死锁) | `problems.judge_type` |
| 积分 rating | v1 | 需积累比赛历史 | 持久化每次比赛原始结果,可回溯计算 |
| 虚拟比赛 / 团队 / 查重 / 题目导入 | v1–v2 | 非核心循环 | 无需预留 |
| 题解投稿工作流(草稿/审核) | v1 | MVP 只做查看/发布 | 与评论共用渲染器 |
| IOI 子任务赛制 | v1 | MVP 只做 ACM | `config_json` 已含 batched 声明 |
| 多租户 domain | v2 | 单站点 admin 足够 | 无 |

## v1

- **题单**:启用已建表,用户自建/收藏题目集合
- **SPJ + testlib**:内置 checker 之上支持外部 checker 二进制(testlib 生态)
- **IOI 赛制**:子任务/部分分,`score` 字段已就绪
- **积分 rating**:用已持久化的比赛原始结果回溯计算
- **虚拟比赛**:克隆历史比赛数据重放
- **题解投稿工作流**:草稿/审核状态机
- **实时推送**:socket.io room-per-submission + 榜单增量推送(替换 5s 轮询)
- **Rejudge UX**:批量重判、结果对比

## v2

- 交互式题目
- 子任务依赖/数据分档
- 查重(作弊检测)
- 团队赛
- 统计与题目分析
- 题目导入(fps/qduoj 兼容)
- 更强的隔离(可选 Firecracker 微 VM 后端)

## 验收标准(MVP)

1. 判题正确性:AC/WA/TLE/MLE/CE/RE/OLE 全用例断言正确
2. 安全清单:worker 无 `--privileged`、断网、只读 rootfs(见沙箱文档 §5)
3. 端到端:CI 中真实跑通判题 + 比赛链路(见 `.github/workflows/e2e.yml`)
4. 比赛演练:一场 ~20 人 ACM 比赛无人值守,freeze → reveal → rejudge 正确
