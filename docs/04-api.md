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

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/register` | 创建用户和 revocable session |
| POST | `/api/auth/login` | 创建 session |
| POST | `/api/auth/refresh` | 使用 HttpOnly cookie 原子轮换 refresh token |
| POST | `/api/auth/logout` | 吊销当前 `sid` 并清 cookie |
| POST | `/api/auth/logout-all` | 吊销用户全部 session |
| GET | `/api/auth/me` | 返回数据库中的当前用户与角色 |

认证响应包含 `accessToken/expiresIn/user`；`token` 暂时作为 deprecated 兼容别名。refresh token 不出现在 JSON body。
注册和登录 JSON 限制为 16 KiB，密码限制为 6–72 字节；不存在的用户名仍执行一次 bcrypt 比较，避免通过响应耗时枚举账号。
登录同时按规范化账号和 TCP peer 限流，注册按 TCP peer 限流；比赛密码按用户/比赛限流。超过配额统一返回 `429 request.rate_limited`。应用不信任客户端伪造的 `X-Forwarded-For`，反代与多实例部署的真实客户端配额由可信 ingress 补充。

## 查看者相关的读接口

以下接口对匿名与已登录用户返回不同内容，前端必须在 access token 恢复之后再请求，否则会拿到匿名结果。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/problems` | 可选认证。带 token 时每道题返回 `userStatus`(`solved`/`attempted`/`none`)，并支持 `status` 过滤 |
| GET | `/api/problems/{id}` | 可选认证，同样返回 `userStatus` |
| GET | `/api/tags` | 公开题库的标签目录，按题目数量倒序 |
| GET | `/api/users/{username}` | 个人主页聚合：通过数、提交数、按难度的通过进度、最近 90 天提交热力图 |

`userStatus` 只由 `contest_id IS NULL` 的练习提交推导，不做冗余存储，因此 rejudge 与比赛重算不会泄漏封榜或隐藏反馈。
匿名请求一律返回 `none`。

## 判题进度

`submissions` 上的 `judgedCases` / `totalCases` 用于前端显示「已评测 3 / 10 个测试点」：

- 前端通过 `GET /api/submissions/{id}/progress` 轮询轻量读模型；该接口与提交详情使用相同的查看者权限，不重复返回源码、用户或题目信息；
- `totalCases` 在 job 被 claim 时按测试数据用例数写入；
- `judgedCases` 由 worker 在 heartbeat 中上报(`judgedCases` 字段，可选)，服务端以 `GREATEST` 更新，乱序心跳不会让进度回退；
- 进度只是展示数据：非法值会被夹取而不是拒绝续租，不会因此丢 lease；
- rejudge 会把两个字段清零。

## 出题接口

出题接口全部在 `/api/admin/problems/{id}` 下，要求 admin。完整清单与语义见[出题设计](08-problem-authoring.md)。

- `GET /api/admin/problems/{id}/package` 一次返回题面、源文件（不含正文）、测试点、最近一次构建和「还不能构建的原因」。
- 源文件按 `(kind, name)` upsert；checker/validator/interactor 限定 C++，保存即设为启用项。
- 测试点的生成命令不经过 shell，服务端与 worker 各自独立校验 argv token。
- `POST .../builds` 在包不完整时返回 `400 authoring.not_buildable`，已有构建在跑时返回 `409` 并带上正在跑的那一个。

## 比赛与赛务接口

完整语义见[赛制与榜单设计](05-contest-rankboard.md)。

- `GET /api/contests/{id}/rankboard` 默认返回按封榜规则裁剪的视图;`?view=jury` 只对裁判/观察员生效,
  其他调用者拿到的仍是封榜视图——服务端决定给哪一套数值,客户端无法绕过。
- `GET /api/contests/{id}` 额外返回 `staffRole`,前端据此显示裁判台入口。
- 比赛中的题面通过 `GET /api/contests/{id}/problems/{problemId}` 读取；该接口同时验证比赛、题目归属、时段和参赛/赛务身份，非公开比赛题不经过通用 Problem API。
- 答疑与人员管理挂在 `/api/contests/{id}` 下并按**比赛角色**鉴权,不要求系统管理员。
- 比赛进行中,选手读到的提交按 `contests.feedback` 屏蔽:`summary` 去掉测试点明细,
  `none` 把判定替换为 `Submitted`;比赛结束或裁判查看时恢复完整信息。
- 提交列表、详情和 progress 使用同一条数据库可见性谓词：比赛进行中、榜单隐藏或仍在封榜时，普通用户只能读取自己的提交，赛务人员可读全部；比赛结束且公开榜单已经解封后，其他读者仍只能看到该比赛本身会向其公开的题目。不可见行不会进入分页总数，按 UUID 访问也统一返回 `404`。
- `POST /api/admin/rejudgings` 的选择器不能为空,单批上限 5000 条,返回批次后进度可轮询。

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

| 方法 | 路径 | 成功响应 |
|---|---|---|
| POST | `/internal/judge/v1/jobs/claim` | `200` job 快照；无任务长轮询到期为 `204` |
| POST | `/internal/judge/v1/jobs/{jobId}/heartbeat` | `204` |
| PUT | `/internal/judge/v1/jobs/{jobId}/result` | `204` |
| POST | `/internal/judge/v1/builds/claim` | `200` 完整题目包；无任务为 `204` |
| POST | `/internal/judge/v1/builds/{buildId}/progress` | `204`（同时续租） |
| POST | `/internal/judge/v1/builds/{buildId}/package` | `200` 产物元信息（octet-stream 上传，lease 走请求头） |
| PUT | `/internal/judge/v1/builds/{buildId}/result` | `204` |

claim job 包含 generation、attempt、lease token/expiry、源码、资源限制以及测试数据路径、版本、哈希、用例数和 checker。heartbeat/result 必须回传 generation、worker ID 和 lease token。

构建协议的围栏规则与判题一致：lease 不匹配或过期返回 `409 build.stale_lease`，产物上传本身不发布数据，只有 `success=true` 的 result 才会更新 `problem_testdata`。

lease 不匹配、过期或 generation 已变化返回 `409 judge.stale_lease`。同一已完成 lease 的 result 重试返回幂等 `204`。result body 限制 16 MiB，case 数量最多 10,000。

## 健康检查和文档

- `/api/health/live`：进程 liveness，不访问数据库。
- `/api/health/ready`：readiness，至少检查数据库；容器 healthcheck 使用此端点。
- `/api/health`：兼容的 readiness 别名。
- `/swagger/index.html`：仅在 `SWAGGER_ENABLED=true` 时启用，生产默认关闭。
