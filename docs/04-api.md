# Vertex OJ — API 设计

机器可读规范是 Swag 生成的 Swagger/OpenAPI 2.0，以 `server/docs/swagger.json` 和 `/swagger/index.html` 为准；本文只记录跨接口约定和安全语义。

## 通用约定

- 资源 API 使用 `/api/domains/{domain}`；认证、域目录与站点治理留在 `/api`，内部 Worker API 使用 `/internal/judge/v1`。旧无域资源接口固定指向官方域，不依赖客户端当前页面或请求体猜测域。
- 错误统一为 `{"code":"domain.reason","error":"兼容的人类可读信息"}`。
- 分页使用 `page`（从 1 开始）与 `size`（最大 100），列表统一返回 `ListResponse[T]`（`{items,total}`）。
- domain entity 不直接作为 HTTP body。
- 用户认证与 Judge service credential 在 OpenAPI 中使用不同 security definition。
- 所有 JSON 写接口在解码前设置显式 body 上限；超限统一返回 `413 request.too_large`，畸形 JSON 返回 `400 request.invalid`。

题目/包、比赛/赛务、提交/重测、题单、题解/讨论、域内个人统计，以及标签/公告读写已挂载带域路由，同一领域处理器负责新旧路径。下文保留部分无域路径作为兼容示例；生成规范同时描述两套路径，且只为带域路径声明 `domain` 参数。账号管理与系统统计不复制为域内接口；标签和公告治理使用域资源管理能力，不以站点角色代替。

域目录响应区分 `canEnter`、域权限目录、`canTransfer` 与 `canArchive`。归档域仍可向有权恢复的 owner/站点维护者返回 `canArchive=true`，但 `canTransfer=false`、一般写权限为空。公开域可见不等于有效成员；邀请和申请状态通过 membership 接口转换。群组响应的 `canManage` 不自动授予 `canTransfer` 或 `canDelete`，域内成员可读组信息，组成员列表另行检查组关系。

## Auth

| 方法 | 路径                   | 说明                                        |
| ---- | ---------------------- | ------------------------------------------- |
| POST | `/api/auth/register`   | 创建用户和 revocable session                |
| POST | `/api/auth/login`      | 创建 session                                |
| POST | `/api/auth/refresh`    | 使用 HttpOnly cookie 原子轮换 refresh token |
| POST | `/api/auth/logout`     | 吊销当前 `sid` 并清 cookie                  |
| POST | `/api/auth/logout-all` | 吊销用户全部 session                        |
| GET  | `/api/auth/me`         | 返回数据库中的当前用户与角色                |

认证响应包含 `accessToken/expiresIn/user`；`token` 暂时作为 deprecated 兼容别名。refresh token 不出现在 JSON body。
注册和登录 JSON 限制为 16 KiB，密码限制为 6–72 字节；不存在的用户名仍执行一次 bcrypt 比较，避免通过响应耗时枚举账号。
登录同时按规范化账号和 TCP peer 限流，注册按 TCP peer 限流；比赛密码按用户/比赛限流。超过配额统一返回 `429 request.rate_limited`。应用不信任客户端伪造的 `X-Forwarded-For`，反代与多实例部署的真实客户端配额由可信 ingress 补充。

## 查看者相关的读接口

以下接口对匿名与已登录用户返回不同内容，前端必须在 access token 恢复之后再请求，否则会拿到匿名结果。

| 方法 | 路径                    | 说明                                                                                            |
| ---- | ----------------------- | ----------------------------------------------------------------------------------------------- |
| GET  | `/api/problems`         | 可选认证。带 token 时每道题返回 `userStatus`(`solved`/`attempted`/`none`)，并支持 `status` 过滤 |
| GET  | `/api/problems?view=available` | 需登录，返回当前有权引用的已发布题目，包括 owner/协作者可读私题；供比赛和题单选择器使用 |
| GET  | `/api/problems/{id}`    | 可选认证，同样返回 `userStatus`                                                                 |
| GET  | `/api/tags`             | 公开题库的标签目录，按题目数量倒序                                                              |
| GET  | `/api/users/{username}` | 个人主页聚合：通过数、提交数、按难度的通过进度、最近 90 天提交热力图                            |

`userStatus` 只由 `contest_id IS NULL` 的练习提交推导，不做冗余存储，因此 rejudge 与比赛重算不会泄漏封榜或隐藏反馈。
匿名请求一律返回 `none`。

默认题库和公开标签只包含已发布公开题目；可复用视图仍在 SQL 中按当前域、成员及题目权限过滤后计数和分页，不返回未发布工作副本元信息。公开个人统计只计当前域的已发布公开练习，私题和比赛记录不参与。

提交状态筛选使用调用者可见的状态。禁反馈比赛中的终态按 `Submitted` 筛选，不能通过 `status=Accepted` 等原始判定过滤和总数推断隐藏结果。裁判/观察员的授权完整视图，以及解禁反馈后的查询，正常使用真实状态。

## 判题进度

`submissions` 上的 `judgedCases` / `totalCases` 用于前端显示「已评测 3 / 10 个测试点」：

- 前端通过 `GET /api/submissions/{id}/progress` 轮询轻量读模型；该接口与提交详情使用相同的查看者权限，不重复返回源码、用户或题目信息；
- `totalCases` 在 job 被 claim 时按测试数据用例数写入；
- `judgedCases` 由 worker 在 heartbeat 中上报(`judgedCases` 字段，可选)，服务端以 `GREATEST` 更新，乱序心跳不会让进度回退；
- 进度只是展示数据：非法值会被夹取而不是拒绝续租，不会因此丢 lease；
- rejudge 会把两个字段清零。

## 公开编号

页面使用带域的短地址，例如 `/d/official/problems/1000`、`/d/official/contests/42/problems/A`；提交、题解、题单、公告、group 和工作台同样在 `/d/{domain}` 下使用数字编号。旧无域地址固定跳转官方域，旧 UUID 页面链接加载后规范化为当前域的数字地址。

相应 API 的资源路径参数接受 UUID 或公开编号；列表的 `problem` / `contest` 查询参数也兼容两者。比赛题目 API 的 `problemId` 还接受比赛内题号（如 `A`）。响应中的 `id` 及关联 `problemId` / `contestId` 仍是 UUID，`publicId` / `problemPublicId` / `contestPublicId` 用于生成链接。创建、更新请求体内的关联字段不改为公开编号。

编号按域与资源类型分配。无域前缀的兼容资源 API 固定解析到官方域，已知另一域的 UUID 也不会越过 Store 作用域；query/body 不能选择或覆盖域。域中间件在身份认证后运行，对不可访问或停用的成员视角返回 404，归档域的资源写请求返回 403。域资源和治理接口都已注册于 `/api/domains/{domain}`，具体路径以生成规范为准。

比赛题页不显示或加载题解、普通讨论，比赛答疑统一使用澄清接口。练习页的题解和讨论继续遵循原有可见性规则；在比赛中复用公开题目不会使其全站练习内容自动下架。

## 出题接口与题目权限

现有出题接口保留 `/api/admin/problems/{id}` 地址，已移除站点 admin 硬门槛。创建检查域的 `problem.create`；工作台列表只返回自己拥有、被授权或可按域管理的题目。包读取要求 reader/editor/owner 或域资源管理权限，包写入和构建要求 editor/owner 或域资源管理权限。完整清单与语义见[出题设计](08-problem-authoring.md)。

题目响应新增 `ownerId`、`domainId` 和有效 `permissions`。`GET/PUT .../access` 与 `DELETE .../access/{grant}` 分别查看、授予、移除协作授权；`PUT .../owner` 专门转让所有权。通用题目更新不能覆盖 owner 或域。移除个人授权不等于移除 group 继承，授权列表保留来源而不是展开为永久个人权限。关键写入重新读取当前账号、域角色与资源授权，不相信过时的 role 声明。测试数据上传在解析 multipart 前先检查编辑权限，整个 body 和 zip 都有大小上限。

- `GET /api/admin/problems/{id}/package` 一次返回题面、源文件（不含正文）、测试点、最近一次构建和「还不能构建的原因」。
- 源文件按 `(kind, name)` upsert；checker/validator/interactor 限定 C++，保存即设为启用项。
- 测试点的生成命令不经过 shell，服务端与 worker 各自独立校验 argv token。
- `POST .../builds` 在包不完整时返回 `400 authoring.not_buildable`，已有构建在跑时返回 `409` 并带上正在跑的那一个。

## 比赛与赛务接口

完整语义见[赛制与榜单设计](05-contest-rankboard.md)。

- `GET /api/contests/{id}/rankboard` 默认返回按封榜规则裁剪的视图;`?view=jury` 只对裁判/观察员生效,
  其他调用者拿到的仍是封榜视图——服务端决定给哪一套数值,客户端无法绕过。
- `GET /api/contests/{id}` 返回当前 `permissions`、`ownerId`、`domainId`、`admission`，并保留 `staffRole` 兼容字段。比赛详情和赛务读取入口使用后端能力，不把 JWT/客户端的 role 声明当作授权依据。
- 比赛中的题面通过 `GET /api/contests/{id}/problems/{problemId}` 读取；该接口同时验证比赛、题目归属、时段和参赛/赛务身份，非公开比赛题不经过通用 Problem API。
- 答疑回复按 jury 能力检查；人员/协作管理按 owner 或域资源管理能力检查，普通 jury 不能授予角色。`GET/PUT .../access`、`DELETE .../access/{grant}` 保留用户/group 授权来源，`PUT .../owner` 转让给同域有效成员。`DELETE /api/contests/{id}` 只允许删除没有报名、提交、澄清或重测历史的比赛，不删除引用题目。
- 管理列表 `/api/admin/contests` 已移除全站 admin 硬门槛，只返回当前用户可管理或参与协作的比赛；参赛资格通过 `admission` 和 participant 授权控制，报名与提交会在写事务中重新授权。
- 比赛进行中,选手读到的提交按 `contests.feedback` 屏蔽:`summary` 去掉测试点明细,
  `none` 把判定替换为 `Submitted`;比赛结束或裁判查看时恢复完整信息。
- 提交列表、详情和 progress 使用同一条数据库可见性谓词：比赛进行中、榜单隐藏或仍在封榜时，普通用户只能读取自己的提交，赛务人员可读全部；比赛结束且公开榜单已经解封后，其他读者仍只能看到该比赛本身会向其公开的题目。不可见行不会进入分页总数，按 UUID 访问也统一返回 `404`。
- `POST /api/admin/rejudgings` 的选择器不能为空,单批上限 5000 条,返回批次后进度可轮询。
  该历史路径不再要求站点 admin：比赛裁判必须明确指定其负责的 `contestId`；题目 owner 的题目级选择器只影响练习提交，不能重测引用该题的其他比赛。域资源管理者可使用更广的域内选择器。详情、变更列表和取消同样核对当前目标权限，观察员只能读取。

提交列表/详情/进度重新解析当前域权限，忽略过时的管理员布尔声明；域资源管理者及有效赛务能够读取比赛数据。源码单独授权给提交者、域资源管理者、对应比赛赛务或练习题 owner，包 reader 不自动获得其他用户源码。隐藏反馈总是通过比赛领域的当前权限计算，不再凭 role 字符串绕过。

## 社区与后台接口

题单的 `ownerId` 与创建归属 `authorId` 分开，响应包含 `permissions`；兼容字段 `canEdit` 来自已解析的编辑能力，不再由客户端角色推断。`GET/PUT /api/problem-sets/{id}/access`、`DELETE /api/problem-sets/{id}/access/{grantId}` 管理直接用户/group 授权，`PUT /api/problem-sets/{id}/owner` 转让给同域有效成员。编辑协作者不能改变公开可见性、删除、转让或授权他人。条目仍受原题目权限约束；含不可见已有条目时整单替换返回 `400`，元信息仍可单独编辑。所有写入在事务中重新检查当前域与资源权限。

完整清单见[社区与后台管理](09-community-admin.md)。

- 题单、题解的读接口使用 optional auth，按权限附加进度、可读草稿和点赞状态。公告公开读取始终排除草稿；草稿须通过已授权的域内治理接口读取。
- `GET /api/editorials` 返回不含正文的 `EditorialSummaryResponse`；进入阅读页后再通过 `GET /api/editorials/{id}` 获取完整正文。
- 题解的 `solved_only` 在 SQL 读模型中屏蔽；返回 `locked: true` 且详情 `contentMd` 为空，元信息保留。未解锁时同样拒绝点赞与讨论；只有练习 AC 参与解锁。
- 所有题解、讨论操作继承当前域和父题目边界，包括作者本人。关键写入在事务内重新授权，忽略旧管理员布尔声明。作者编辑与治理删除分别由 `permissions.edit` / `permissions.delete` 表示。
- 两种讨论均支持可选 `parentId`，回复必须位于同一题目/题解线程。列表返回 `DiscussionThreadResponse`（含 `items`、`total`、`canPost`），每条评论携带权限。比赛普通讨论接口已移除，使用澄清；删除父楼会级联删除回复。
- 封禁账号后登录返回 `403 auth.account_disabled`,已签发的 access token 立即失效(`401`)。

## Judge 内部协议

判题与构建接口共用同一个 Judge bearer token：

| 方法 | 路径                                           | 成功响应                                              |
| ---- | ---------------------------------------------- | ----------------------------------------------------- |
| POST | `/internal/judge/v1/jobs/claim`                | `200` job 快照；无任务长轮询到期为 `204`              |
| POST | `/internal/judge/v1/jobs/{jobId}/heartbeat`    | `204`                                                 |
| PUT  | `/internal/judge/v1/jobs/{jobId}/result`       | `204`                                                 |
| POST | `/internal/judge/v1/builds/claim`              | `200` 完整题目包；无任务为 `204`                      |
| POST | `/internal/judge/v1/builds/{buildId}/progress` | `204`（同时续租）                                     |
| POST | `/internal/judge/v1/builds/{buildId}/package`  | `200` 产物元信息（octet-stream 上传，lease 走请求头） |
| PUT  | `/internal/judge/v1/builds/{buildId}/result`   | `204`                                                 |

claim job 包含域、发布版本、generation、attempt、lease token/expiry、源码、资源限制以及测试数据路径、候选版本、哈希、用例数和 checker。版本在 generation 创建时固定，领取不追随最新发布；heartbeat/result 必须回传 generation、worker ID 和 lease token。

构建协议的围栏规则与判题一致：lease 不匹配或过期返回 `409 build.stale_lease`。输入在排队时封存，响应包含 domain/data revision。产物上传和成功 result 都不发布，只更新匹配的数据候选。`POST /api/admin/problems/{id}/publish` 显式发布，需 `revision`、`artifactVersion` 和可选 `language`；`GET .../releases` 返回发布记录。`PUT /api/contests/{id}/problems/{problemId}/version` 以 `expectedVersion` / `version` 明确切换比赛版本，不自动重测。

lease 不匹配、过期或 generation 已变化返回 `409 judge.stale_lease`。同一已完成 lease 的 result 重试返回幂等 `204`。result body 限制 16 MiB，case 数量最多 10,000。

### 题目版本复制

`POST /api/domains/{domain}/problem-copies` 的路径选择目标域，请求体为 `sourceDomain`、`sourceProblem`（源域数字编号或 UUID）、正整数 `sourceVersion` 与 `attribution`。源包复制权限和目标域创建权限同时成立才创建独立、未发布的草稿；请求体中的 owner/domain 字段不能覆盖服务端归属。复制说明最多 4096 字节，继承说明合并后最多 8192 字节；文件复制受 64 MiB / 4096 条目上限保护。

`GET /api/domains/{domain}/admin/problems/{id}/origin` 仅返回包协作者可读的历史来源，无来源时返回空对象。详细私域出处不加入公共题目 DTO。正常工作台的「复制与来源」使用同一 API，mock 模式也按所选发布快照复制，不携带源授权、提交或比赛记录。

## 健康检查和文档

- `/api/health/live`：进程 liveness，不访问数据库。
- `/api/health/ready`：readiness，至少检查数据库；容器 healthcheck 使用此端点。
- `/api/health`：兼容的 readiness 别名。
- `/swagger/index.html`：仅在 `SWAGGER_ENABLED=true` 时启用，生产默认关闭。
