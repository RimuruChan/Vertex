# Vertex OJ — API 设计

机器可读规范是 Swag 生成的 Swagger/OpenAPI 2.0，以 `web/docs/swagger.json` 和 `/swagger/index.html` 为准；本文只记录跨接口约定和安全语义。

## 通用约定

- 公开 API 前缀 `/api`；内部 Judge API 前缀 `/internal/judge/v1`。
- 错误统一为 `{"code":"domain.reason","error":"兼容的人类可读信息"}`。
- 分页使用 `page`（从 1 开始）与 `size`（最大 100），列表统一返回 `ListResponse[T]`（`{items,total}`）。
- domain entity 不直接作为 HTTP body。
- 用户认证与 Judge service credential 在 OpenAPI 中使用不同 security definition。

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

## Judge 内部协议

三个接口都要求独立的 Judge bearer token：

| 方法 | 路径 | 成功响应 |
|---|---|---|
| POST | `/internal/judge/v1/jobs/claim` | `200` job 快照；无任务长轮询到期为 `204` |
| POST | `/internal/judge/v1/jobs/{jobId}/heartbeat` | `204` |
| PUT | `/internal/judge/v1/jobs/{jobId}/result` | `204` |

claim job 包含 generation、attempt、lease token/expiry、源码、资源限制以及测试数据路径、版本、哈希、用例数和 checker。heartbeat/result 必须回传 generation、worker ID 和 lease token。

lease 不匹配、过期或 generation 已变化返回 `409 judge.stale_lease`。同一已完成 lease 的 result 重试返回幂等 `204`。result body 限制 16 MiB，case 数量最多 10,000。

## 健康检查和文档

- `/api/health/live`：进程 liveness，不访问数据库。
- `/api/health/ready`：readiness，至少检查数据库；容器 healthcheck 使用此端点。
- `/api/health`：兼容的 readiness 别名。
- `/swagger/index.html`：仅在 `SWAGGER_ENABLED=true` 时启用，生产默认关闭。
