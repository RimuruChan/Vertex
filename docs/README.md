# Vertex OJ 设计文档

本目录收录 Vertex Online Judge 的设计文档,随代码演进持续更新。

## 文档索引

| 文档 | 内容 |
|---|---|
| [设计概览](01-architecture.md) | 总体架构、技术选型与组件交互 |
| [判题沙箱设计](02-judge-sandbox.md) | 通用执行契约、隔离模型、资源限制与安全清单 |
| [数据库设计](03-database.md) | Schema 全览、关键索引与设计决策 |
| [API 设计](04-api.md) | REST 接口清单与数据结构 |
| [赛制与榜单设计](05-contest-rankboard.md) | ICPC/IOI/OI 计分、封榜、裁判权限、重测与答疑 |
| [部署与运维](06-deployment.md) | Docker Compose 部署、安全加固与验证流程 |
| [路线图](07-roadmap.md) | MVP 范围、v1+ 计划与接口预留 |
| [出题设计](08-problem-authoring.md) | 题目包、testlib 集成、构建流水线与题面渲染 |
| [社区与后台管理](09-community-admin.md) | 题单、题解防剧透与投票、讨论、站点后台 |
| [Sandbox 通用化计划](plans/2026-08-03-sandbox-generalization.md) | 交互、通信、数据生成与对拍的分阶段执行架构 |

## 快速导航

- **MVP 范围**:见[路线图](07-roadmap.md)
- **判题安全模型(最重要)**:见[判题沙箱设计](02-judge-sandbox.md)
- **部署上手**:见[部署与运维](06-deployment.md)
- **出题上手**:见[出题设计](08-problem-authoring.md)
