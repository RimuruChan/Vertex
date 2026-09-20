# 出题工作台重构实施计划

设计依据：[调研与产品设计](../research/2026-09-19-authoring-redesign.md)。
分支：`refactor/authoring-workbench`，基线 `04ef7d3`。
用户要求：完整调研后创建 goal 持续实现；允许开发期破坏性重构，沿用既有做题/比赛 UI 风格及后端权限设计。

## 完成定义

交付可实际使用的出题闭环：私人工作副本 → 显式提交 → 更新与冲突合并 → 检查 → 不可变发布 → 比赛固定引用，以及范围明确的 ICPC/Kattis 批处理题包导入/导出。前后端、mock、生成客户端、文档和关键自动化验证同时完成。

不能用下面任意一项代替完成：仅换文案、仅加一个历史表、仅加 mock、只解压 ZIP、导入时丢弃未知判题语义、仅验证按钮隐藏、只有空的接口/未接入的领域代码。高级格式能力未实现必须在产品和文档中准确报告，不伪称全面兼容。

## 进度

- [x] 读取现有模型/查询/Worker 与前端路径，确认实际共享副本和版本计数问题。
- [x] 查证 Polygon、洛谷、ICPC/Kattis、DOMjudge 与验证工具的官方材料。
- [x] 本地启动 mock 并检查现有出题入口和工作台实际布局。
- [x] 写出模型、冲突策略、UI、兼容矩阵、验收基线。
- [x] A. 内容树、工作副本、提交与三方合并（领域/PG回归及第26–27轮真实浏览器验证）。
- [x] B. PostgreSQL 持久化、授权和 API 闭环（真实PG/API、撤权/回滚、公开文件权限与生成一致性已验证）。
- [x] C. 编辑器、导航、更改/冲突/历史 UI 与 mock。
- [x] D. 固定输入的检查、产物、发布与 Worker 对接。
- [x] E. 题包适配、导入预检、导出与互操作验证。
- [x] F. 删除旧工作流、生成一致性、真实端到端与界面验收。

## A. 领域语义

实现位置：`server/internal/modules/authoring/domain`。

1. 稳定条目标识与内容树；元信息、题面、程序、数据、附件的用途与协议显式建模。
2. 对内容而非保存动作计算变化；重复保存不递增产品版本。
3. Commit 指向不可变 tree，单父共享主线；WorkingCopy 保存 base、tree、etag。
4. Merge 使用 base/local/head，字段、条目、文本、二进制与排序分类；先保证不丢数据，再增加非重叠文本自动合并。
5. Conflict 含可供 UI 定位的类型和路径；解决结果重新走材料完整性验证。
6. 恢复历史进入工作副本，既不改写历史也不发布。

验收：同改/异改、并行新增、删除-修改、改名-修改、双重改名、路径冲突、空文件与删除区别、二进制冲突、排序冲突、确定性摘要与无输入别名修改。

## B. 存储、事务和 API

实现位置：authoring 的 `application`、`infrastructure/postgres`、`infrastructure/filesystem`、`transport/http`，以及 `server/migrations/000001_init.*`。

1. 合并基础 schema，增加 head/copies/commits/trees/blobs/merges；复用现有资源身份、权限模型和模块私有 sqlc。
2. 新建/读取/保存自己的副本；共享提交可读，私人副本不能跨用户列出。
3. 提交、更新与合并完成都在事务内校验 etag、头提交、域与权限；幂等请求键不能跨用户/题复用。
4. history/compare 按页与按项读取；blob 下载从资源引用授权进入。
5. 新 API 使用域内公开题号；内部 UUID 不作为浏览器资源别名。
6. 生成 OpenAPI、客户端；业务查询明确命名，不产生 `Column1`/数字后缀占位字段。

建议资源组织（最终以生成 API 为准）：

```text
/api/domains/{domain}/authoring/problems/{number}
  /working-copy                 GET / PUT
  /working-copy/update          POST
  /working-copy/discard         POST
  /working-copy/restore         POST
  /changes                      GET
  /commits                      GET / POST
  /commits/{revision}           GET
  /merges/{id}                  GET / PUT
  /merges/{id}/complete         POST
  /checks                      GET / POST
  /checks/{id}                  GET
  /imports                     POST
  /imports/{id}/apply           POST
  /exports                     POST
  /releases                    GET / POST
```

检查/导入/合并 ID 是任务身份，可使用 UUID；题目仍为公开编号。文件路径作为经过规范化的条目属性，不能直接拼接到主机路径。

PostgreSQL 验收：两份副本同时提交、双标签保存、提交失败回滚、共享头变化后完成旧合并、权限撤销/域归档同时写入、跨域编号碰撞、blob 猜测、账号切换、幂等重试。

## C. 产品界面

实现位置：替换 `ui/src/pages/admin/ProblemWorkspacePage.tsx` 与 workspace panels，并更新 `RootRoutes`、App 布局匹配、领域 API 和 mock。

1. 单题路径导航与桌面侧栏/手机覆盖抽屉，复用现有站点容器/按钮/表单/动效。
2. 紧凑标题与工作状态，不再展示保存计数为“包版本”。
3. 题面双栏编辑预览；窄屏切换；语言、样例与原格式附件独立处理。
4. 程序树、角色和预期判定；数据表格与组编辑，批量导入/操作。
5. 差异与提交说明、更新提示、可恢复三方冲突页面、历史比较/恢复。
6. 检查来源、报告矩阵、发布卡；局部更新保留编辑状态。
7. 保存 loading → success 动效没有空隙，字段错误就近，按钮高度/轮廓一致。
8. 用少量代表性 mock 支持两名协作者、冲突、失败检查和标准包数据，不铺满排列组合。

浏览器验收：亮/暗、约 390/768/1440 像素、键盘/焦点、长标题、滚动、保存失败、后退前进/直接访问子路由、刷新恢复副本、切域切人、请求乱序。

## D. 构建、验证与发布

1. 把现有 build input 固定到 tree/内容摘要，历史构建不再依赖可变最新材料。
2. 数据/工具链/检查策略指纹决定复用，不能把旧通过状态带到新数据上。
3. 支持保留导入答案以及标程生成答案；参考解验证遵守 checker 多解语义。
4. 支持明确的 testlib 与 Kattis 校验器协议；Kattis 默认 token/大小写/空白/浮点策略不能复用当前简化 diff。
5. 引入校验器正反例、参考解预期结果矩阵和详细阶段报告。
6. 产物不可变，保留过期 Worker 防护；发布绑定 commit + artifact + check，事务更新公开投影。
7. 比赛编排、练习、复制、重测读取发布快照；作者未提交内容绝不能进入公开与比赛。

Worker/E2E 验收：42/43 与异常退出区分、输入拒绝、WA/TLE 参考解、checker 自身崩溃、实际旧版本判题、过期租约回写、上传后事务失败、并发同摘要落盘、取消与重试。

## E. 题包导入导出

1. 独立的受限归档读取器、格式识别和结构化诊断，不复用内部扁平 build artifact 协议假装外部标准包。
2. Vertex 原生、legacy-icpc、Kattis legacy/2025-09 常用批处理路径；DOMjudge 扩展；Polygon 常用已生成包与洛谷数据入口。
3. 输入/答案、题面/图片/附件、多文件程序、checker/validator、参考解、样例/秘密数据和支持的组规则双向映射。
4. 预检 token 绑定 archive hash、副本 etag、格式和用户；确认原子应用。
5. 导出固定 tree，确定性归档；缺必需材料或含不支持语义就生成准确兼容报告，不能静默降级。
6. 标准包保留原题面格式。legacy 导出满足 TeX/PDF 要求，转换在受限环境；2025-09 允许按规范导出 MD。
7. 测试夹具自建/明确许可并记录来源；固定外部验证器版本，实际验证导出包，而非只跑自己的 parser。

互操作验收：导出→导入后语义一致；`.ans`、嵌套根目录、多语言、内存/时限单位、默认比较、validator 退出码、秘密材料权限；未知格式/题型/字段的诊断；ZIP 越界/链接/重复路径/膨胀/部分失败；包内容不能改变资源权限或身份。

## F. 收尾与验证

- 去掉旧共享工作区材料写路径和自动候选版本 UI，删除不再适用的生成 API、mock、测试与文档。
- `docs/08-problem-authoring.md` 重写为真实实现说明，`docs/15-backend-models.md` 更新关系图。
- Go 领域/应用测试、真实 PostgreSQL、Worker、前端行为测试、生产/mock 构建、生成可重复性和格式检查。
- 原始 JSON 权限断言：公开页/选手/reader/editor/owner/跨域访问；秘密文件、参考解、草稿、内部日志均覆盖。
- 浏览器实际走新建/导入、编辑、提交、两人合并、检查、发布、比赛固定版本完整闭环。
- 不运行无关重复全量验证；先定向检查，集成里程碑再跑全量。
- 未经本轮明确要求，不自动发布到生产、合并或重置用户数据库；测试使用独立数据库与可清理容器。

## 当前交接记录

2026-09-19 第一轮实现：

- 已新增不可变内容清单、稳定条目标识、内容 hash、跨平台路径校验与流式 blob 存储。相同内容并发上传不会覆盖原文件；二进制输入/答案不自动文本合并。
- 已新增字段/条目/有界文本三方合并、结构化 JSON 合并和历史差异领域函数，覆盖删除-修改、改名-修改、路径碰撞、空文件/删除、排序数组冲突等。
- `000001_init` 已加入私人副本、共享 head/commit、tree/blob 引用和可恢复 merge session；SQL 经 sqlc 生成。现有共享工作区暂时保留给尚未切换的旧编辑器，**最终必须删除旧写路径**。
- 新服务已接入 `/api/domains/{domain}/authoring/problems/{id}`：开启/读取/保存副本、文本编辑、提交/历史/历史内容、更新/丢弃/恢复、保存/完成冲突、multipart 文件上传和受权限控制下载。OpenAPI 与前端客户端已生成。
- 重复保存不移动 etag，不产生提交；提交重试幂等；陈旧 token 被拒绝。完成冲突时如 head 又变化，保留解决结果并继续三方合并，而非丢弃已有工作。
- 已通过领域与文件系统测试、authoring Go 测试与 vet、服务编译、TypeScript 检查；**真实 PostgreSQL** 验证同时提交、同账号双页面保存、20 次 no-op、历史恢复、私有 blob 防猜测、撤销权限与提交中途失败回滚/重试。
- 已通过 `TestDomainAPIIntegration/AuthoringWorkbench`：真实认证/域中间件、公开编号、数据库和文件存储，直接断言 HTTP JSON/文件字节的私人草稿隔离、reader/editor 权限、跨域拒绝及提交不发布。

2026-09-19 第二轮实现：

- 新增 `PackageMetadata`、`ProgramMaterial`、`TestMaterial`、`GroupMaterial` 文档结构：程序可关联多个文件，生成器参数使用数组，测试输入与答案分别声明文件/程序来源，分组有聚合与依赖；本地字段校验不阻止保存尚未补齐引用的草稿。
- 首次开启副本时初始化真实元信息和默认语言题面。head 保存不可变 `initial_tree_hash`，让首次提交前的多个协作者也有共同基线；没有更改初始材料时也能显式提交 r1。
- 新增材料创建/编辑/删除、按副本或历史版本读取结构化材料、工作副本与任意历史提交的差异 API。文本请求不再要求客户端伪造 blob 哈希。OpenAPI 与前端类型已同步。
- `RootRoutes` 已切换到新 `pages/authoring/WorkbenchPage.tsx`，使用 `/authoring/:id/:section?`；原 `pages/admin/ProblemWorkspacePage.tsx` 和旧 panels 仍待最终清理，后端旧构建/发布暂未切换。
- 新 UI 已接个人副本的题面编辑/预览、代码编辑、元信息/程序/测试/分组表单、材料文件上传、显式提交、差异/历史/恢复、选择双方内容解决冲突、协作权限。桌面侧栏与窄屏覆盖抽屉共用导航，文件重命名折叠到次级操作。二进制/非 UTF-8 数据不进入文本编辑器，避免仅改路径时损坏数据。
- mock 新增独立副本/共享提交/私有 blob 状态，持久化在原 mock 存储中；有初始材料、无变更保存、过期 token、幂等提交和私有 blob 防引用回归。mock 当前保守地按条目产生冲突，尚未模拟后端的非重叠行自动合并。
- 新 schema 在独立 `authoring_materials` 数据库通过 `TestRevisionRepository` 和 `TestDomainAPIIntegration/AuthoringWorkbench`，后者新增初始文档、结构化程序往返、非法材料拒绝、差异与历史材料审阅检查。Go 定向测试、前端 TypeScript、相关 mock 测试、`pnpm build:mock` 通过。
- Browser 实查约 1272px 桌面与 390px 手机宽度，走过编辑 → 保存中 → 已保存 → 显式提交 r1；确认子路由变化和侧栏覆盖/关闭。发现并修正手机标题被按钮挤成竖排的问题，手机标题现独占一行。临时 viewport 已 reset。尚未完整验证亮色与 768px、真实后端浏览器闭环。

2026-09-19 第三轮实现：

- `domain/inspection.go` 提供有界材料引用检查与 `CheckSnapshot`：元信息、题面语言、程序文件/入口/用途、主参考解、验证器协议、测试来源/答案、顺序、分组依赖和循环均有定位到材料的诊断。检查不执行程序、不读取大测试数据。`CanBuild` 仅表示结构检查无阻断，**不是运行检查通过**。
- 完整 tree hash 与数据指纹分开：题面说明/来源/标题变化保留数据指纹；程序、文件布局、参数、限制、比较策略、输入/答案和组规则变化使指纹失效。新增明确 `ProgramMaterial.directory`，程序文件必须在该目录下；禁止根据共同前缀猜工作目录而改变相对路径语义。
- 新增 inspection、checks 启动/列表/详情/取消 API，以及按活动租约读取冻结内容的内部下载端点。任务直接扩展 `problem_build_jobs` 的 `source_tree_hash/source_revision/data_hash/check_policy/toolchain_key`，并通过 FK 与不可变触发器绑定输入。新检查完成不会写旧 `problem_candidates`；通过状态要求有效工具链指纹。
- 私人检查只供本人读取；当完全相同的 tree 被显式提交时，其检查可供协作者审阅，`matchingRevision` 描述此关联而不修改原始来源。仅数据指纹相同不会触发共享。旧 builds/工作区读取端点排除新检查，防止从旧路径泄漏私人报告。
- 新任务需要 Worker 在 claim 中声明 `checkProtocol: "vertex-authoring-1"`。**当前 Worker client 尚未声明，也尚未消费 `check` 快照；新任务会保持 queued，不能把数据库模拟领取/完成测试当作真实构建已打通。** 下一轮先实现 Worker 快照执行，再打开能力声明。
- Worker 已新增 `checker.Policy` 的 exact/token/ASCII 大小写与空白/绝对和相对误差比较；显式 `floatingPoint` 区分禁用与零容差。新增 `CheckProgram` 的 Kattis/testlib 调用与退出码处理、输入 validator 成功码区分、独立 `JudgeMessage` 和 team message。新 helper 尚未接入新快照执行循环；真实 Linux 沙箱运行仍待验收。参考 [Kattis 协议](https://icpc.io/problem-package-format/spec/legacy.html) 与 [官方 default validator](https://raw.githubusercontent.com/Kattis/problemtools/master/support/default_validator/default_validator.cc)。
- 新增测试点时，描述符与元信息的顺序在同一次副本保存中更新；删除同理。mock 同步这一行为。比较设置 UI 增加显式浮点开关，程序表单增加目录设置。
- 并发检查可能共用同一产物目录，因此 `UploadPackage` 不再在租约/数据库写入失败后立即删除新落盘对象，避免误删另一个有效任务的引用；需在后续实现引用感知且带宽限期的回收。相关回归改为验证保留共享内容。
- 真实 PostgreSQL `TestFrozenCheckRepository` 验证：重复排队复用、私人报告隔离、旧入口不泄漏、能力未声明时不领取、新旧租约下载隔离、修改后仍读原输入、冻结输入不可更新、工具链缺失不能成功、共享内容复用及取消。最新基线在 `authoring_checks_sharing` 通过该测试、`TestRevisionRepository` 和 HTTP `TestDomainAPIIntegration/AuthoringWorkbench`。临时数据库也完成 baseline down/up；没有动用户数据库。
- Worker 的 Windows 可运行 Go 测试、authoring 定向测试与 vet、相关前端测试和类型检查通过；生成客户端已更新。这不覆盖原生沙箱、所有竞态、最终发布和题包互操作。

2026-09-19 第四轮实现：

- Worker 已消费 `check` 快照并声明新协议。领取后通过带 worker/lease header 的内部内容端点下载材料，核验长度与 SHA-256，按固定快照编译、生成、校验、生成或保留答案、检查全部参考解，再上传产物和回写工具链指纹。
- `run/inputs.go` 使用 Go 1.25 的 `os.Root` 约束嵌套输入写入，防止父目录 symlink 越界；可执行权限通过已打开文件设置。stdin/产物导出继续保持单文件名限制，不放宽 native C++ 路径契约。Linux 上已实际验证嵌套路径与外部链接拒绝。
- `compile.CompileFiles` 支持多文件 C/C++ 编译、头文件和 Python 同伴文件，保留显式程序目录布局；缓存包含全部源文件路径/内容、编译参数和实际工具链版本。检查工具链指纹还包含 native sandbox 二进制摘要、testlib 摘要、语言配置和阶段限制。
- 新执行器运行所有配置的输入验证器，区分 Kattis 42 与 testlib/stdio 0；Kattis 输出校验器使用 stdin 和 feedback 目录。导入答案保持原字节，并用参考解与 checker 核对；WA 参考解不能掩盖后续测试的 TLE。报告增加逐点参考解结果矩阵、浮点分值和样例预览截断标记；截断预览不能在后续发布中当作完整样例。
- 新内部 `artifact.json` 产物包含冻结快照、工具链指纹、实际数据摘要、程序源文件和固定 testlib 依赖。Server 验证文件清单、大小/摘要、测试顺序和数据指纹；上传记录再与领取时的完整快照比较，完成时校验产物与报告工具链一致。递归目录摘要已覆盖嵌套源码。外部 Kattis/Polygon 导入导出仍未实现，不能把此内部产物当作交换题包。
- `TestFrozenBuilderNative` 在真实 Linux sandbox 中通过：嵌套头文件、独立翻译单元、Python Kattis 输入/输出验证器、正确与 WA 参考解、保留 `03\n` 导入答案及逐点报告。`TestRootedInputs` / `TestRootedInputRejectsEscapingParentLink` 在 Linux 中通过；`TestEnvironmentLifecycle` 原生回归通过。
- **真实 Server + Worker + PostgreSQL + sandbox 的 `TestEndToEndFrozenAuthoringCheck` 已通过**：从 HTTP 保存材料开始，排队后修改输入，检查仍用旧输入，结果成功上传并校验，保留原答案，私人报告对 reader 返回 404，检查完成没有自动发布。通过环境变量 `VERTEX_FROZEN_CHECK_E2E=1` 显式启用，API-only suite 不把它伪装为执行测试。
- 这次执行使用本任务的 `authoring_runtime` 数据库、专用 Server/Worker 容器与专用数据卷；验证后已移除运行容器及数据卷，未触及用户现有卷。测试二进制与构建日志留在被忽略的 `.cache`。Windows 上 Worker Go 测试、相关 vet、Server authoring 测试、TypeScript 检查通过，OpenAPI/客户端已生成。

2026-09-19 第五轮实现：

- 新增提交发布 API `/authoring/problems/:id/releases`，请求固定 revision、checkId、expectedVersion 和语言。事务重查 Publish 权限、结构检查、检查数据指纹/策略/工具链及产物对应关系，幂等重试返回同一版本；过期发布拒绝。`problem_versions` 增加提交/tree/check/toolchain 绑定及 FK，仍原子更新公开投影，比赛不自动采用新版本。
- 发布从已提交题面 blob 和实际产物中的样例读取完整数据，不使用检查报告的截断预览；元信息新增版本化 difficulty。**当前发布入口只处理 Markdown，单份题面/样例限制 1 MiB，样例总计 4 MiB；TeX/PDF 与大/二进制样例仍需转换或下载式展示，不能视作已满足完整格式兼容。**
- Worker 共享 `internal/artifact` 合约，评测前核验整个目录摘要（保护 manifest 比较语义）和文件摘要。新 `artifact` checker 类型支持 exact/token 策略，以及使用包内源码和固定依赖编译的多文件 checker；每测试点的独立限制随产物生效。未知 checker 不再静默回退到 diff。
- Kattis jury message 不进入选手反馈。校验器初始化/编译错误仅写 Worker 日志，给提交的 System Error 使用通用说明，避免源码诊断泄露。运行故障也不转发 team feedback。
- **真实 Server/Worker/PostgreSQL/Linux sandbox E2E 已通过发布和做题完整闭环**：私人检查 → 提交 → v1 发布及幂等重试；reader/editor 发布均 403；不匹配检查及过期版本均拒绝；v2 使用不同答案与 exact 比较。比赛在 v1 编排后仍使输出 `03` 得到 AC，普通练习 v2 对同一输出给 WA，提交的 problemVersion 分别为 1/2；另测比赛 WA，原始 JSON 不含 jury 私有消息/产物内部字段。
- 新基线在本任务 `authoring_publish` 数据库完成真实测试和 down/up；测试用 Server/Worker 容器及 `vertex-authoring-publish-data` 卷已清理。Worker 产物完整性单元测试、相关 Go 测试及 vet 通过，OpenAPI/客户端已更新。

2026-09-19 第六轮实现：

- 新增独立 `infrastructure/packages` 格式层：受限 ZIP、原生完整归档、ICPC legacy-icpc / Kattis legacy / 2025-09 基础批处理导入导出、DOMjudge 固定时限扩展、Polygon 已生成数据包导入、洛谷平铺 `.in/.out`（亦接受 `.ans`）数据导入。XML/YAML 配置有复杂度限制；拒绝重复键、DTD、路径越界、特殊文件、大小写碰撞及外部/循环链接；内部文件链接有界解析，解析后也计入实际展开总量。
- 题包导入分为私人预检和原子应用，新增 `problem_imports` 到 baseline；1 小时有效期，预检及应用均重查权限、copy token 和冲突状态，幂等应用不会覆盖之后编辑。完整题包替换副本；平铺数据包保留题面、程序、全局配置和已有其他测试，同名导入材料按稳定 ID 更新，数字编号按数值排序。导入不隐式提交或发布。
- 未映射的评分/分组、程序协议、运行脚本、自测数据、未知配置等保留原材料，并写入版本化 `requirements`，明确阻止错误检查或发布。基本设置显示这些要求，作者处理相关材料后可显式解除。Kattis/Polygon 元信息新增 `resourceMode: exact`，Worker 与发布产物评测按精确资源限制执行，不再套用站点语言倍率；普通站点模式保留 `language-scaled`。
- 原生归档确定性导出，完整 tree/blob 往返保持 hash；标准导出核对题面格式、语言、校验器协议与比较策略。生成型输入/答案读取同数据指纹、同检查策略且有权访问的成功检查产物，校验字节摘要后导出；不能使用其他作者尚未共享的私人检查。导出返回私人 blob 和兼容提示，不将归档塞入 JSON。
- 新增 `/imports`、`/imports/:id`、`/imports/:id/apply`、`/exports` API，OpenAPI/sqlc/前端客户端同步。新工作台增加题包子页面：本地选择文件、预检差异数量和兼容提示、应用到草稿、格式选择和下载。真实接口已接通，**mock 的题包操作尚未实现，不能把目前 mock 页按钮当作完整演示闭环**。桌面暗色布局已截图实查，手机/亮色交互仍待验证。
- **外部互操作已实际验证 legacy**：`TestExportWithKattisProblemtools` 使用固定镜像 `docker.io/problemtools/icpc@sha256:36abd98219649fac88dcc22dea6bf59c9e84f11db9385df992382fb531ce7009`（verifyproblem 1.20260907），无网络/去 capabilities，在自建可运行夹具上检查题面、输入校验器、样例/秘密数据、AC 和 WA 程序；`-e -t 1.5` 通过，0 errors / 0 warnings。固定时限由命令参数提供，不能声称该工具自动理解 DOMjudge INI。夹具自建，无第三方题库材料。该工具只声明 legacy 和部分 2023-07-draft 支持，**不把自测往返当作 2025-09 外部认证**。
- Polygon 测试覆盖时限/内存单位、testlib 程序角色、测试顺序/样例、原答案 `03\n` 保留、生成命令原文保留、缺失已生成数据拒绝与未匹配工具链阻断。洛谷配置目前映射逐点时间/内存，其余规则保留阻断。更高级 Kattis 分组/参数、自测数据、testlib→Kattis 导出适配和 TeX/PDF 发布尚未完成。
- **真实 PostgreSQL/API 验证**：`authoring_exchange` 中通过 imports/revisions/checks 和 `TestDomainAPIIntegration/AuthoringWorkbench`。原始 HTTP 断言私人导入预览、秘密 blob、导出归档不能被其他协作者或跨域读取；数据库测试还验证生成产物篡改拒绝、未共享检查不能复用、共享提交后可复用（这里模拟任务成功，非新一轮沙箱执行）。独立 `authoring_exchange_schema` 完成 baseline up/down/up。既有用户数据库未动。
- 相关 Go 测试/vet、Worker artifact/builder/checker/executor 测试、TypeScript、mock 回归与 mock 构建通过；366 个 sqlc/OpenAPI/client 文件重复生成字节一致。Windows formatter 文件锁重试后成功；无生成漂移。未提交或推送。

2026-09-19 第七轮实现：

- 新增检查、发布两个真实子页面并接现有新 API。检查可以选择已保存私人副本或提交，展示结构引用问题、排队/执行进度、可取消任务、参考解逐点矩阵、测试点与日志；轮询只更新报告，手动刷新不再重置材料检查状态。报告明确区分完整 tree 一致、评测材料一致和旧材料；大矩阵按程序/测试分页，数据详情也分页。
- 发布页面联动所选提交的材料、题面语言、同数据指纹/检查策略的成功检查和当前公开版本；请求携带 expectedVersion，保留权限/格式/compatibility requirements 阻断。显示发布历史和比赛固定版本说明。共用 SaveButton 的连续加载/对勾反馈，并允许自定义“发布中/已发布”标签，原保存调用保持默认文案。
- `workbench-checks` mock 新增私人检查、固定材料快照、进度/取消、共享后的报告读取、成功检查匹配、显式发布、幂等与版本冲突；日志明确写“本地演示，未执行程序”。演示设置增加主动创建一题完整 A+B 示例，不替换已编辑题目、不自动增加一堆组合。该示例含两个参考解和六个测试。
- 材料编辑在停顿 900ms 或失焦后自动保存，保存期间继续允许输入；请求只确认自己发送的那份文本，迟到响应不覆盖后续输入。保存失败停止自动重试并保留输入；可读取服务器最新版本，就地比较后采用服务器或带新 token 显式保存当前材料，其他材料不被覆盖。父容器不再在每次保存结果回来时无条件清除后续输入的 dirty 标记。
- 协作冲突新增基线/本人/最新三份内容和手工合并，可编辑文本、路径和属性；上传新内容后通过已有 token 原子记录逐项解决。未保存手工输入与提交说明受离页保护，合并请求中禁用导航。历史增加按 before 游标加载更早提交；只读审阅者的差异页读取共享提交，不再错误地请求自己的工作副本。
- **浏览器验证（mock）**：新建演示题 #1012 → 检查完成 → 对 r1 显式发布 v1；自动保存修改后仍基于 r1，公开版本不变。慢速 1.8s 请求实查“保存中仍输入 B”，旧请求返回后 B 保留，后续自动保存完成并重载仍保留最终内容。约 1272px 桌面暗色/亮色检查矩阵均截图实查；临时外观和慢速场景已恢复为原“跟随系统/正常”。新检查/发布页的窄屏与真实后端浏览器流程、保存冲突/手工合并浏览器验收仍待补齐，不能用 mock 演示替代原生执行证据。
- 新增 mock 回归覆盖私人检查不可读取、编辑后旧检查不能发布、共享相同提交后检查可审阅、editor 不能发布、发布幂等、取消保持终态、陈旧 token、上传手工合并结果且不自动发布。前端 42 个测试文件 / 197 项测试通过；TypeScript、mock build、diff whitespace 检查通过。本轮未改后端，也未重复无关 Go/原生沙箱验证；此前真实后端/Worker 证据仍见第四至六轮。

2026-09-19 第八轮实现：

- 新增有界材料目录 API `GET /materials`：按 test/program/group 读取类型化文档，测试按元信息顺序分页，游标用稳定 entry ID；返回位置、总数、next 和源 token/revision。私有副本与共享提交分别授权，目录不读取输入/答案大文件。单页最多 100 项并受描述符字节预算限制；无效文档保留可定位的错误项，便于打开原文修复。
- 新增 `POST /working-copy/batch`：批量移除、样例/预测试标志、分组、分值和完整测试顺序。先在内存/内容存储准备，最终一次 CAS 保存整棵副本；任何无效选择、错误顺序或陈旧 token 都不会部分更新副本。删除测试点与删除其顺序引用一起保存，不级联删除可能被复用的数据文件。
- 测试数据改为紧凑分页表格，区分测试点/分组/原始数据文件；只在选择后展示批量操作，支持上下移顺序。数据和附件在右侧覆盖抽屉中编辑，关闭时保护未保存输入。源程序页改为按用途的程序列表、可折叠目录树和代码/配置编辑区，程序中的重名源文件使用完整路径区分。
- 上传支持一次选择多份文件或程序目录，保留相对目录结构，4 个并发上传后一次保存副本；预先拒绝已有/重复路径，一次最多 200 个文件（更多数据使用题包）。材料编辑补单项移除，二进制材料重命名省略 text 字段，保留原始字节。
- 发现并修复发布能力边界：之前直接创建的分组/逐点分值/预测试可能绕过“导入 requirements”阻断。现在 inspection 增加独立 `publicationIssues`，允许继续检查程序/数据，但发布事务和 UI 都阻止尚未接入实际提交汇总的语义。**这只是准确的能力阻断，分组/逐点计分和预测试执行本身仍待实现，不算这些能力完成。** 普通发布 E2E 夹具改成无逐点分值；浮点材料/检查报告的保留仍由领域和 Worker 测试覆盖。
- **真实 PostgreSQL/HTTP 验证通过**：本人目录分页、测试顺序、reader 读取共享程序、editor 私人副本看不到 owner 的新测试、错误 batch 不改 token/内容、成功批量标记/排序、陈旧批量拒绝、移除同步顺序、二进制改名不改字节。`TestPackageImportTransactions` 另验证 PublishCommit 明确拒绝未实现的逐点计分，而非只靠前端禁用按钮。
- **浏览器 mock 实查**：#1012 数据表六个测试的行高/对齐、选择两项后出现批量栏、一次改成样例、右侧编辑抽屉、程序列表和展开的源码目录。目录上传和新页面窄屏仍需最终浏览器验证。前端 42 文件 / 198 项测试、TypeScript、mock 构建通过；authoring Go 测试、vet、diff whitespace 通过，生成 API 已同步。
- 本轮未运行新的原生 Server/Worker 全链路；因发布能力阻断与 E2E 夹具发生变更，收尾时必须重新运行原生发布/比赛固定版本 E2E，不能以第五轮结果证明当前最终版本。独立 `authoring_exchange` 数据库继续用于测试，没有重置用户数据库，未提交/推送。

2026-09-19 第九轮实现：

- 新增 `/authoring/problems` 列表 API，显示本人副本状态、共享 head、可读的最近检查、公开版本和发布来源；查询保持域及 owner/用户/组授权，其他人的私人副本与未共享检查不参与列表或搜索。分页查询在新鲜账户/域权限锁下执行，不逐题创建副本或逐题读取 blob；超出末页也保留正确 total。
- `problem_content_trees` 在 baseline 增加不可变 `summary` 派生投影，保存内容树时从已授权且核验摘要的 metadata blob 生成 title/source/difficulty。内容仍以 blob/tree 为事实来源；无效草稿不产生伪造摘要。列表按本人 copy → shared head/initial 的树选择投影，支持搜索当前草稿标题、来源和公开题号，以及未提交/冲突/未发布筛选。
- 新 `LibraryPage` 替换旧管理列表，保留站点容器、标题、按钮及紧凑表格；刷新/筛选只更新表，显示旧检查针对之前材料。新建和导入入口使用私人题目，并将错误放在名称输入旁，不触发浏览器默认校验或顶部通知。
- 删除无引用的旧 `AdminProblemPage`、`ProblemWorkspacePage` 和 `admin/workspace` 的 9 个面板/类型文件；路由只指向新 `LibraryPage` / `WorkbenchPage`。**后端旧共享表、旧 package 写 API、旧构建服务依赖及跨域复制仍在，尚未完成最终清理，不能把前端死代码删除当作全栈切换完成。**
- **真实 PostgreSQL/API**：新 `authoring_library` 基线通过 revisions/checks/imports 回归；新列表测试证明本人能搜索私人标题，editor/reader 不能通过搜索或原始 JSON 取得他人的私人标题、etag、blob 摘要及检查 ID；本人正确看到检查匹配状态，未打开的协作者不会被创建副本；跨域列表隔离及空末页 total 正确。第一次运行修正了手工应用基线后的测试库 migration tracking，并在真实查询中发现/修复 head 无 updated_at 列的问题，改取共享 commit 时间。
- 浏览器 mock 实查桌面列表状态、搜索单个题号、新建空标题的就近校验；没有额外创建示例题。前端 43 文件 / 199 项测试、TypeScript、mock 构建通过；authoring Go 测试/vet、diff whitespace 通过。374 个 sqlc/OpenAPI/client 文件重复生成字节一致。未提交或推送。

2026-09-19 第十轮实现：

- 新增 `POST /authoring/problem-copies` 与 `/authoring/problems/:id/origin`。复制从指定已发布版本的 source_revision/source_tree_hash 读取完整材料，在目标域分配独立私人题目与工作副本；源/目标域权限及源题目包权限在事务内锁定，没有隐式提交或发布，来源记录沿用不可变 provenance。
- 新 `CloneChecked` 校验来源目录摘要、实际 manifest 与发布记录、全部文件和评测数据指纹，然后复制普通文件，使用发布提交的 snapshot 替换检查时可能尚未提交的说明元信息。输入、答案、程序文件、固定依赖和校验规则保持一致；目标拥有独立文件与摘要。拒绝篡改、外部链接及不同评测规则，复制不执行程序。
- 目标记录 `stage=copied` 的验证产物，明确说明复用来源验证，没有伪造新的执行日志/参考解矩阵。提交这份副本后可用该产物发布；再次修改评测材料仍需新检查。未提交的源材料、私人检查说明不会转移。
- 真实复制测试发现了内容树图在删除整个题目时的外键检查顺序问题；baseline 将相关 NO ACTION 外键设为 DEFERRABLE INITIALLY DEFERRED。事务末仍禁止删除被引用 blob，但完整题目级联可正确完成。Problem.Delete 不再在 COMMIT 前删除磁盘文件，物理删除归后续引用感知回收负责，避免提交失败损坏仍有效的版本。
- **真实 PostgreSQL + 文件系统** `TestCommittedReleaseCopy` 通过：来源权限拒绝、复制落盘后注入失败的数据库回滚、私人检查不可直接读取但可安全复制已发布材料、无隐式提交、原始字节独立、来源题目及产物删除后副本仍能提交/发布。另有 `CloneChecked` 文件篡改、私有元信息清除、规则不匹配测试；普通引用 blob 的单独删除仍被 FK 阻止。
- **实际 Server + PostgreSQL + Worker + Linux 原生沙箱** 的 `TestEndToEndFrozenAuthoringCheck` 已重新通过，覆盖第八轮发布阻断后的普通题、v1/v2 校验差异、比赛固定 v1、原始提交 JSON 隐私，以及来源已到 v2 时复制 v1 → 提交 → 发布 → 以 v1 Kattis 规则判 `03` 为 AC。复制来源记录对无权限成员返回不含 provenance 的拒绝响应。最初两次测试调整了测试夹具资源上传类型和预期拒绝状态码后，完整运行通过。
- 发布页新增按版本和目标域复制，以及来源记录展示；mock 同步为独立 tree/blob/验证产物，并补回归。浏览器从 #1012 的 v1 创建 #1013，验证原发布题面、来源说明、尚未提交/未发布状态；源副本后来新增的文字没有被复制。另修正发布页重载后对同一提交/检查错误显示“发布 v2”的问题，现在识别当前已发布选择。
- 前端 44 文件 / 200 项测试、TypeScript、mock 构建通过；authoring/相关 problem Go 测试和 vet、真实 revisions/checks/imports/API 回归通过；375 个生成文件重复生成字节一致。本轮创建的 `vertex-authoring-copy-server`、`vertex-authoring-copy-worker` 及 `vertex-authoring-copy-data` 卷已清理，日志在 `.cache/authoring-native-copy-*.log`。没有动用户原有数据库/卷，未提交/推送。

2026-09-19 第十一轮实现：

- 新增引用感知回收服务，Server 启动及每小时执行有界轮次，默认宽限期 24h；`AUTHORING_GC_INTERVAL` / `AUTHORING_GC_GRACE` 可配置，最小宽限期 1h。记录/文件年龄用于宽限期判断，不提供删除后的回收站语义；部署文档已说明配置和边界。
- 回收过期预检和临时上传授权，再清理无根内容树和 blob。根包括全部提交、head/initial tree、个人副本、合并本地/远端树及手工解决结果、检查输入、预检及发布绑定；产物按实际存储路径检查发布/检查/候选数据引用，兼顾尚未移除的旧路径。临时上传重复提交同摘要会刷新授权期限；拥有历史检查/合并结果的作者在临时授权过期后仍能合法读取自己的材料，其他作者不能借此访问。
- 存储生产者新增每题共享事务锁；回收用同一 key 的独占 session 锁跨越“引用清理 COMMIT → 文件清理”，繁忙命名空间本轮跳过。上传和导入利用 HTTP 已暂存/读取的内容，在单个受保护事务内安装文件和引用；产物上传也在同连接事务中登记，避免双连接嵌套导致连接池耗尽。
- 物理清理只处理 UUID 命名空间、摘要对象和带题目标识的已知暂存目录。先原子改名为 `.gc-*`，再删除，进程中断不会留下可被生产者误复用的半个正式目录；残留 tombstone 后续清理。使用 rooted 文件操作，不跟随链接，不触碰未知文件。数据库提交失败不开始物理删除；取消时释放 session 锁，解锁失败则丢弃连接，防止污染连接池。
- 发现上传纳入事务后可能让 PostgreSQL `now()` 停留在事务开始时，因而新/旧构建租约判定与续期统一改用 `clock_timestamp()`。新增真实 PostgreSQL 慢速事务测试：事务开始时租约有效，300ms 后超过 200ms 租约，回写必须仍被拒绝。
- **真实 PostgreSQL 并发/故障测试通过**：提交失败时文件保留；过期预检/旧自动保存与孤立上传被清理；提交、副本、合并结果、历史检查输入和检查产物保留；合并结果经历授权过期和 GC 后仍可完成；删除整题后遗留命名空间清理。另一测试阻塞去重上传，确认 GC 跳过；暂停 GC 在 COMMIT 后，确认新上传等待再重新建立文件；取消 GC 后新上传不被遗留锁阻塞。Linux 文件系统测试验证外部符号链接、未知文件与同摘要新对象不被 tombstone 清理误伤。
- **原生 Server/Worker E2E 已重新通过**检查 → 发布 v1/v2 → 比赛固定旧版 → 复制 v1 再提交发布和判题的完整回归。使用独立 `authoring_native_gc` 与任务专用 Server/Worker/数据卷，验证后已移除运行容器与数据卷；日志留在 `.cache/authoring-native-gc-*.log`。
- 最新 baseline（含路径查询索引）在独立 `authoring_gc_schema` 完成 up/down/up。authoring / problem / config Go 测试、相关 vet、真实 PostgreSQL/API 回归通过。本轮没有前端逻辑修改，因此未重复前端全量验证。未提交、推送或重置用户数据库。

2026-09-19 第十二轮实现：

- 正式 Router 已停止注册旧 package/statements/files/tests/builds/publish/releases、旧 `/problem-copies` 和旧 origin 路径，以及旧问题元信息 PUT/直接 testdata 上传。移除 Router/Main 的 AdminPackages 依赖。新增真实路由断言，已认证 owner 对旧写入口也得到 404，不能再通过旧 API 改写共享材料或发布候选。
- 新 `/authoring/problems/:id/visibility` 将可见性作为资源治理处理，要求 ManageAccess，按 expectedVisibility 防止陈旧修改；不会改工作副本 token、内容树或提交历史。协作页新增统一组件的可见性设置和删除题目交互，保存反馈沿用连续动效，错误就近显示。权限修改后只刷新资源权限，不再全页重载工作副本。
- 业务 E2E 的测试数据准备迁移到新材料 API、明确提交和发布；通用静态数据夹具附带确定性的 Python 参考解。API-only 与停止 Worker 的协议测试显式使用测试 Worker 凭据模拟夹具完成，并在检查日志标明未执行程序；正常原生套件由真实 Worker 执行，不用此模拟模式。
- 重写 `TestEndToEndProblemAuthoring` 为新工作副本/API：testlib checker、validator、generator、正确和错误参考解、参数数组、生成答案、发布样例、真实 AC/WA 和纯题面改动复用检查。域协议测试也改成新检查快照，验证改动后的代码不会混进已冻结任务，取消后回写被拒绝。
- **实际 Server + PostgreSQL + Linux 原生 Worker** 通过 `TestEndToEndProblemAuthoring`（完整 testlib 执行）、`TestEndToEndFrozenAuthoringCheck`（版本固定与复制）、`TestEndToEndDomainWorkflow`；随后停止 Worker，通过新 `TestEndToEndDomainProtocol`。真实 API-only 全套 `TestDomainAPIIntegration` 通过，包括旧入口 404、可见性 editor 拒绝/owner 成功/陈旧状态拒绝且副本不变。
- 前端 44 文件 / 201 项测试、TypeScript、mock 构建通过；相关 Go 测试/vet/差异格式检查通过。浏览器 mock 验证 #1013 修改可见性后仍“尚未提交”，保存完成无整页刷新；已恢复其私人可见性。原生测试使用独立 `authoring_cutover` 及任务专用容器/卷，现已清理，日志在 `.cache/authoring-native-cutover-*.log`。
- **清理仍未完成**：旧 handler/DTO 的源码、Swagger 注释、旧 application/package repository、共享表及旧 mock 路由/测试还存在。它们已不在正式 Router 注册，但应继续迁移测试并删除，不能把 404 入口作为全栈模型清理完成的证据。当前 OpenAPI 仍需随这些源码删除去掉陈旧操作；最终生成一致性检查必须在清理后重跑。未提交或推送。

2026-09-19 第十三轮实现：

- 从旧编辑 Service 中移出 Claim/Progress/UploadPackage/Complete，新增独立 BuildService 与窄的 WorkerBuildRepository/ArtifactPublisher 端口。正式 main 和 API 集成装配只创建 BuildService，不再把旧 PackageRepository 注入 Worker 服务；旧编辑 Service 不再包含 Worker 请求方法。
- BuildService 要求当前 frozen-check 协议，并拒绝没有 Check 快照的旧内联题包。新增测试验证不声明协议、旧包被拒绝、当前冻结包正常返回；原有 Worker 身份、目标路径、过期租约及失败后保留产物的测试已迁移到实际 BuildService。
- application/transport 测试、vet 和真实 PostgreSQL 的完整 TestDomainAPIIntegration 通过。没有改动前端或 wire DTO，未重复前端验证。此步只是切断运行时编辑服务依赖，**不是旧模型删除完成**：BuildRepository 仍有旧查询方法，旧 handlers/DTO/共享表及部分 tenancy/legacy 测试尚待迁移删除。未提交或推送。

2026-09-19 第十四轮实现：

- 移除 mock 旧共享 workspaces、problemDrafts、候选样例、buildInputs、旧复制实现和 package/files/tests/builds/publish/testdata 写路由；保留资源创建、权限、所有者、删除等治理接口。不可变公开投影继续供比赛和历史提交固定版本，新 tree/blob/commit/check 是唯一出题状态。新建默认为 private，初始化材料同时保留标签/难度；标签治理只更新当前分类，不改私人副本或历史发布。
- 旧 mock 回归迁移到新接口，保持权限撤销、大输入完整读取、未发布材料隐私、陈旧检查拒绝、发布幂等、比赛手工采用版本和来源删除后复制可独立发布等覆盖。检查报告从冻结文件读取预览/字节数；公开样例由所选提交的文件生成，后续私人修改不会混入，动态生成样例在 mock 中明确拒绝而非伪造结果。前端 44 文件 / 201 项测试、TypeScript、mock 构建通过。
- tenancy 的真实 PostgreSQL/HTTP 测试改用新工作副本和检查/提交/发布/复制 API，保留跨域读取及取消拒绝、内部 Worker 按租约读取源文件与输入、发布 owner/editor 区分、请求大小限制、比赛采用版本 CAS、目标域/owner 注入拒绝、来源权限与公开 provenance 隐私。用于授权测试的产物夹具明确标注未执行程序，不替代原生 Worker E2E。迁移中补齐原空题面夹具，并修正 CancelCheck 双返回值的测试断言后，20 项域集成规格全部通过；完整 TestDomainAPIIntegration 亦通过。
- 删除旧 PackageHandler 及 statements/files/tests/builds/publish/copy 处理器、旧路由注册、旧 workspace/release DTO、Problem.Update/UploadTestdata HTTP 处理器及对应 DTO。共用错误映射/请求上限独立到 errors.go。OpenAPI 和生成客户端不再宣告这些已下线操作，资源治理 GET/POST/DELETE 与新 /authoring 路由保留。OpenAPI/client 二次生成一致（当前生成文件共 353 个），相关 Go 测试、vet、diff whitespace 通过。
- **内部清理尚未结束**：旧 application.Service、PackageRepository/PackageQueries、部分旧 Worker 内联 wire 字段、共享表及相应基础设施测试仍待迁移删除。旧 Service 已没有正式 HTTP/运行时消费者，只剩遗留仓储测试；本轮不把接口/客户端切换等同于底层模型清理完成。未运行新的原生 Worker E2E，收尾仍须重跑。未提交或推送，没有更改用户原有数据库/卷。

2026-09-19 第十五轮实现：

- 删除旧编辑 application.Service 与其 Copy/Origin 包装，不再保留“每次保存增加 packageRevision”的应用层流程。旧文件/测试点表单规则单测随旧服务移除；当前 typed material/tree/inspection 的验证测试保留，Worker 租约、目标路径、防止错误清理产物等单测迁到 build_service_test.go。复制输入规范化测试移到实际领域 NormalizeCopy。
- 原跨域复制 PostgreSQL 规格全部迁到 RevisionRepository.CopyRelease 和已提交内容树，保留未发布源码隔离、未使用源文件也完整复制、ACL 不继承、来源删除后提交/发布/判题任务独立、复制来源串联且不可修改，以及源包权限/目标创建权限、归档域和锁等待期间撤销目标角色/源组成员的验证。
- 删除旧 PackageRepository.Copy/Origin、旧 ArtifactCopier/Publisher 接口和对应共享表复制 SQL。Origin 映射随新 replica 实现保留。新复制失败不在事务回滚前冒险删除文件，测试改为实际执行引用感知 GC，确认失败复制无目标资源记录、无引用文件随后清除、源发布仍存在；文件损坏拒绝和 SQL 注入失败覆盖均保留。
- **真实 PostgreSQL** 的 19 项出题仓储规格通过；首次迁移发现 BeforeEach 的 context 在 It 中已取消，已改成使用当前 spec context；旧“先篡改 build input 再由 Claim 拒绝”用例改为验证数据库直接拒绝冻结输入修改，符合新不可变约束。相关 authoring/problem/tenancy/e2e Go 测试与 vet 通过；sqlc 重复生成字节一致，diff whitespace 通过。本轮未改 UI/HTTP wire/schema，也未重新执行原生 Worker；不以未设置真实服务的普通 go test ./e2e 声称原生 E2E 完成。
- **剩余内部清理**：PackageRepository/PackageQueries、旧共享草稿表与问题/标签查询依赖、BuildRepository 的旧 Enqueue/Get/Cancel 与候选写回、Worker 旧内联 wire 字段仍需删除。旧 filesystem.Clone 及旧兼容产物路径也尚在。问题 access 测试、旧 release/package 仓储测试还有旧写 API 依赖，下一轮需迁移后再删，保留比赛固定版本和取消重测恢复版本的回归。未提交/推送。

2026-09-19 第十六轮实现：

- 旧 release 规格迁为已提交内容树 + 冻结检查发布，保留公开投影不随私人保存变化、owner/editor 发布权限、重试任务不能复用旧租约产物、缺少本次产物不能成功、排队提交固定发布版本、比赛题目重排不自动换版、显式采用版本 CAS、取消重测恢复原版本与判定、被引用题目不可删除等回归。授权夹具仍明确未执行程序；原生执行证据须在最终 E2E 重跑。
- 问题协作 PostgreSQL 测试改用本人副本和显式提交给 reader 审阅；私人标题不出现在公开复用列表、组权限撤销即时生效的断言保留。只测试旧“每次编辑 bump revision/单 active 文件/共享样例候选”的 package_repository 规格删除，当前新材料/批量/内容树/检查用例继续验证替代行为；Ginkgo 套件入口移到 suite_test.go。
- 删除 authoring 的 PackageRepository、PackageQueries、旧 statement/release 仓储与 domain.Repository 旧端口；删除 package_read/package_write/statement SQL 及 sqlc 遗留输出。BuildRepository 收窄为内部 Worker 端口，移除 Enqueue/Get/Latest/List/Cancel，队列只接受 source_tree_hash 非空的冻结检查；完成检查不再写回 problem_candidates、built_revision 或共享题面。保留按租约、工具链和产物绑定验证结果。
- 迁移旧“无权上传不读取正文”HTTP 测试时发现新 blobs/imports 在权限验证前解析 multipart。新增 AuthorizeEdit 前置检查，拒绝后不读取正文，也不创建工作副本；实际上传/导入写事务内仍重复新鲜权限检查，避免把前置检查当作最终授权。HTTP 回归覆盖两种入口的无权正文不读取。
- **真实 PostgreSQL** authoring 完整仓储测试通过，包括标准库 revisions/checks/copy/GC 测试和迁移后的发布/复制 Ginkgo 规格；problem 的 8 项权限/查询规格和完整 TestDomainAPIIntegration 通过。相关 Go 测试、vet、diff whitespace 通过；sqlc 重复生成一致（当前生成文件 350 个）。本轮没改 UI 或公共 wire，无须重复前端构建；没有重跑原生 Worker。未提交/推送。
- **共享表还未删**：依赖剩在 problem.Repository.Update/SaveTestdata/Create 的标签写入、problem 管理查询、console 标签治理、dbtest.PublishedProblems 和少量其他领域夹具。先改这些依赖再合并 baseline 删除 problem_workspaces/problem_candidates/problem_statements/problem_files/problem_tests，不能在它们仍被查询时仅删 schema。问题发布表及 Worker 旧 revision/data_revision/内联 wire 字段也尚待后续清理。

2026-09-19 第十七轮实现：

- 从 baseline 移除 problem_workspaces、problem_candidates、problem_statements、problem_files、problem_tests 五张共享编辑表及初始化触发器、索引/FK；不添加兼容表。Problem.Update/SaveTestdata 应用/仓储方法与旧端口删除，Problem.Repository 只保留资源治理，不再注入文件存储。所有调用者已同步；无人调用的旧 problem/filesystem ZIP 候选实现及其旧接口测试一并移除，题包安全性由当前 authoring packages/filesystem 测试覆盖。
- 问题管理与公开查询改读资源投影，不与已删除的共享工作区 join。私人草稿搜索使用 authoring.Library；查询测试验证私人标题仍不能进入公开列表。新建题目直接维护资源标签，使用分类共享锁和原子标签 upsert，避免新建/改名及同名标签并发丢失。
- 标签治理只变更当前资源分类，既不覆写个人副本/etag，也不覆写发布快照；真实 console 规格验证副本前后完全相同。GC 去掉候选表根，继续保护实际发布/检查产物。dbtest 发布夹具直接从资源生成不可变版本，Judge 专用夹具显式提供数据引用；不通过已删除的候选表绕行。
- 使用新建的任务测试库 **authoring_unified** 自动应用当前基线，执行真实 PostgreSQL 的 `go test ./...`。除 Judge 的旧 DataVersion=3 夹具断言外所有模块通过；新提交发布不再产生候选计数，该过渡字段当前为 0。修正断言后单独重跑完整 13 项 Judge 仓储规格通过，包括首轮因 fail-fast 跳过的规格。完整 Server Go 测试及 vet 通过。测试过程中发现等待来自测试共享 advisory lock 和磁盘同步，未重启活跃任务或绕过验证。
- 独立 **authoring_unified_schema** 完成初始 schema up/down/up，查询确认五张旧表不存在。这个手动 roundtrip 库没有 migration tracking，不能作为 dbtest 运行库。用户原有数据库/卷未更改。
- 更新 docs/03-database.md、08-problem-authoring.md、15-backend-models.md 的模型图和流程，05-contest-rankboard.md 的评分边界改为当前真实发布阻断。出题文档明确格式支持范围、legacy 外部验证范围、mock 不执行程序、TeX/PDF/大样例/高级评分和浏览器最终验收尚未完成。
- sqlc/OpenAPI 二次生成字节一致，当前 350 个生成文件，diff whitespace 通过。本轮没改前端 wire/UI，未重复前端构建；未执行新的原生 Worker，最终仍须重跑。未提交或推送。

2026-09-19 第十八轮实现：

- Worker 只执行 frozen check pipeline，删除旧内联程序编译/生成/答案覆盖流程及 SourceFile/Solution/TestSpec。Claim 协议只传域、任务与租约、冻结快照和阶段预算；源码和输入通过内容引用下载。客户端拒绝缺失快照、旧内联任务及未知协议，不把成功 HTTP 中的无效输入无限重试。服务端 checkProtocol 与响应 check 明确必填。
- 旧领域 File/Test/PackageMeta/Workspace/PublishInput/Release、旧表单 Normalize/Validate、内置旧模板和候选发布规则移除。Worker 持久化信封重命名 CheckInput，只含域/题目身份和冻结快照。题面按原始 Markdown 发布，RenderSamples 只追加已验证的完整样例，不从报告 head 构造公开答案；代码块转义/前导空白/多样例/语言回退测试保留。
- 移除发布表 workspace_revision/data_revision/artifact_version、旧结构化题面/源文件/重复材料树/样例 JSON 镜像；移除构建表 revision/data_revision，source_tree_hash 改为必填，去掉旧 NULL 快照队列索引。提交 revision、当前公开 version 和内容/评测指纹各自保留明确用途。Judge/Worker 去掉无消费者的 dataVersion，数据由固定 problemVersion 和产物摘要确定。
- 旧 filesystem.Clone 兼容拷贝实现删除，保留冻结产物拷贝共用的安全文件复制。安全回归迁到 CloneChecked，Linux 实测覆盖缺清单、内容篡改、缺失/额外文件、文件及路径组件符号链接、同源目标、取消与不覆盖既有产物；不再为旧目录散列提供兼容行为。
- **实际 Server + PostgreSQL + Linux 原生 Worker** 通过 TestEndToEndFrozenAuthoringCheck、TestEndToEndProblemAuthoring、TestEndToEndDomainWorkflow。真实 Kattis/testlib、生成器/校验器、参考解、显式发布、比赛固定 v1、复制 v1 后独立发布和评测均通过。停止 Worker 后 TestEndToEndDomainProtocol 通过，直接检查原始 claim JSON 不含旧 revision/dataRevision/内联源码和数据等字段，并下载冻结源字节验证不是后来编辑的内容。协议规格随后增加未声明协议返回 400 且不消费任务，真实 API 集成重跑通过。
- 最新任务测试库 **authoring_protocol_tests** 通过 authoring/judge/problem/tenancy 的真实 PostgreSQL 回归与完整 TestDomainAPIIntegration；原生运行库为 **authoring_protocol**。**authoring_protocol_schema** 完成手工 up/down/up（不带 migration tracking）。Server/Worker Go 测试与 vet、前端 44 文件 / 201 项测试、TypeScript 和 mock build 通过。347 个 sqlc/OpenAPI/client 文件再生成一致；中途 Windows 映射文件临时锁导致的生成失败重试后已消除。
- 任务 Server/Worker 容器和 vertex-authoring-protocol-data 卷已清理，只有 PostgreSQL 运行；原生日志在 `.cache/authoring-native-protocol-*.log`。用户数据未重置，未提交/推送。

2026-09-19 第十九轮实现：

- 新增实际 ZIP 的 mock 预检/应用/导出：Vertex 原生归档写入真实内容摘要与字节，平铺数据合并到本人的已有材料，保留题面、程序与稳定测试 ID；重复导入相同数据不重复增加测试。预检不改副本，应用一次 CAS、不提交/发布，幂等重试返回当前副本，陈旧/过期预检拒绝且保留新编辑，reader 只能导出共享提交。
- 浏览器前置异步读取/压缩与同步状态写入分开；写入前重新检查身份、权限和源副本 token。新增 fflate 0.8.3，锁文件只增加该叶子依赖，冻结离线安装通过。演示 ZIP 显式限制 8 MiB/展开 32 MiB/单文件 8 MiB/1000 条目，校验中央/本地头、CRC、实际展开字节和原生 SHA-256；拒绝路径逃逸、大小伪造、大小写/文件目录冲突、特殊文件、加密/分卷/ZIP64。mock 不实现标准格式解释器，页面明确说明使用真实后端。
- PackagesPanel 差异数量包含属性/类型变更；导入与导出错误分别靠近自己的控件，导出错误放按钮上方，避免手机用户回到页面顶部找错误。仍使用项目下拉框与按钮，没有浏览器默认校验弹窗。
- **跨语言互操作**：前端完整 A+B 示例导出 `.cache/authoring-demo-native.zip`，Go 的 TestBrowserNativeArchiveInterop 实际导入、检查结构、确认六个测试、再导出并校验树摘要不变，得到 `.cache/authoring-server-native.zip`。浏览器通过文件选择器加载 Go 导出的 ZIP → 预检 → 应用至任务演示题 #1014，仍“尚未提交”。浏览器实际下载 `C:/Users/RimuruChan/Downloads/vertex-draft.zip`（21:50:28，8891 字节）也由 Go 同一测试导入/往返通过；浏览器下载事件监听曾超时，但实际文件存在且已独立校验，没有把超时当作下载成功证据。
- **浏览器验证**：默认约 1272px 暗色、390px 暗/亮、768px 亮色、1440px 亮色题包页均实查；检查导入/导出、无自动提交、加载后材料名称、移动侧边菜单及关闭按钮、实际就近错误。临时外观恢复为“跟随系统”，viewport override 已 reset。#1014 的导入副本保留用于后续开发，未覆盖 #1012/#1013 的既有副本。
- 前端 45 文件 / 209 项测试、TypeScript、mock build、相关格式检查和 diff whitespace 通过。新增 Go 互操作测试明确用 VERTEX_MOCK_ARCHIVE 指定真实前端文件，普通测试不伪称执行互操作。本轮未改后端产品代码/schema/wire，无须重复原生 Worker；第十八轮证据继续适用。347 个生成文件保持一致。未提交/推送。

2026-09-19 第二十轮实现：

- legacy/legacy-icpc/DOMjudge 标准导出新增 Markdown → TeX 转换，使用固定 goldmark v1.8.6 AST，支持常用段落/标题/列表/强调/代码/表格、受限数学命令与矩阵、公开图片。输出 problemname 与普通名称注释；文字和代码完整转义，数学命令白名单、括号/环境约束、节点/深度/输出预算阻止把任意 TeX 程序塞进公式。转换不改源材料，字体/链接差异通过兼容报告说明；HTML、未支持宏和内嵌样例明确拒绝。
- 标准题面目录中的 PNG/JPEG/PDF 辅助文件按用途标为公开 statement-support；秘密测试说明图仍为私有 resource，不能通过 Markdown 图片引用导出。图片引用复制到标准题面目录的稳定摘要路径，并保留说明文字；修正原本把 statement 图片统一移到 attachments 导致相对引用失效的问题，增加导出目标冲突检查。
- 发现标准导出只检查 CanBuild、可能忽略直接编辑的逐点分值，现同时检查未实现的发布语义，不能将评分题悄悄变成 pass/fail。原生归档仍完整保存这些材料，新增回归验证。
- **实际外部互操作**：原 TeX 及 Markdown 转换的 legacy 示例均通过固定 problemtools 的 verifyproblem，正确/错误参考解分别 AC/WA，零错误/警告。转换示例进一步调用官方 problem2pdf 真实生成 PDF，使用 Poppler 渲染后完整检查公式、表格、代码特殊字符/缩进、图片说明和样例。QA PDF 为 `.cache/authoring-markdown-legacy.pdf`，日志 `.cache/authoring-markdown-interop.log`。初次验证发现新测试夹具漏传原浮点零容差，修正 2025-09 的组参数后通过；未修改输入/答案去迁就判定。
- 外部工具明确不识别合法 legacy-icpc 版本名；已查官方规范确认导出声明正确。没有修改实际导出格式来伪造通过，legacy-icpc 与 2025-09 的外部认证限制继续保留。PDF 工具在 Windows bind mount 处理最终文件时报权限错误，改在容器 /tmp 内完成生成/净化再按字节拷出，未放松隔离和 capability 限制。
- 新增结构/字面量、危险宏/字符重写、私有图片、路径/文件布局和评分导出回归；Server 全量普通 Go 测试、authoring vet 与差异格式检查通过，347 个生成文件未变。真实 PostgreSQL TestPackageImportTransactions 与完整 TestDomainAPIIntegration 通过；后端产品只改格式层，无新 Worker 执行逻辑，未重跑原生 Worker。未提交/推送。

2026-09-20 第二十一轮实现：

- 发布新增不可变 problem_version_files（合并 baseline），只登记已选题面、显式公开附件和检查产物内的样例。字节继续使用题目内 content-addressed blobs，原始样例不从报告预览重建。GC、协作者可读内容引用与 schema 回滚同步支持新根。
- Markdown / PDF 题面和附件、样例 API 增加显式白名单 DTO，不含摘要/私有存储路径。练习文件要求当前发布版本，比赛要求固定版本；原有域、比赛开始/报名、资源可见性检查应用到文件。测试覆盖私有资源、秘密附件、开赛前、历史私有版本、比赛旧版、匿名公开和下载安全响应头。
- 前端使用 pdfjs-dist 6.3.289 的隔离 worker 绘制 PDF，禁用注释动作/XFA/外部 worker 取数，设置文件/图片/画布预算；提供分页、缩放、文字内容与原件下载。Markdown 图片使用授权下载后的 object URL，附件链接跳到下载按钮。大文本样例预览受限、完整数据可下载；二进制仅下载。PDF 服务端目前只检查大小/签名，未声称完整语法验证；复杂字体/编码兼容仍需审查。小文本样例继续兼容原有 Markdown 样例呈现。
- **真实 Worker 捕获并修复问题**：含 NUL 的二进制样例预览导致 PostgreSQL JSONB 拒绝完成结果，Worker 重试使页面卡在检查中。报告文本现在替换 NUL/无效 UTF-8，按 UTF-8 边界截断；原样例 blob 不变。新增真实 PG 的进度日志/结果/二进制预览/多字节截断回归通过。
- **验证**：Server 普通全量测试、authoring/problem vet 通过；真实 PostgreSQL authoring/problem 仓储及完整 API 集成通过，修复后的专门二进制报告测试通过。原生 Server + Worker 的 TestEndToEndPublishedFiles 使用真实已验证 PDF，1.1 MiB 文本和 NUL 二进制样例，完整通过。初始失败来自容器网络和测试配置；随后真正发现 JSONB 缺陷并修复，不以 API 夹具代替原生证据。前端 46 文件 / 211 测试及生产构建通过，PDF 缩放改动后再次构建通过。352 个 sqlc/OpenAPI/客户端文件二次生成一致；当前 baseline 在新库完成 up/down/up。
- **浏览器**：真实后端公开题面在桌面约1272px与390px暗色实看，PDF 文字/公式/表格/图片正常，样例截断和二进制说明显示；2倍缩放画布约685px，文档宽仍382px，横向滚动限制在PDF区域。生成客户端期间 Vite 缓存了中间文件导致白屏，重启开发服务后恢复，非正式构建错误。尚未在本轮完成独立 Markdown 图片、亮色、下载交互和完整多页 PDF 验收，不能据此宣称所有公开呈现验证完成。未提交/推送。

2026-09-20 第二十二轮实现：

- 检查协议升级 vertex-authoring-2。CheckSnapshot 只为可执行 TeX 保存源文件和公开附件的不可变引用；它们纳入检查指纹，Markdown/PDF 纯题面修改仍不改变评测指纹。每次最多20份，文件数量/下载总量受已有预算约束。私有 resource、私有 asset、程序与秘密数据不进入单独的题面渲染沙箱。
- Worker 新增 statement 阶段，XeTeX +固定可信封装在原生 Landlock/seccomp/独立UID/cgroup/rlimit 环境运行；禁止 shell escape，使用独立暂存目录和显式程序参数，不拼 shell。支持常用 Kattis 片段、problemname、公式、表格、相对路径图片和中文字体。渲染失败使检查失败；不在发布事务里执行 TeX。早期尝试 LuaTeX 遇到字体缓存权限与内存成本，最终改用XeTeX并实测，不放宽原生沙箱边界。
- 渲染器镜像预装 texlive-xetex / 常用宏包 / Noto CJK；只读格式与索引复制到 /usr/share/vertex/texmf-var，不为编译开放 /var。工具链包括引擎版本、发行包版本、封装/模板与现有sandbox策略。每份30秒CPU/40秒墙钟、1GiB内存、8进程、默认64MiB工作区、32MiB PDF上限；运行时不联网下载包。未支持的自定义宏包或特殊TeX路径明确报错，不能声称任意TeX文档完全兼容。
- 产物声明 statements/ID.pdf，服务端严格校验与快照一一对应和内容摘要；发布保存对应PDF到公开blob，继续绑定release/比赛固定版本。新增检查PDF预览API，在题目授权事务内检查共享/私人check权限后读取、核验文件。检查报告只返回预览ID/语言；检查页可发布前预览/下载，编辑器不再把TeX当Markdown渲染。复用既有PDF分页/缩放/文字层和本地错误显示。
- **真实原生验证**：独立原生renderer测试通过中文、公式、表格、相对图片、未暂存文件不可读、shell escape关闭和无限循环取消；生成 .cache/authoring-tex-native.pdf 经Poppler渲染完整检查，无缺字。真实Server+Worker TestEndToEndTeXStatements和PublishedFiles均通过，日志 .cache/authoring-tex-native-e2e.log。覆盖私人检查预览对其他编辑者404、提交后可审阅、发布PDF字节与预览相同、修改TeX用旧检查发布409且旧版不变。API-only套件显式生成占位PDF只验证权限/绑定，不拿它证明编译；原生E2E未使用该替身。
- **回归**：Server普通全量测试与authoring vet、Worker普通全量、真实PostgreSQL authoring仓储与完整API套件通过；前端46文件/211项、TypeScript和生产构建通过。新增domain回归确保公开图片改变检查指纹、私有材料不影响渲染、不把Markdown送编译；artifact回归拒绝篡改/缺失PDF。sqlc/OpenAPI/客户端二次生成字节一致（354文件），差异格式检查通过。
- **浏览器**：任务作者登录真实5174工作区，在检查页打开PDF预览，实际显示中文题面，且报告明确是r1材料、当前副本已在r2。390px浅色的检查报告/PDF预览也已实看，布局没有横向溢出。点击下载题面得到 Downloads/statement-zh.pdf，SHA-256 与真实E2E发布的PDF一致；测试文件保留，未删下载目录。没有提交/推送。

2026-09-20 第二十三轮实现：

- 标准导出支持 exact 逐字节判定器，保留 NUL、空白、大小写和末尾换行语义；不再拒绝普通精确比较，也不悄悄切换 token 比较。C++ testlib 输入/输出校验器使用标准 build/run 适配，源文件保持不变，testlib.h 与 Worker 固定提交/摘要相同，保留上游许可；ZIP 正确记录脚本可执行位。退出0映射42、正常拒绝映射43、内部故障/部分分数/崩溃为judge error，输出诊断只写judgemessage.txt。
- 适配器在独立进程运行原始程序，保留testlib finalizer和C++ main隐式返回语义。初次重命名main的方案被实际执行测试证明不可靠，已删除该方案，最终只保留标准build/run。导出报告说明适配，README明确POSIX/C++17/Python3依赖。未知语言或运行时伴随资源仍明确拒绝，非空程序参数暂未解除导出限制。
- 再导入只识别完整匹配的生成适配器契约：描述JSON、脚本正文、固定header、所有声明文件及目录文件集合；重建testlib程序而不执行脚本。回归拒绝脚本篡改、头文件替换、额外文件、尾随JSON，并在四种导出格式执行结构往返。原生归档仍不丢原材料。
- **外部互操作**：固定problemtools verifyproblem对testlib适配包和exact包均零错误/警告，正确参考解AC、错误参考解WA。独立容器实际编译并执行适配器，覆盖AC/WA/EOF/内部fail/部分分数/未quit的finalizer失败/abort，且没有teammessage泄露、临时输出已清理；exact覆盖NUL、大小写、空白、末尾换行和空文件。日志 .cache/authoring-validator-interop.log。测试夹具曾使用会被testlib严格整数解析拒绝的03作为提交输出，已修正夹具，使参考答案自身有效；额外退出语义测试仍保留03答案字节来验证不擅自重写。
- **真实导入链路**：TestEndToEndStandardPackageExecution用 .cache/authoring-testlib-legacy.zip，经HTTP私有预检/应用、真实Worker编译testlib/执行两份参考解/渲染TeX、明确提交发布，再经HTTP导出。10.58秒通过，日志 .cache/authoring-package-execution.log；导入不自动提交，检查AC/WA预期都匹配，公开题面为PDF，重新导出保留两个适配器。该测试明确禁止API-only替身。验收域 e2e-dljrnw0f3nqi，题目1000，作者user_323459385/testpass123。
- 删除内部artifact publisher接受旧平铺ZIP/checker.cpp的兼容分支及其旧扫描器/常量，上传只接受冻结manifest产物。存储测试改用真实manifest，继续覆盖稳定摘要/不同数据快照、丢失/非法文件、压缩膨胀预算、失败暂存清理和安全删除；应用层的两处旧上传夹具也迁移，数据库写失败仍保留可能被其他租约引用的产物。没有删掉失败场景测试来迁就实现。
- Server普通全量Go测试和authoring vet通过；真实PG authoring仓储/文件存储/完整API套件通过。354个生成文件未变，diff whitespace通过。本轮没有改前端或公共wire，未重复前端构建。未提交/推送。

2026-09-20 第二十四轮实现：

- 标准导出ZIP改为唯一根目录，目录名和下载文件名主体一致且仅含小写字母/数字；Identity生成稳定名字，未提供身份时从树摘要生成。原生归档保持原布局。导出器返回的Filename直接传给HTTP响应，避免文件名与目录名各自拼装。
- 按标准校验导出路径的每个组成部分，不将带空格/非标准字符的程序或附件放进消费端会忽略的位置；明确要求重命名，原生归档仍保留。回归覆盖四种格式的单根目录/元信息位置/文件名匹配与非标准附件拒绝。
- 2025-09多语言及非英语单语言使用name映射，导入将本地化标题保存在对应statement属性，导出保留翻译；默认语言采用当前基本设置标题。英文/中文往返测试通过，不再把所有语言统一覆写成英文标题。
- 审查发现私有asset和题面目录内resource可能在标准导出后被重新导入成公开附件，已明确阻止此类隐式提升；原生归档照常完整保留，新增隐私回归。
- **互操作**：调整外部工具夹具按真正ZIP根目录解包，Markdown转换/真实PDF渲染、testlib适配和exact三个legacy示例均通过固定problemtools，零错误/警告，日志 .cache/authoring-layout-interop.log。没有用解包后改目录结构掩盖ZIP布局问题。
- **真实链路**：更新任务Server后，标准根目录题包经HTTP导入、真实Worker检查/TeX发布/再次导出通过，且API下载Filename和ZIP全部条目根目录一致，日志 .cache/authoring-layout-native.log。普通Server全量Go测试/authoring vet通过，新增格式与隐私测试通过；真实PG TestPackageImportTransactions与完整TestDomainAPIIntegration通过。354个生成文件未变，diff whitespace通过。没有前端/wire变化，未重复前端构建。未提交/推送。

2026-09-20 第二十五轮实现：

- Polygon题面显式记录dialect=polygon，检查协议升级vertex-authoring-3，方言与依赖一起封入指纹。Worker在隔离XeTeX中支持常用problem环境、InputFile/OutputFile/Examples/Note等章节和多行exmp；保留原始题面字节与目录，图像按声明题面的目录归为公开statement-support，秘密数据目录的图片仍私有。未知方言明确检查失败。
- 标准导出保留原始Polygon TeX为独立文件，生成标准入口封装并将图片复制到对应子目录。封装兼容problemtools的import路径及已有Interaction定义，未靠移动原始源码或忽略编译错误造出通过。标准包的TeX辅助文件归为私有resource/purpose=statement-support，可参与渲染但不会出现在公开附件API；未声明用途的资源、程序、秘密数据仍不放入题面沙箱。
- **真实渲染**：原生Render测试涵盖Polygon标题/限制/章节、多行样例、图片，PDF经Poppler完整实看。固定problemtools对导出的legacy包零错误/警告，正确/错误参考解AC/WA，实际生成PDF并渲染核对，日志 .cache/authoring-polygon-interop.log。原文样例保持不变，外部模板会额外追加测试数据样例；此排版差异已加入导出报告，没有静默删原文内容。仍不声称任意Polygon自定义宏包完全兼容。
- **两条真实链路**：自编的常用Polygon已生成包直接HTTP导入→真实Worker校验/两份参考解/TeX→发布→标准再导出，8.86秒通过，日志 .cache/authoring-polygon-native-e2e.log；将导出的Kattis封装包重新HTTP导入、再次检查发布和导出，1.13秒通过，日志 .cache/authoring-polygon-roundtrip-e2e.log。测试原包 .cache/authoring-polygon-package.zip，Kattis包 .cache/authoring-polygon-kattis.zip。不是下载并验证了Polygon平台的任意现成题库。
- Server/Worker普通全量Go测试通过；真实PG authoring仓储/完整API套件通过；前端46文件/211测试和生产构建通过。sqlc/OpenAPI/客户端重复生成354文件一致，diff whitespace通过。原生及外部PDF分别 .cache/authoring-polygon-native.pdf / authoring-polygon-external.pdf。未提交/推送。
- 正式worker/Dockerfile完整镜像构建成功，目标localhost/vertex-authoring-worker-full:review，镜像712e82e1ec422c6070ea2e9cd99e1b60802526359d078fa7b26fff439e784fcd，日志 .cache/authoring-worker-full-build.log。正式镜像在只读根文件系统、无网络、声明tmpfs下的原生TeX/Polygon/取消测试通过，日志 .cache/authoring-worker-full-render.log。sandbox smoke首次遗漏scratch/cache挂载，按实际运行挂载补齐后通过（authoring-worker-full-smoke.log）。任务Worker现已切换到正式镜像，以只读根文件系统执行真实Polygon导入/检查/发布/再导出E2E通过（authoring-worker-full-e2e.log），没有宿主二进制覆盖镜像内Worker。

2026-09-20 第二十六轮实现：

- **真实浏览器工作副本**：同一作者的双标签页先后改题面，第二页收到409并保留本地输入；对照服务器副本后手工组合两份修改并保存。多次保存后仍基于r1，明确提交才生成r2。提交历史恢复r1只产生私人待提交改动，没有改写历史或发布。
- **真实协作合并**：通过HTTP建立任务协作者并从r2提交r3，在作者浏览器更新副本得到三方冲突。页面展示共同基线/本人/共享最新，手工结果保存到合并会话后刷新仍可继续，完成合并再显式提交r4。DOM中的最终题面与预期组合内容完全一致。另一位协作者的提交由HTTP测试夹具产生，没有声称两位作者都通过UI提交。
- 差异页由两份全文并排改为带双方行号、增删符号/柔和色块和完整文件下载的统一视图；材料属性变化单独折叠。比较算法有字符/行数/动态规划预算，超过预算显示整段替换或截断提示，不伪称完整差异。新增回归验证重复行、空行/结尾换行、双方可重建、独立改动行号与大差异预算。当前更改页移除重复的顶部提交入口；冲突对比明确“我的副本/共享最新”，不再显示内部blob字段名。
- 出题错误码映射为中文操作提示；本地FormValidationError保留具体错误，重复上传实际显示冲突路径。上传有独立进度与完成提示，不再让“更新副本”按钮代替上传转圈；上传期间程序编辑区域inert，避免同页编辑制造无谓CAS竞争。目录上传后优先显示main源文件，扩展名大小写/C语言识别修正。抽屉先聚焦标题，避免默认聚焦删除；只读/可编辑说明分开，文件大小使用B/KiB/MiB。移动菜单减少重复顶部留白。
- **目录/大文件实际操作**：浏览器文件选择器上传main.cpp、include/add.h、lib/add.cpp，原目录层级保留，源文件4→7，没有提交。又上传16MiB输入和答案，列表/抽屉显示正确大小，实际下载到Downloads/large-100.in与原文件SHA-256一致。浏览器download事件监听超时，但实际文件与哈希已独立验证，未据超时假称成功。具有题目editor权限的协作者直接请求该未提交blob，真实后端返回404。
- 大文件、二进制、PDF及附件增加替换入口。替换保持entry ID/路径和引用；上传后CAS失败会保留待保存的新blob，核对后重试不退回旧内容。实际用2MiB替换文件触发另一页修改造成的陈旧副本冲突，再从页面核对保存。HTTP复核确认新SHA匹配、entry ID不变、另一页题面修改仍在、shared head仍r4、publishedVersion仍v1。
- 热更新复现Domain context is required白屏：上下文定义从Provider/UI依赖图拆到domain/domain-context.ts，公开hook仍由原入口导出。修复后再次修改共享format依赖，页面保持可用，无新的同类错误；旧控制台记录仍保留。不是把重启服务当成修复。
- 桌面暗色差异页、390px暗/亮材料抽屉、移动侧边菜单均实看；替换/下载/关闭控件不溢出，标题焦点确认。前端47文件/214测试、生产构建通过；最后补充的ConflictEditor保护（另一项冲突保存后不覆盖当前未保存手工输入）通过TypeScript，但其多冲突浏览器情形仍需下一轮实测。354个生成文件未变，diff whitespace通过。仅前端改动，未重复后端全量或Worker构建。

2026-09-20 第二十七轮实现与验收审计：

- **多冲突草稿**：为现有任务作者和协作者构造files/validate.cpp及files/check.cpp两项真实冲突，协作者提交r5。浏览器同时编辑两份手工结果，先保存一项，另一项未保存内容保持不变，验证第26轮ConflictEditor保护。冲突卡增加可访问区域标签，可按文件定位。
- **共享头前进**：解决期间协作者另改solutions/wrong.cpp并提交r6；第二份手工结果仍在。保存后刷新会话，再完成合并，副本更新到base/head r6。通过真实HTTP核对两份手工内容、远端新增注释、2MiB数据均保留，mergeId清空且publishedVersion仍v1；未自动提交。
- **公开呈现**：扩展PublishedFiles真实E2E加入240×80 PNG、Markdown相对附件链接及有效两页PDF。真实Worker/API通过，新增图像下载字节对比，比赛继续固定旧Markdown v1，练习为PDF v2。浏览器验证PDF第二页文字、前后翻页与缩放；比赛页图片实际为授权请求获得的blob URL，naturalWidth/Height为240/80；相对链接跳到附件按钮，实际下载guide.txt内容为public attachment。没有用“只显示图标”证明文件可读。
- PDF按稳定文件ID保留加载回调，避免只因元信息对象重建就重置文档。增加30秒加载/20秒页面预览超时和任务释放，文字预览按预算逐项拼接；已按当前pdfjs API使用loading task销毁，未使用不存在的PDFDocumentProxy.destroy/isEvalSupported参数。正常翻页/渲染实测通过，未伪称执行了超时负例。
- 前端47文件/214测试、TypeScript/生产构建通过；修改后的PublishedFiles真实PG API回归通过（authoring-media-api-final.log）。本轮未改后端产品/schema，未重复Worker完整构建。Git仍未提交/推送。
- **审计发现必须补齐**：研究文档第164、181行和计划D.5明确承诺校验器正反例自测，而现实现只对data/invalid_input、invalid_output、valid_output保留阻断。它不是可以在收尾时降级为“高级未支持”的缺口，因此D/E/F仍不勾完成。A/B核心要求已有当前实现与强验证证据，先按事实勾选。

当前优先事项：实现校验器自测的完整闭环（材料描述/引用与指纹、Worker执行和异常区分、报告/发布门槛、2025-09导入导出、UI与真实E2E），不能只增加类型或解除阻断。官方规范：invalid_input只含.in且至少一个输入校验器拒绝；invalid_output/valid_output含.in/.ans/.out，输入必须有效，输出分别拒绝/接受。常规有效输入和输出自测共同覆盖输入正例。用例参数/description也要按能力准确映射或保留阻断，不能忽略。

同时在最终格式审计中补查标准TeX的Input/Output/Interaction环境与常用宏、样例插入指令；研究承诺的核心批处理路径不能以“例子能跑”替代。后续应建立逐项验收表，核对C/D/E/F，而非根据累计进度文字直接宣布完成。

验证环境：mock5173此前服务保留但在客户端生成后可能需刷新/重启。真实前端5174已重新启动，session15855，代理18081由SSH session17415转发到Podman VM18080；task pod vertex-authoring-media-pod保留media-pg，当前server/worker分别为vertex-authoring-polygon-server、vertex-authoring-full-worker，专用卷vertex-authoring-media-data。Server二进制 .cache/authoring-polygon-server；当前Worker来自正式镜像localhost/vertex-authoring-worker-full:review，旧QA运行镜像localhost/vertex-authoring-tex-runtime:xe只保留此前验证记录。旧任务media-server-fixed/media-worker/tex-server/adapters-server/layout-server/tex-worker/polygon-worker已停止，未删用户卷。

真实TeX验收域e2e-dljfm1r358nm，题目1000，作者user_744959845 / testpass123，检查acdfcc61-18c7-4c79-ab2f-8bda8844c7ec；r1已发布v1，r2修改题面尚未重新检查发布（这些旧检查使用v2，当前v3需重做检查）。最新Polygon直接验收域e2e-dljsa0weion2 / 作者user_977501265，标准往返域e2e-dljsap0bhgi2 / 作者user_464898881，均题目1000，测试密码testpass123。浏览器browser绑定有效，texQATab(tab4)为本轮任务页，之前tab2/tab3已被关闭（未重选browser）；viewport 已 reset，外观已恢复为跟随系统。taskPG vertex-authoring-pg的authoring_presentation继续用于PG回归；media-pg/authoring_media_live用于真实Server/Worker与浏览器。没有修改用户现有数据库。未提交、推送、合并。

本轮浏览器：运行时重新初始化后browser仍选iab 1；editB(tab3)已markHandoff，当前tests页，editA(tab2)已关闭，先前无法访问站点的tab1未操作。viewport已reset，主题恢复跟随系统。测试作者user_261673985/testpass123，域e2e-dljsgjbiv8ap，题目1000；shared r4、public v1，私人副本保留3个上传源文件、large-100.out和已替换成2MiB的large-100.in，以及题面末尾“concurrent edit during binary replacement”测试注释，均未提交。协作者browserqa_1789872556551583400/testpass123提交了r3，其当前副本基于r3，可用于下一轮并发测试。不要重复运行 .cache/authoring-browser-collaboration.py（会另建账号/提交）。上传夹具在.cache/authoring-ui-upload，替换前身份记录replacement-before.json，均为本任务测试文件；未重置用户数据库。Git仍未提交/推送。

本轮环境补充：后台持久Start-Process启动被自动审批拒绝，已使用可管理exec前台会话启动SSH/Vite，未绕过限制。旧失败站点tabs的data错误页被URL策略拒绝，不操作错误页；同一browser绑定新建了mergeReview和mediaReview，已markHandoff。原生媒体fixture：域e2e-dljx4i4vgf4x，题1000、比赛1，作者user_809596773；user_261673985已加入该任务域并报名，用于读固定v1 Markdown，练习v2为两页PDF。未改变本轮主题/viewport。
主协作fixture域e2e-dljsgjbiv8ap仍为user_261673985：现在shared head=6、本人copy base=6，两个resolved注释、远端wrong.cpp新增注释及原有未提交上传文件均保留，公开v1。协作者browserqa_1789872556551583400的copy基于r6。不要重跑已执行的多冲突setup脚本，它会再次改材料。


2026-09-20 第二十八轮实现：

- 补齐 D.5 的校验器自测，新增独立 `validation` 材料以及冻结输入/标准答案/候选输出引用；它不进入正式测试顺序、公开题面或选手评测。保存、差异、合并、原生归档、引用检查和数据指纹覆盖该类型。
- Worker 对 invalid_input 运行全部输入校验器，至少一次正常拒绝才通过；输出正反例要求输入先合法，再用当前比较器验证候选输出。信号崩溃、执行限制和系统失败不算正常拒绝。结果逐项带预期、实际与错误信息，检查完成和发布都要求完整匹配的成功结果。schema 修改并入 000001 的 validation_json。
- Kattis 2025-09 三类自测目录及描述导入/导出往返通过；缺少三联文件、孤立答案、非法输入带答案明确报错。未映射的逐点/组校验参数保留并阻止检查。legacy 导出不能静默丢弃自测，提示选择 2025-09 或原生归档。
- UI 在测试数据中增加独立自测列表、类型/文件引用/说明表单，检查报告单列结果并分页；mock 提供明确标注未实际执行的结果，A+B 示例补两项输出正反例。真实浏览器已创建“接受正确求和输出”、选择文件、自动保存、运行 Worker 检查并读到“实际接受 · 符合预期”，未提交副本或自动发布。
- 检查协议升级为 vertex-authoring-4。新领取/上传/发布严格校验当前策略；已发布 schema 1 产物的读取按产物 schema 和固定摘要验证，避免增加检查能力后使既有比赛版本无法判题。新增旧策略发布产物可读、未知 schema 拒绝测试。
- 新鲜任务库 authoring_validation 的 authoring PostgreSQL/API 套件通过；增加成功状态但自测结果缺失/错误时不能发布的真实 PG 回归。原生只读容器通过三类自测及“总是接受”“非法输入时崩溃”和正反例预期不符检查。真实 Server+Worker 的 FrozenAuthoringCheck E2E 通过，含公开原始 JSON 不泄漏自测与旧版本固定判题。
- 前端 47 文件 / 215 测试、生产和 mock 构建通过；358 个 sqlc/OpenAPI/客户端生成文件重复生成一致。证据在 `.cache/authoring-validation-*.log`。运行环境是任务专用 pod/数据库/卷，未重置用户库。
- 继续完成标准 TeX 环境/样例插入指令的兼容审计和最终整体验收，goal 尚未完成。此次响应式视口设置未作用到当前审阅页，已 reset，不能将此轮截图当成 390px 验收证据。


2026-09-20 第二十九轮实现与验收：

- 标准 TeX 新增 Input/Output/Interaction、可省略参数的 problemname、右侧绕排 illustration、常用列表/删除线/链接/题记，以及 nextsample/remainingsamples。渲染移到数据和参考解/校验器自测通过后，只放入公开样例；大/二进制样例不作为源码执行，保留下载提示。标题绑定快照指纹，原始题面不改。镜像补充实际缺失的 TeX 宏包及字体。
- 实际解析生成 PDF 抓到并修复了计数器提前展开导致的样例错位；原生回归现在检查每个插入点的实际展开。导出 PDF 经 PyMuPDF 验证标题、样例位置、数量、字面 TeX 样例及秘密数据不出现，实际查看页面排版。空余样例自动追加，过多 nextsample 明确失败。
- Markdown 发布支持同名样例指令，保护代码块/转义字面量，过大样例转为真实下载区域锚点。标准 Markdown 导出移除重复首行标题；不能渲染的 SVG/原始 HTML/外链图片与未映射附件目录明确报兼容阻断。legacy 无法可靠保留样例插入位置时拒绝转换。
- 当前完整 Worker 镜像 `localhost/vertex-authoring-worker-standard:review`（495d1964e894...）构建通过，原生 TeX/cancellation 和只读/无网络 sandbox smoke 通过。真实 Server+Worker+PG 的 TeX 预览/共享/发布/旧版保持、公开 Markdown/PDF/大与二进制样例、标准包导入检查发布再导出均通过。
- Server 全量真实 PostgreSQL测试、Worker 全量普通测试、两侧 vet、主原生 E2E 套件（与 CI 相同排除三个独立协议测试）、重连恢复及 DomainProtocol 通过。外部固定 problemtools 的 legacy/Markdown/Polygon/testlib/exact 五组通过，0 errors / 0 warnings。2025-09 仍不声称完整外部认证。
- 初次全量 native E2E 误将需停 Worker 的协议用例混跑，并触发默认账号限流；按 CI 分组重跑后通过，CI job 仅在测试环境提高限额，默认生产配置不变。Actions 增加 Builder/TeX 原生测试与 frozen-check E2E。
- 额外单独 JudgeFencing 命令曾被自动审批拒绝（仅给 blocked by policy），未重试该运行方式。审阅发现该旧用例停 Worker 后仍等待真实检查的缺陷，改为明确标注的协议夹具并加入独立测试数据库的内存 HTTP API 集成套件，JudgeFencing 子测试通过（含旧租约/旧代次/重复完成）；不把合成判定描述为真实执行。
- 最终需求对照补上最初调研中的平铺数据 ZIP 导出、批量限制、全屏编辑及重复数据提示。新数据导出保留二进制字节、顺序和 config.yml 逐点限制，携带清楚的数据专用提示；mock 支持无 config 的真实 ZIP 往返。新增领域/ZIP/HTTP/前端回归，前端现为 216 tests。
- 真实浏览器确认：390px 亮色 PDF 与样例下载；390px 暗色出题页/覆盖侧栏；768/1440 无横向溢出；主题恢复浅色、viewport reset。全屏进入/退出保持题面原文；两项测试批量设为 1700ms / 131072KiB 后，表单读取正确。此测试仍只是私人副本，未产生提交或发布。
- 新建独立 authoring_final_schema 完成 baseline up/down/up，无用户库重置。生成358文件曾重复一致；四项收尾功能后的最后生成/构建/浏览器数据下载核验继续进行，未提前标记goal完成。


最终验收：上述四项收尾功能已完成。当前216个前端测试、生产/mock构建、格式检查、最新Server全量真实PG测试、Worker测试、native主E2E、题包外部/真实执行互操作、独立API租约与回滚回归均通过。平铺数据浏览器下载已核对真实字节与两个限制配置；全屏未改变编辑内容。生成358文件重复一致。完整需求映射和准确边界见 [最终验收记录](../research/2026-09-20-authoring-acceptance.md)。未提交、推送或重置用户数据库。
