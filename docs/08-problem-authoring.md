# 出题(题目包)设计

页面入口为导航栏「出题」(`/authoring`)，单题工作台使用 `/authoring/{公开编号}`。后端已改为域与题目协作授权：owner、协作者和域资源管理者按能力访问。前端导航/路由仍有旧 admin 限制，完整 capabilities 驱动界面在 P3 接入；mock 通过独立账号切换视角，不提供专门的出题演示页。

题目使用独立且必填的 `owner_id`；`author_id` 保留最初创建者，不参与可变所有权判断。reader 可审阅包，editor 可修改源材料和构建；变更可见性、删除、转让和管理协作者需 owner 或域资源管理权限。每次关键写入按账号 → 域 → 题目的顺序锁定并重查权限，组成员/域角色修改等待域共享锁释放。转让不改变公开编号或创建者，也不自动保留旧 owner 权限。

工作副本、候选数据和发布版本已经分开。保存题面或元信息不改变当前公开内容；上传 ZIP 或构建成功只更新候选数据。owner/域资源管理者在「发布」页确认工作 revision、候选版本及语言后，才原子切换公开投影。可见性是独立的资源访问设置，发布私有题目不会自动把它公开。

Vertex 的出题流程对标 [Polygon](https://polygon.codeforces.com/):题目不是「一段题面 + 一个 zip」,
而是一个**可构建的题目包**——结构化题面、testlib checker/validator/generator、标程与其它解、
测试点计划。构建在判题沙箱里跑完整流程，生成可供审核发布的候选数据。

## 工作副本、候选与不可变发布

直接上传 zip 有三个长期问题:

1. **输入与答案会不同步**。答案是人工产生的,改了输入却忘了重算答案不会有任何报错。
2. **无法回答「这份数据是怎么来的」**。生成器、参数和标程都不在系统里。
3. **改数据会静默改变判定**。正在判的提交可能读到一半被换掉的数据。

Vertex 的解法:

- `problem_workspaces` 保存可变元信息，`problem_statements` / `problem_files` / `problem_tests` 是源材料；公共 `problems` 字段仅在发布时更新；
- `problem_testdata` 是当前候选，可来自完成的构建或预制 ZIP 导入，不是判题读取入口；
- `package_revision` 跟踪全部材料修改，`data_revision` 跟踪程序、测试计划、限制等判题材料修改；题面文案不变更 data revision，因此可复用匹配的候选；
- `problem_versions` 保存不可变发布快照。发布校验所见 revision 与候选版本，重复发布同一组合幂等，过期请求返回 `409`；
- 产物目录按内容哈希寻址(`<testdata_root>/<problemId>/<sha256>/`),不可变,
  judge generation 在创建时绑定发布版本，领取和重领都读取相同的限制与数据。比赛编排固定版本，普通保存不升级；赛务须显式采用新版本，再决定是否重测旧提交。

```text
出题人编辑                     构建 worker(沙箱)                 判题
─────────                     ──────────────────                 ────
题面 / 源文件 / 测试点  ──►  编译 → 生成 → 校验 → 标程 → 自检 → 对拍 → 打包
                                              │
                                              ▼
                                       候选数据 ──► owner 确认发布 ──► 不可变版本 ──► judge generation
```

## 题目包的组成

| 角色      | 表 / 字段                                            | 语言             | 说明                                                             |
| --------- | ---------------------------------------------------- | ---------------- | ---------------------------------------------------------------- |
| 题面      | `problem_statements`                                 | —                | 按语言分行,分段存储;`problems.statement_language` 指定渲染哪一份 |
| checker   | `problem_files` kind=`checker`                       | C++              | testlib 特殊判定;缺省用内置的忽略行尾空白比较                    |
| validator | `problem_files` kind=`validator`                     | C++              | 构建时对每个输入运行一次,失败即整次构建失败                      |
| generator | `problem_files` kind=`generator`                     | C++ / Python     | 由测试点的生成命令按名字调用                                     |
| 标程      | `problem_files` kind=`solution`,`is_active`          | C / C++ / Python | 产生每个测试点的答案                                             |
| 其它解    | `problem_files` kind=`solution` + `expected_verdict` | C / C++ / Python | 构建期对拍,验证数据强度                                          |
| 测试点    | `problem_tests`                                      | —                | `manual` 存输入文本,`generator` 存一条生成命令                   |

checker / validator / interactor 限定 C++,因为 testlib 是一个 C++ 头文件。
生成器与解可以用任意受支持的判题语言;Python 生成器无法使用 testlib,但对小规模构造够用。

**答案永远不能人工上传**,只能由标程产生。这条约束是「输入与答案不同步」问题的根治办法。

## 构建流水线

worker 领到构建任务后,在与判题相同的 `vertex-sandbox` 中依次执行:

| 阶段        | 动作                                                 | 失败含义                                 |
| ----------- | ---------------------------------------------------- | ---------------------------------------- |
| `compile`   | 编译 checker、validator、全部生成器与全部解          | 源码有编译错误                           |
| `generate`  | 手工测试点直接落盘;生成器测试点执行 `argv` 取 stdout | 生成器崩溃/超限/命令引用了不存在的生成器 |
| `validate`  | 对每个输入运行 validator(输入走 stdin)               | 数据不满足题目约束                       |
| `answer`    | 标程读输入产生答案                                   | 标程崩溃、超时或非零退出                 |
| `check`     | 用 checker 判定**标程自己的输出**                    | checker 读不懂它要判的输出格式           |
| `solutions` | 其它解按题目限制跑一遍,与 `expected_verdict` 比对    | 数据太弱(错解通过)或标程有问题           |
| `package`   | 打包 `N.in` / `N.out`(+ `checker.cpp`)上传           | 打包或上传失败                           |

任何阶段失败都会终止构建并把原因写回构建报告;**已发布的数据保持不变**。
局部成功不会发布——发布半套数据等于悄悄改变判定标准。

### 生成命令

生成命令是一行 Polygon 风格的文本,例如 `gen 100000 1000000000`。
第一个词是生成器名,其余作为 `argv` 原样传入。命令**不经过 shell**:
服务端与 worker 各自独立校验 token(`^[-A-Za-z0-9_.,=:+/@\[\]]{1,64}$`),
因此 `;`、`$()`、反引号、引号都会被拒绝。testlib 的 `registerGen` 用 `argv` 播种,
相同参数总是产生相同数据。

### 对拍(invocation)

带 `expected_verdict` 的解会按题目自己的时间/内存限制跑一遍全部测试点,
遇到第一个非 AC 即停止(与真实判题一致)。`Any Rejection` 表示「只要不是 AC 就算符合预期」。
所有解都符合预期,构建才算成功——这是数据强度的自动化回归测试。

## testlib 集成

- worker 镜像按 **commit 固定 + sha256 校验**下载 `testlib.h`(见 `worker/Dockerfile`),
  路径由 `TESTLIB_PATH` 指定,默认 `/usr/local/share/vertex/testlib.h`。
- 编译时把 `testlib.h` **复制进沙箱 workspace**,用 `-I.` 引用。
  没有任何 include 路径指向沙箱之外。
- 编译缓存键包含 testlib 摘要,升级头文件会自动作废旧产物。
- checker 以**源码**形式随测试数据快照发布(`checker.cpp`),判题节点用自己的工具链现编。
  构建节点因此无法把一个外来二进制送到判题节点上执行。

### 判题时的 checker

`problem_testdata.checker = 'testlib'` 时,判题 worker:

1. 读取快照里的 `checker.cpp`,用编译缓存编译(每个版本只编一次);
2. 每个测试点运行完选手程序后,把 stdout 从沙箱工作区拷出;
3. 重置沙箱,复制 `input / output / answer` 进去,运行 `./checker input output answer`。

退出码映射:

| testlib 退出码                            | 判定                                  |
| ----------------------------------------- | ------------------------------------- |
| 0 `_ok`                                   | Accepted                              |
| 1 `_wa`                                   | Wrong Answer                          |
| 2 `_pe` / 4 `_dirt` / 8 `_unexpected_eof` | Wrong Answer(附 checker 说明)         |
| 7 `_points`                               | Wrong Answer(尚未支持部分分)          |
| 3 `_fail`                                 | System Error(评测方错误,不是选手的错) |
| 其它 / 自身超限                           | System Error                          |

checker 自身超时或超内存一律记 System Error:它没有对选手程序作出任何判断。

## 构建任务的调度与围栏

构建队列复用判题队列的模型,`problem_build_jobs` 与 `judge_jobs` 结构同构:

- `FOR UPDATE SKIP LOCKED` 领取,分配 `lease_token`;
- 每次进度上报都续租,并且必须带齐 (build, worker, lease token) 三元组;
- 租约过期会被其它 worker 接管,超过 `maxBuildAttempts` 标记 `dead`;
- 上传产物与写结果都是围栏写入,过期的 worker 写不进任何东西;
- 上传路径中的 problem ID 由服务端根据 live build lease 解析，worker 传入的 query 只用于一致性校验；跨题 ID、目录穿越和绝对路径在落盘前拒绝；
- 部分唯一索引 `ux_problem_build_jobs_active` 保证一道题同时只有一个未完成构建,
  重复点击「构建」返回正在跑的那一个而不是排第二个。

构建输入在排队时写入 `input_json`，重试不换输入，并清空旧尝试上传状态。上传仅把内容寻址目录落盘并记录在受 lease 保护的构建行上；成功完成后只有 data revision 仍匹配时才更新候选。显式发布才写 `problem_versions` 并切换公开题面、限制、标签和版本指针，既有比赛及任务不受影响。
若围栏写库失败，本次新建的内容寻址产物会补偿删除；已存在且可能被有效构建引用的同哈希产物不会误删。

## 题面渲染

公开页读的仍然是 `problems.statement_md`,由结构化题面渲染而成:

```text
## 题目描述     ← legend
## 输入格式     ← input_format
## 输出格式     ← output_format
## 样例         ← 最近一次成功构建里 is_sample 的测试点(输入 + 标程答案)
## 计分方式     ← scoring
## 说明与提示   ← notes
```

- 样例**不是手写的**,而是构建产生的输入与标程答案,因此题面上的样例一定能被判题接受。
- 样例用围栏代码块渲染,围栏长度按内容里最长的反引号串自动加长,数据无法逃逸出代码块。
- 只改题面不需要重建匹配的数据候选；保存只更新工作副本，发布时使用候选样例渲染公开题面。
- `tutorial` 段只保存,不进入公开题面。

## 跨域复制

复制明确选择的发布版本，要求源题 `copy` 与目标域 `problem.create` 同时成立。副本默认草稿、未发布，复制人成为新 owner；题面、标签、全部源程序（含未激活程序）、测试计划与样例来自该版本，不读取后来的工作修改。源协作权限、提交、比赛引用与构建历史不复制。

数据使用独立普通文件，不建立指向源题的硬链接或符号链接；校验路径、完整测试对、大小和哈希后才完成事务。源题删除不影响副本，副本需再次显式发布才可评测。来源与复制说明保存在只供包协作者读取的不可变记录中，重复复制会保留已有说明；复制权限不是对材料许可证的自动认定，操作者仍应遵循源材料授权。

工作台「复制与来源」提供源发布版本、目标域与来源说明，确认后直接进入目标题目详情。目标域只列出有创建权限且未归档的域，可按名称查找；源版本由后端再次校验。副本详情保留来源域、公开编号、版本与说明，不自动公开私有来源记录。

「协作权限」显示 owner 的用户名，以及用户直接授权、group 授权来源。编辑协作者不能发布、授权或转让；reader 可阅读题面、完整源程序、测试定义和构建记录，但没有保存、上传或构建操作。只读输入仍可选择复制。

## 相关接口

正常出题接口使用 `/api/domains/{domain}/admin/problems/{id}`，旧 `/api/admin/problems/{id}` 固定官方域。`admin` 前缀不代表必须是站点管理员；路由要求登录，领域服务/Store 按实际资源权限检查。新建题目还要求当前域的 `problem.create` 能力。以下以旧官方域兼容地址简写：

```text
GET    /api/admin/problems/{id}/package                  一次取回整个工作区
GET    /api/admin/package-templates                      内置 testlib 模板
PUT    /api/admin/problems/{id}/statements/{language}    保存工作题面
POST   /api/admin/problems/{id}/statements/{language}/preview  预览渲染结果
GET    /api/admin/problems/{id}/files                    列出源文件(不含正文)
GET    /api/admin/problems/{id}/files/{fileId}           取单个源文件正文
PUT    /api/admin/problems/{id}/files                    按 (kind, name) upsert
DELETE /api/admin/problems/{id}/files/{fileId}
GET    /api/admin/problems/{id}/tests                    测试点计划
GET    /api/admin/problems/{id}/tests/{testId}           完整测试定义，编辑前读取
POST   /api/admin/problems/{id}/tests
PUT    /api/admin/problems/{id}/tests/{testId}
DELETE /api/admin/problems/{id}/tests/{testId}           删除后自动顺延编号
POST   /api/admin/problems/{id}/tests/{testId}/move      调整顺序
POST   /api/admin/problems/{id}/builds                   触发构建
GET    /api/admin/problems/{id}/builds                   构建历史
GET    /api/admin/problems/{id}/builds/{buildId}         构建状态与报告
POST   /api/admin/problems/{id}/builds/{buildId}/cancel
POST   /api/admin/problems/{id}/publish                明确发布所审核的工作/候选版本
GET    /api/admin/problems/{id}/releases               不可变发布记录
GET    /api/admin/problems/{id}/access                 查看直接授权及 group 来源
PUT    /api/admin/problems/{id}/access                 授予用户或 group reader/editor
DELETE /api/admin/problems/{id}/access/{grant}          移除指定授权，不消除其他来源
PUT    /api/admin/problems/{id}/owner                  转让给同域有效成员
GET    /api/admin/problems/{id}/origin                 包协作者可见的复制来源
```

工作区与测试点列表只提供最多 512 字符的输入预览；不能用该预览作为完整输入回写。测试编辑器先读取单个完整定义，避免仅修改备注时截断长数据。跨域复制使用 `POST /api/domains/{domain}/problem-copies`，路由域是目标，请求体明确指定来源域、题号和版本。

构建 worker 协议与判题 worker 共用凭据和 base URL:

```text
POST /internal/judge/v1/builds/claim                     长轮询领取构建任务
POST /internal/judge/v1/builds/{buildId}/progress        续租 + 上报阶段进度
POST /internal/judge/v1/builds/{buildId}/package         上传产物(octet-stream)
PUT  /internal/judge/v1/builds/{buildId}/result          围栏写入终态
```

## 配置

| 变量                      | 位置   | 默认                                | 说明                                        |
| ------------------------- | ------ | ----------------------------------- | ------------------------------------------- |
| `BUILD_LEASE_TTL`         | server | `120s`                              | 构建租约;必须大于 `JUDGE_LONG_POLL_TIMEOUT` |
| `BUILD_WORKER_ENABLED`    | worker | `true`                              | 关闭后该节点只判题不构建                    |
| `BUILD_PROGRESS_INTERVAL` | worker | `15s`                               | 进度上报(即续租)间隔                        |
| `TESTLIB_PATH`            | worker | `/usr/local/share/vertex/testlib.h` | testlib 头文件路径                          |

启用构建的 worker 会多占用一个 sandbox box(`SANDBOX_BOX_ID + JUDGE_WORKERS`),
构建因此不会和判题抢同一个工作区。

## 已知边界

- **交互题**:`interactor` 已进入包结构与校验,但交互判题本身尚未开放。
- **部分分**:测试点的 `points` 与分组会写进 `problem_testdata.config_json`,
  判题侧目前仍按「首个非 AC 即最终判定」聚合。
- **资源文件**:暂不支持题面图片等附件。
- **包导入导出**:暂不支持导入 Polygon 包。
- **mock**:构建、判题和 ZIP 导入只模拟工作流与版本状态，不执行程序、不解析或校验真实数据包，也不回退真实 API。

## 参考实现

- 服务端领域层:[`server/internal/modules/authoring`](../server/internal/authoring)
- 构建流水线:[`worker/internal/builder`](../worker/internal/builder)
- testlib checker:[`worker/internal/checker`](../worker/internal/checker)
- 前端工作区:[`ui/src/pages/admin/ProblemWorkspacePage.tsx`](../ui/src/pages/admin/ProblemWorkspacePage.tsx)
- 端到端测试:[`server/e2e/authoring_test.go`](../server/e2e/authoring_test.go)
