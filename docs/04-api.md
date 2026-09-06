# Vertex OJ — API 设计

机器可读规范是 Swag 生成的 Swagger/OpenAPI 2.0，以 `server/docs/swagger.json` 和 `/swagger/index.html` 为准；本文只记录跨接口约定和安全语义。

## 通用约定

- 公开 API 前缀 `/api`；内部 Judge API 前缀 `/internal/judge/v1`。
- 错误统一为 `{"code":"domain.reason","error":"兼容的人类可读信息"}`。
- 分页使用 `page`（从 1 开始）与 `size`（最大 100），列表统一返回 `ListResponse[T]`（`{items,total}`）。
- domain entity 不直接作为 HTTP body。
- 用户认证与 Judge service credential 在 OpenAPI 中使用不同 security definition。
- 所有 JSON 写接口在解码前设置显式 body 上限；超限统一返回 `413 request.too_large`，畸形 JSON 返回 `400 request.invalid`。

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
| GET  | `/api/problems/{id}`    | 可选认证，同样返回 `userStatus`                                                                 |
| GET  | `/api/tags`             | 公开题库的标签目录，按题目数量倒序                                                              |
| GET  | `/api/users/{username}` | 个人主页聚合：通过数、提交数、按难度的通过进度、最近 90 天提交热力图                            |

`userStatus` 只由 `contest_id IS NULL` 的练习提交推导，不做冗余存储，因此 rejudge 与比赛重算不会泄漏封榜或隐藏反馈。
匿名请求一律返回 `none`。

## 判题进度

`submissions` 上的 `judgedCases` / `totalCases` 用于前端显示「已评测 3 / 10 个测试点」：

- 前端通过 `GET /api/submissions/{id}/progress` 轮询轻量读模型；该接口与提交详情使用相同的查看者权限，不重复返回源码、用户或题目信息；
- `totalCases` 在 job 被 claim 时按测试数据用例数写入；
- `judgedCases` 由 worker 在 heartbeat 中上报(`judgedCases` 字段，可选)，服务端以 `GREATEST` 更新，乱序心跳不会让进度回退；
- 进度只是展示数据：非法值会被夹取而不是拒绝续租，不会因此丢 lease；
- rejudge 会把两个字段清零。

## 公开编号

页面使用短地址：`/problems/1000`、`/contests/42`、`/contests/42/problems/A`、`/submissions/123`、`/editorials/12`、`/problem-sets/3` 和 `/authoring/1000`。旧 UUID 页面链接仍可打开，加载后会规范化为数字地址。

相应 API 的资源路径参数接受 UUID 或公开编号；列表的 `problem` / `contest` 查询参数也兼容两者。比赛题目 API 的 `problemId` 还接受比赛内题号（如 `A`）。响应中的 `id` 及关联 `problemId` / `contestId` 仍是 UUID，`publicId` / `problemPublicId` / `contestPublicId` 用于生成链接。创建、更新请求体内的关联字段不改为公开编号。

编号现在按域与资源类型分配。当前无域前缀的资源 API 固定解析到官方域，已知另一域的 UUID 也不会越过 Store 作用域；query/body 不能选择或覆盖域。域中间件在身份认证后运行，对不可访问或停用的成员视角返回 404，归档域的资源写请求返回 403。`/api/domains/{domain}` 下目前对外注册的是域治理接口；资源前缀与协作权限将在 P2 后续一起开放，尚未注册的目标路径不是已上线接口。

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
- 答疑回复按 jury 能力检查；人员/协作管理按 owner 或域资源管理能力检查，普通 jury 不能授予角色。`GET/PUT .../access`、`DELETE .../access/{grant}` 保留用户/group 授权来源，`PUT .../owner` 转让给同域有效成员，`DELETE /api/contests/{id}` 删除比赛及比赛内记录但不删除引用题目。
- 管理列表 `/api/admin/contests` 已移除全站 admin 硬门槛，只返回当前用户可管理或参与协作的比赛；参赛资格通过 `admission` 和 participant 授权控制，报名与提交会在写事务中重新授权。
- 比赛进行中,选手读到的提交按 `contests.feedback` 屏蔽:`summary` 去掉测试点明细,
  `none` 把判定替换为 `Submitted`;比赛结束或裁判查看时恢复完整信息。
- 提交列表、详情和 progress 使用同一条数据库可见性谓词：比赛进行中、榜单隐藏或仍在封榜时，普通用户只能读取自己的提交，赛务人员可读全部；比赛结束且公开榜单已经解封后，其他读者仍只能看到该比赛本身会向其公开的题目。不可见行不会进入分页总数，按 UUID 访问也统一返回 `404`。
- `POST /api/admin/rejudgings` 的选择器不能为空,单批上限 5000 条,返回批次后进度可轮询。
  当前重测 API 仍保留站点 admin 门槛；比赛 jury 的完整重测工作流尚未接入，不能把能力字段当作该入口已开放的证明。

## 社区与后台接口

完整清单见[社区与后台管理](09-community-admin.md)。

- 题单、题解与公告的读接口都使用 optional auth：登录后才带上个人进度、草稿与「我是否点过赞」。
- `GET /api/editorials` 返回不含正文的 `EditorialSummaryResponse`；进入阅读页后再通过 `GET /api/editorials/{id}` 获取完整正文。
- 题解的 `solved_only` 屏蔽在服务层完成，被屏蔽时返回 `locked: true` 且详情 `contentMd` 为空，但标题等元信息保留。
- 题解及讨论在读取和创建时都会复核关联题目/比赛的可见性；不可见资源按 `404` 处理，防止通过猜测 UUID 绕过。
- 讨论编辑是**作者专属**的，管理员只能删除；删除父楼会级联删除其回复。
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

claim job 包含 generation、attempt、lease token/expiry、源码、资源限制以及测试数据路径、版本、哈希、用例数和 checker。heartbeat/result 必须回传 generation、worker ID 和 lease token。

构建协议的围栏规则与判题一致：lease 不匹配或过期返回 `409 build.stale_lease`，产物上传本身不发布数据，只有 `success=true` 的 result 才会更新 `problem_testdata`。

lease 不匹配、过期或 generation 已变化返回 `409 judge.stale_lease`。同一已完成 lease 的 result 重试返回幂等 `204`。result body 限制 16 MiB，case 数量最多 10,000。

## 健康检查和文档

- `/api/health/live`：进程 liveness，不访问数据库。
- `/api/health/ready`：readiness，至少检查数据库；容器 healthcheck 使用此端点。
- `/api/health`：兼容的 readiness 别名。
- `/swagger/index.html`：仅在 `SWAGGER_ENABLED=true` 时启用，生产默认关闭。
