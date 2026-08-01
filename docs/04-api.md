# Vertex OJ — API 设计

> REST,前缀 `/api`。认证用 `Authorization: Bearer <JWT>`。响应统一 `{ "error": "..." }` 或业务对象。

## 约定

- 分页参数:`page`(1-based,默认 1)、`size`(默认 20,最大 100)。列表响应 `{ "items": [...], "total": N }`。
- 时间戳:ISO 8601(RFC3339)。
- ID:UUID。

## 认证

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| POST | /api/auth/register | 公开 | `{username, email, password}` → `{token, user}` |
| POST | /api/auth/login | 公开 | `{username, password}` → `{token, user}` |
| GET | /api/auth/me | 登录 | 当前用户 |

## 题目

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | /api/problems | 公开 | 仅 public;筛选项 `difficulty`/`tag`/`keyword`;列表不返回题面 |
| GET | /api/problems/:id | 公开 | 仅 public;admin 可看 private/draft |
| POST | /api/admin/problems | admin | 创建(默认 draft) |
| PUT | /api/admin/problems/:id | admin | 全量更新 |
| DELETE | /api/admin/problems/:id | admin | 删除(含测试数据文件) |
| GET | /api/admin/problems | admin | 管理员视角列表(含草稿) |
| GET | /api/admin/problems/:id | admin | 详情 |
| POST | /api/admin/problems/:id/testdata | admin | multipart 上传 zip(1.in/1.out...) |

## 提交

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| POST | /api/submissions | 登录 | `{problemId, language, sourceCode, contestId?}` → 202 + 提交行;比赛提交校验时间窗、报名与题集;限流 10 次/分钟 |
| GET | /api/submissions | 登录 | 筛选项 `user`/`problem`/`contest`/`language`/`status` |
| GET | /api/submissions/:id | 登录 | 详情(逐测试点);非本人/非 admin 隐藏源码 |
| POST | /api/admin/submissions/:id/rejudge | admin | 重置回 Pending 重新判定 |

`language` 支持:`c`、`cpp`、`python`。

## 比赛

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | /api/contests | 公开 | 列表 |
| GET | /api/contests/:id | 公开 | 详情 + 题目集 |
| GET | /api/contests/:id/rankboard | 公开 | 榜单;`?frozen=true/false` 覆盖封榜判断 |
| POST | /api/contests/:id/register | 登录 | 报名(开始前) |
| POST | /api/admin/contests | admin | 创建 |
| PUT | /api/admin/contests/:id/problems | admin | 重建题目集 |

## 题解 / 评论

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | /api/editorials?problem=:id | 公开 | 题解列表 |
| GET | /api/editorials/:id | 公开 | 题解详情 |
| POST | /api/editorials | 登录 | 发布题解 |
| GET | /api/problems/:id/discussions | 公开 | 题目评论 |
| POST | /api/problems/:id/discussions | 登录 | 发表评论(可回复,`parentId`) |
| GET | /api/editorials/:id/discussions | 公开 | 题解评论 |
| POST | /api/editorials/:id/discussions | 登录 | 发表题解评论 |
| DELETE | /api/discussions/:id | 作者或 admin | 删除评论 |

## 判题状态枚举

`Pending` / `Judging` / `Accepted` / `Wrong Answer` / `Time Limit Exceeded` / `Memory Limit Exceeded` / `Runtime Error` / `Compile Error` / `Output Limit Exceeded` / `System Error` / `Skipped`

## 榜单响应结构(ACM)

```json
{
  "problemCount": 2,
  "problemIds": ["<uuid>", "<uuid>"],
  "frozen": false,
  "rows": [
    {
      "rank": 1,
      "username": "alice",
      "userId": "<uuid>",
      "solved": 1,
      "penalty": 1200,
      "cells": [
        { "attempts": 2, "penaltySec": 1200, "solvedAt": "2026-08-01T10:00:00Z", "pendingCount": 0 },
        { "attempts": 0, "penaltySec": 0, "solvedAt": null, "pendingCount": 0 }
      ]
    }
  ]
}
```

前端依据 `cells[i]` 渲染:`solvedAt` 有值 → 绿(`penaltySec/60` 分钟);`attempts>0` → 红(`-attempts`);`pendingCount>0` → `?`(封榜中)。
