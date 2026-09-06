# Vertex OJ — 路线图

## 当前主线能力（开发中）

**已实现**:
- 判题核心:C/C++/Python,自研 C++ 沙箱(Landlock/seccomp/cgroup v2),五限 + 断网,判定分类学,编译缓存
- 出题:题目 CRUD、版本化工作区、题面/测试点/源码管理、validator/generator/参考解构建、测试数据 zip 上传、可见性控制
- 练习:题库筛选分页(含个人进度标记与状态过滤)、题目详情左右分栏(题面 + CodeMirror,原地提交与逐测试点进度)、提交记录/详情(逐测试点)
- 个人:登录后首页仪表盘、个人主页(通过数、难度分布、提交热力图)
- 比赛:ICPC/IOI/OI 计分、实时榜单、封榜/解榜、赛务角色、答疑、批量重测与改判对比
- 社区:题单、题解草稿/发布/防剧透/点赞、题目/题解/比赛讨论串
- 账号：短期 access JWT、opaque refresh 轮换、logout/logout-all 即时吊销、user/admin 角色
- 调度：Worker 通过 Server 长轮询，PostgreSQL job/lease/generation fencing，LISTEN/NOTIFY 唤醒；Worker 不持有数据库凭据
- 判题扩展：内置 checker、testlib 自定义 checker，以及交互题双节点 broker 基础原语（尚未接入完整题型流程）
- API 合约：Swag OpenAPI + Orval 前端客户端，CI 检查生成物漂移
- 基础设施:docker-compose、可选前端 nginx 镜像、GitHub Actions E2E、docs

**明确排除(含原因与预留接口)**:

| 功能 | 阶段 | 原因 | 预留接口 |
|---|---|---|---|
| 交互式题目 | v2 | 单题成本最高(interactor + 管道 + 死锁) | `problems.judge_type` |
| 积分 rating | v1 | 需积累比赛历史 | 持久化每次比赛原始结果,可回溯计算 |
| 虚拟比赛 / 团队 / 查重 / 题目导入 | v1–v2 | 非核心循环 | 无需预留 |
| 题解审核队列 | v1 | 当前已有作者草稿/发布，尚无独立审核角色与队列 | `editorials.status` |
| IOI 子任务依赖/数据分档 | v1 | 当前支持 IOI/OI 计分，尚未表达子任务依赖 | `score` 与 package manifest |

## v1

域、资源所有权和协作权限已提升为当前主线重设计，按 [实施计划](plans/2026-09-07-domain-redesign.md) 推进；不能再以单站点 admin 模型作为最终方案。

- **题单收藏/分叉**：在现有自建题单上增加收藏、复制与协作维护
- **子任务模型**：在现有 IOI/OI 计分上增加分组、依赖和分档反馈
- **积分 rating**:用已持久化的比赛原始结果回溯计算
- **虚拟比赛**:克隆历史比赛数据重放
- **题解审核工作流**:在现有草稿/发布状态上增加审核队列与治理角色
- **前端实时推送**：提交详情与榜单的 SSE/WebSocket 增量推送（当前前端仍轮询；逐测试点进度已通过 heartbeat 上报并在轮询中呈现，推送化后可去掉轮询）
- **两阶段重测**：可选的 preview/apply 流程；当前批量重测结果立即生效，并提供改判对比与安全取消

## v2

- 交互式题目
- 子任务依赖/数据分档
- 查重(作弊检测)
- 团队赛
- 统计与题目分析
- 题目导入(fps/qduoj 兼容)
- 更强的隔离(可选 Firecracker 微 VM 后端)

## 当前发布门槛

1. 判题正确性:AC/WA/TLE/MLE/CE/RE/OLE 全用例断言正确
2. 安全清单:worker 无 `--privileged`、默认 AppArmor、断网、只读 rootfs、仅项目 cgroup 子树可写(见沙箱文档)
3. 端到端:CI 中真实跑通判题 + 比赛链路(见 `.github/workflows/e2e.yml`)
4. 比赛演练:一场 ~20 人 ICPC 比赛无人值守,freeze → reveal → rejudge 正确
