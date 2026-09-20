# 出题工作台验收记录

日期：2026-09-20。分支：`refactor/authoring-workbench`。依据：2026-09-19 调研及实施计划；以下记录对应当前工作树，未提交、推送或部署到正式环境。

## 需求与证据

| 计划范围 | 已实现的不变量 / 行为 | 验证入口与证据 |
| --- | --- | --- |
| A.1–2 内容和保存 | 稳定材料 ID、不可变 blob/tree；保存只更新个人副本，空保存不产生版本 | domain/content_tree_test.go、materials_test.go；真实 API AuthoringWorkbench 保存/历史断言 |
| A.3–4 提交和合并 | 显式提交单父共享历史；B/L/R 三方合并，字段/文本/顺序/改名/删除/二进制分开处理 | content_merge_test.go、text_merge_test.go；PG TestRevisionRepository 并发作者与提交 |
| A.5–6 冲突和恢复 | 持久化逐项解决；共享头再前进时重新合并；恢复历史进入副本 | PG revisions_test.go；第26–27轮真实双作者/多冲突浏览器验收，shared r6、published v1 |
| B.1–2 存储和身份 | 模块私有 sqlc；000001 baseline；个人副本/上传授权/私有检查隔离 | schema authoring_final_schema up/down/up；PG repository / API 权限测试 |
| B.3–4 原子性和读取 | etag、head、角色和域状态在写事务校验；幂等键不跨资源；大 blob 与分页读取分离 | revisions/checks/exchange/garbage/replica PG 回滚、撤权、去重、并发回收用例 |
| B.5–6 API | 公开题号与域作用域；UUID 不作为浏览器别名；生成客户端同步 | 全量 server tests 包含 references/http/privacy；358 个生成文件重复生成一致 |
| C.1–3 布局和题面 | 真实子路径、桌面侧栏/手机覆盖抽屉、统一间距与按钮；双栏预览及全屏编辑 | 390/768/1440 实测无横向溢出；亮暗主题截图；全屏进入/退出原文不变 |
| C.4 材料管理 | 程序树/角色/预期判定；测试表、分组、上传、批量样例/限制/顺序；重复数据提示 | 真实浏览器批量1700ms/131072KiB；API failed batch 不部分保存；领域重复提示回归 |
| C.5 差异/历史 | 按项差异、字段/行比较、下载前后内容、历史恢复、可继续的冲突会话 | line-diff tests、有界超大差异；真实两份冲突草稿互不覆盖及头推进验收 |
| C.6–7 检查/发布和反馈 | 私人/提交检查选择、局部轮询、矩阵、自测、PDF 预览、显式发布、就近错误与连续保存状态 | 真实UI新增自测→保存→检查→结果；原生发布/固定版本 E2E；浏览器保存/重试/文件替换 |
| C.8 mock | 少量 A+B 代表性材料、多人副本、检查/冲突/发布；明确不执行程序；真实 ZIP 文件 | 47 个前端测试文件 / 216 tests；原生和无 config 的平铺 ZIP mock 往返 |
| D.1–2 冻结和复用 | 源 tree、语义数据摘要、检查策略和实际工具链绑定；旧 Worker 不能领取不支持策略 | FrozenCheckRepository；真实 FrozenAuthoringCheck 排队后改输入仍验证旧快照 |
| D.3–4 判题语义 | 保留导入答案；多解比较；Kattis 42/43、testlib 和默认 token/精确比较；异常不算 WA | 原生 Builder；Kattis/testlib/exact 外部互操作；实际 `03` 答案与多解/原版判题 |
| D.5 质量检查 | 校验器输入负例及输出正反例，参考解逐点矩阵；崩溃/超限/缺少结果不能发布 | validation / checker tests；原生 always-accept、crash、错误预期；PG publication gate |
| D.6 发布与租约 | 不可变产物、检查后显式发布、过期租约和幂等完成、取消/重试/回滚 | 真实 PG、DomainProtocol、独立 HTTP 集成 JudgeFencing；native 主E2E和重连恢复 |
| D.7 固定版本和隐私 | 比赛、练习、复制与重测读取指定发布；私人材料与内部日志不进入选手 JSON | 完整真实 E2E 的 ContestResponsePrivacy / FrozenAuthoringCheck / PublishedFiles；源题删除后副本仍可用 |
| E.1–2 格式和归档 | 有界 ZIP/YAML/XML、路径/链接/重复/膨胀验证；Vertex、ICPC/Kattis、DOMjudge、Polygon、平铺数据 | packages 测试组，native exact-tree 往返，格式矩阵与未知语义阻断 |
| E.3–5 映射和事务 | 数据、题面、公开附件、多文件程序、验证器、自测；预检绑定人/副本/归档；导出固定内容 | PG PackageImportTransactions；标准包→真实Worker→发布→再导出；二进制平铺ZIP与逐点限制回归 |
| E.6 题面 | 原文保留；legacy MD→TeX；标准章节/样例指令、中文字体、PDF 可审阅；限制语义准确报告 | 原生 RenderNative；PyMuPDF 校验实际标题/位置/次数/字面数据/无秘密测试；公开样例下载实测 |
| E.7 外部验证 | 固定 problemtools 镜像，CC0 自建夹具，固定 testlib 版本/许可 | 当前 legacy、Markdown、Polygon、testlib、exact 五组均0 errors / 0 warnings |
| F 清理与交付 | 旧共享编辑表/写API/UI已移除；模型图/文档更新；原始JSON权限、生成、构建、浏览器验收 | 当前 routes/schema + 全量Go/PG/Worker测试、vet、前端构建/格式检查及本记录 |

## 本轮主要日志

日志在任务本地忽略目录 `.cache`，自动化测试源码和 CI 配置随仓库维护。

- `authoring-polish-all.log`：最新 Server 全量真实 PostgreSQL 回归。
- `authoring-final-worker.log`、`authoring-final-*-vet.log`：Worker 全量普通测试、两侧 vet。
- `authoring-standard-native.log`、`authoring-final-smoke.log`：原生 TeX、取消与只读无网络 sandbox 验证。
- `authoring-final-e2e.log`：当前主原生 E2E 套件；与 CI 一致排除需要独立运行的协议场景。
- `authoring-final-recovery.log`、`authoring-final-protocol.log`：实际服务重连及独立协议场景。
- `authoring-final-api-integration.log`、`authoring-polish-api.log`：独立 PostgreSQL + 内存 HTTP 路由，含 JudgeFencing、批量原子性、题面和公开文件。
- `authoring-final-interop.log`、`authoring-final-package.log`：固定外部验证器及真实标准包执行往返。
- `authoring-polish-ui-tests.log`、`authoring-polish-ui-build.log`、`authoring-polish-mock-build.log`、`authoring-polish-format.log`：216 tests、生产/mock构建、Prettier。
- `authoring-final-schema.log`：新建任务库 baseline up/down/up。
- 生成内容按358个sqlc/OpenAPI/客户端文件计算SHA-256，重复生成后无变化；Windows瞬态文件占用由已有生成脚本重试成功。
- 真实浏览器下载 `problem-data.zip`，核对4个数据文件与config.yml，两个时间/内存覆盖均正确。

## 能力边界

本次目标为可实际使用的传统批处理题出题闭环，不等于实现所有外部平台或完整2025-09认证。交互/multi-pass/static grader/特殊计分、未映射常量/逐点校验参数/附件模板、未知构建脚本等保留源材料并明确阻断错误执行或发布。原生归档可保存全部材料；标准导出不能表达时明确失败。平铺数据包是数据专用传输，不携带完整评测语义。Polygon支持常用已生成包，不能宣称任意包无损导出。PDF完整语义由实际解析预览校验，不以文件头冒充内容认证。

额外直接针对运行服务的旧租约E2E命令曾被自动审批拒绝，仅给出“blocked by policy”；没有重试该运行方式。发现其停Worker后准备夹具仍等待Worker的缺陷并修复，改在独立测试数据库的HTTP集成环境验证，同一JudgeFencing用例通过。此结果是协议验证，不是假称进行了原生程序执行。

未动用户现有数据库，未修改线上环境，未自动提交/推送/合并。测试数据和运行容器均属于本任务。
