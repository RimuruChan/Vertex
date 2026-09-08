# 域内读模型与统计审计

本记录覆盖 2026-09-07 对当前 Server 读路径的逐项核对及回归，不代表 Docker、移动端或整个重设计已经验收。源码入口已按 2026-09-08 的 sqlc/DDD 布局更新。总进度见 [实施计划](plans/2026-09-07-domain-redesign.md)。

## 口径与授权入口

账号身份全站唯一，业务数据按路由域解释；旧路径固定官方域。`identity/application.Service.Authenticate` 从有效 session 重新加载当前用户，域资源再通过 `tenancy/infrastructure/postgres.ResourceScope` 读取账号、成员和角色，不能用 JWT 中的旧 admin 声明替代当前权限。UUID/数字解析不授予访问权。

| 读模型 | 当前边界 | 源码与回归证据 |
| --- | --- | --- |
| 题库及 count | 默认只列已发布公开题目；`view=available` 需登录，SQL 在分页前筛选有权引用的已发布私题，不读后续工作元信息 | `problem/application/service.go`、`problem/infrastructure/postgres/queries.go`、`access_test.go` 的 reuse candidates 回归 |
| 标签 | 公开目录仅统计本域已发布公开题目；治理目录需域资源管理权，可以计全部关联 | `problem/infrastructure/postgres/queries.go`、`console/infrastructure/postgres/catalog.go`、域资源治理回归 |
| 公开个人统计 | 当前域、已发布公开题目、练习提交；不因查看者是 owner 而加入私题或比赛结果。难度总数、通过数、活动使用同一口径 | `profile/infrastructure/postgres/queries.go`、`queries_test.go` |
| 提交列表/count/详情/进度 | 当前域与共享查看者条件；本人、题目协作、赛务、赛后公开状态分别处理。源码独立授权，列表不含源码 | `submission/infrastructure/postgres/queries.go`、`queries_test.go`、`target_test.go` |
| 提交筛选与反馈 | 禁反馈终态作为 `Submitted` 筛选；原始 AC/WA 不可通过 count 探测。summary/full/赛后及赛务沿用各自可见字段 | `submission/infrastructure/postgres/queries/read.sql`、`submission/application/feedback.go`，真实数据库与 HTTP count 回归 |
| 比赛榜单 | 先检查域、比赛访问及赛务；开赛前普通用户不能从榜单提前读取赛题。封榜只投影公开格，解榜后普通用户读完整格但 `juryView=false` | `contest/application/service.go`、`contest/infrastructure/postgres/rankboard.go`、`contest/transport/http/dto/rankboard.go`，public unfreeze 回归 |
| 题单条目、计数与个人进度 | 题单与题目分别授权；同一题目可读谓词用于 count/条目/进度，仅练习 AC 解锁。隐藏条目阻止不完整整单替换 | `problemset/infrastructure/postgres/queries.go`、`repository.go`、`access_test.go` |
| 题解与讨论 | 继承域和父题目；正文在 SQL 中执行防剧透投影，列表不含正文；楼中楼受同线程约束，作者也不能绕过父资源撤权 | `content/infrastructure/postgres/editorial.go`、`discussion_access.go`，社区父资源回归 |
| 重测及变更计数 | 域与父题目/比赛治理权限；批次成员只从已限定目标选择。比赛裁判不能用裸提交列表扩大到别场 | `submission/infrastructure/postgres/rejudging.go`、`rejudge_access.go`、`rejudging_queries.go`，重测/并发授权回归 |
| 公告列表与 count | 公开入口始终过滤草稿；治理入口独立授权，搜索/分页之前确定域与公开状态 | `console/infrastructure/postgres/announcements.go`、`resources_test.go` |
| 站点账号/系统统计 | 明确为全站管理读，不复制成域接口；公开题目数只计实际发布者，其余管理总数包含所有域 | `identity/application/service.go`、`middleware/auth.go`、`console/infrastructure/postgres/stats.go` |

子表查询有的直接带 `domain_id`，有的通过已在当前域验证的全局唯一父 UUID 限定；后者依赖实际外键和父资源授权，不能从任意请求 ID 直接跳过父验证。

## 本轮修复

- 补齐 profile 与公开标签 Store 的域读取复核，排除公开但未发布的题目。
- 增加已授权复用视图，比赛/题单选择器可以选择私有已发布题；题单选择器支持分页，普通题库行为不变。
- 把状态过滤放到可见状态口径，修复禁反馈 count 侧信道。
- 分离完整榜单投影与赛务身份，修复公开解榜后仍读封榜快照的问题；封榜待定数不再混入完整视图。
- mock 的源码/反馈/过滤/公开统计按相同边界处理，练习计数只在完成评测时更新且不含比赛；站点模拟统计汇总所有域。榜单从模拟报名和提交推导 ICPC/IOI/OI，不再虚构固定人员和分数。

## 合并验收与边界

Worker 跨域任务快照/fencing、文档配置与浏览器角色/域/窄屏矩阵已在 [协议验证](12-worker-protocol-verification.md) 和 [联合验收](13-domain-redesign-acceptance.md) 收拢。本记录的 SQL/服务回归仍不能替代真实部署；本机未执行完整 Docker/外部 E2E。mock 不执行程序，不作为真实评测或计分正确性的替代证据。
