# 主流开源评测系统沙箱调研与 Vertex 重设计建议

调研日期：2026-09-08。状态：调研与候选设计，尚未选型或迁移。

后续实现更新：用户接受 privileged Worker 后，已保留原生沙箱并将 cgroup 初始化改为 Worker 自动管理本容器内部子树，取消预建宿主目录和强制 AppArmor，统一通过 Compose 部署。下文“Vertex 当前设计”记录的是调研时的旧实现；最新边界以 [沙箱设计](../02-judge-sandbox.md) 为准，未接入其他执行后端。

范围：DOMjudge、CMS/Isolate、Judge0、Hydro/go-judge、DMOJ、QDUOJ/Judger。这里的“主流”指有代表性的开源实现，不是市场占有率排名。优先核对官方文档、部署文件与源码；没有实际运行这些外部执行器，也没有进行安全审计或性能基准测试。引用的 main/master/分支文档会变化，最终接入需锁定 release、源码 revision 和镜像 digest。尤其 DOMjudge main 文档为开发版，不能把它的 cgroup 要求外推到所有历史版本；Judge0 所携带 Isolate 也不能直接等同于 Isolate 最新主线。

## 结论

Vertex 应优先重新设计执行后端的接入和部署边界，保留现有任务调度、租约、评测编排和判定逻辑。暂不推荐继续扩展自研沙箱以覆盖所有容器环境，也不推荐在每个测试点的执行路径中引入集群调度。

调研项目的共同经验是：把执行能力、工具链和系统配置封装成可复用的组件或安装流程。部署命令短，并不意味着运行权限少。DOMjudge、Judge0、go-judge 的官方容器方案均包含高权限配置；DMOJ 和 QDUOJ 展示了另一类以系统调用控制、进程限制和监督为主的实现。

针对 Vertex 有两条候选路线：

1. **优先验证成熟执行服务 go-judge**：接口与现有编译、checker、generator、交互执行需求较接近，可减少底层隔离代码维护。但其官方 Docker 快速启动使用 privileged；不能承诺在非特权容器里直接替换就能运行。
2. **如果禁止 privileged、宿主 cgroup 挂载和节点改动是硬条件，优先验证 DMOJ 的跟踪式执行路线**：普通用户运行有官方依据，但容器仍可能需要 SYS_PTRACE 或 seccomp 配置；ARM64、多进程资源语义和编译器策略必须验证。不是把 Vertex 的 cgroup 代码删除就得到 DMOJ。

以上是工程建议，不是已经证明的部署结果。

## 1. 对比

| 系统 | 执行与隔离方案 | 资源约束/计量 | 官方部署要求及对 Vertex 的意义 |
|---|---|---|---|
| DOMjudge | judgedaemon 调用 runguard；编译与执行进入 chroot，切换专用运行用户 | cgroup 与运行监督；当前主线要求 cgroup v2 | 要求 root；judgehost 容器示例使用 privileged 和宿主 cgroup 挂载。适合可管理的评测节点，不是零宿主依赖方案 |
| CMS / Isolate | 使用 Isolate；namespace 限制可访问对象、目录映射、执行 UID，另有针对性系统调用限制 | 支持 cgroup 模式；当前 Isolate 主线使用 v2，systemd 委派 scope | Isolate 按 setuid-root 方式设计；需要完成委派配置。成熟执行器仍有宿主前提 |
| Judge0 | 代码执行 API/任务服务使用 Isolate | 通过 Isolate 参数设置运行限制 | 当前仓库 Compose 为 server、worker 配置 privileged。它封装了完整执行服务，不只是一个低权限库 |
| Hydro / go-judge | Hydro 调用独立沙箱 API；go-judge 使用 Linux namespace、受限根文件系统、环境池 | cgroup，支持不同环境下的 rlimit/rusage 回退；可禁止回退 | 官方 Docker 示例使用 privileged，支持容器内 cgroup 层级初始化。最值得参考的是复用执行环境与服务边界 |
| DMOJ | cptbox：ptrace/seccomp 配合，按语言和文件路径执行访问策略 | tracer、rusage 和进程资源限制；不能未经验证就认定与任意进程树 cgroup 统计等价 | 官方说明 Linux 可用普通用户运行；Docker 示例添加 SYS_PTRACE。环境要求较少，但运行策略复杂 |
| QDUOJ / Judger | C runner：fork、降 UID/GID、按语言加载 seccomp 规则 | setrlimit、wait4/rusage、超时线程 | newnew 源码要求 runner 为 root；2.0 Compose 无 privileged、无 cgroup 挂载，使用只读 rootfs 与部分 capability drop。部署较轻，但多进程统计等语义需单独评估 |

来源：[DOMjudge 安装](https://www.domjudge.org/docs/manual/main/install-judgehost.html)、[DOMjudge 容器](https://hub.docker.com/r/domjudge/judgehost/)、[Isolate 手册](https://www.ucw.cz/isolate/isolate.1.html)、[CMS/Isolate 关系](https://github.com/ioi/isolate)、[Judge0 配置](https://github.com/judge0/judge0/blob/master/judge0.conf)、[Judge0 Compose](https://github.com/judge0/judge0/blob/master/docker-compose.yml)、[Hydro 执行适配](https://github.com/hydro-dev/Hydro/blob/master/packages/hydrojudge/src/sandbox.ts)、[go-judge 设计](https://docs.goj.ac/design)、[DMOJ README](https://github.com/DMOJ/judge-server)、[DMOJ tracer](https://github.com/DMOJ/judge-server/blob/master/dmoj/cptbox/tracer.py)、[QDUOJ runner](https://github.com/QingdaoU/Judger/blob/newnew/src/runner.c)、[QDUOJ child](https://github.com/QingdaoU/Judger/blob/newnew/src/child.c)、[QDUOJ 2.0 Compose](https://github.com/QingdaoU/OnlineJudgeDeploy/blob/2.0/docker-compose.yml)。

## 2. 最值得借鉴的差异

### Hydro / go-judge：编排与执行分离

Hydro 将执行参数转换为沙箱请求，将执行结果转换为上层结果。go-judge 的 API 提供命令、输入输出、文件缓存、CPU/墙钟/内存/进程预算以及进程间管道映射；适合承接编译、运行和交互通信。[Hydro 源码](https://github.com/hydro-dev/Hydro/blob/master/packages/hydrojudge/src/sandbox.ts)、[go-judge API](https://docs.goj.ac/api)

环境池降低重复准备执行环境的成本。这里的“容器”是执行器内部的隔离环境，不意味着每个测试点都要通过容器引擎创建一个新容器。[go-judge 概览](https://docs.goj.ac/)

需要明确的配置差异：

- 官方安装例子的 privileged 用来支持内部容器嵌套，并没有消除外层高权限风险。[安装说明](https://docs.goj.ac/install)
- 无 cgroup 权限时可能回退到 rlimit/rusage，`-no-fallback` 可要求失败退出；Vertex 不能静默接受计量能力变化。[配置说明](https://docs.goj.ac/configuration)
- README 明确 seccomp 过滤不是默认启用；不能因为 Vertex 当前有 seccomp 就假定替换后也保留相同规则。[README](https://github.com/criyle/go-judge)
- `Accepted` 在执行 API 中表示程序正常结束，不能直接映射成题目答案正确；仍须 checker。服务端本地文件路径参数和文件缓存生命周期也应由可信适配层控制。[API](https://docs.goj.ac/api)

### DMOJ：不依赖 root 的路线存在，但不是简单 rlimit

DMOJ 官方明确支持 Linux 普通用户执行，容器启动示例添加 SYS_PTRACE。其隔离策略检查文件访问和部分系统调用参数，拒绝 socket，并具有不同 ABI 的处理逻辑。这是可参考的较低部署权限路线，而不是“任意程序在默认 Pod 中都能安全运行”的证明。[README](https://github.com/DMOJ/judge-server)、[文件与调用策略](https://github.com/DMOJ/judge-server/blob/master/dmoj/cptbox/isolate.py)、[ABI 与监督实现](https://github.com/DMOJ/judge-server/blob/master/dmoj/cptbox/tracer.py)

Vertex 若选择此路，需验证路径解析、软链接竞态、子进程追踪、线程、取消与编译器文件访问；不同语言和架构的策略维护成本不能省略。不能把历史“纯 ptrace 每个系统调用都停顿”的性能描述直接套用到当前 ptrace/seccomp 组合，更不能凭文档断言某实现快多少。

### QDUOJ：较轻部署对应不同资源语义

审查的是 Judger 的 newnew 分支，不是旧 master。runner 调用 wait4 读取 rusage；child 设置 RLIMIT_AS、RLIMIT_CPU、RLIMIT_NPROC、RLIMIT_FSIZE 等，并按配置降权和加载语言规则。该实现能说明不强依赖自建 cgroup 的判题系统确实存在。[runner](https://github.com/QingdaoU/Judger/blob/newnew/src/runner.c)、[child](https://github.com/QingdaoU/Judger/blob/newnew/src/child.c)

但这些数据不应直接解释为“任意后代进程的同时驻留内存总和”：地址空间限制和 RSS、单进程计量和整个执行树计量是不同口径。该判断来自对上述调用方式的分析。不能仅为移除环境要求而不加定义地改变 MLE/TLE 语义。

### Isolate：namespace 是主边界，不是全面 syscall allowlist

当前手册明确以 namespace 限制可访问对象，同时禁止部分未充分 namespace 化的接口（如 keyring、VSOCK、io_uring 等），而不是普遍禁止大多数系统调用。这与 Vertex 使用 Landlock 路径规则和 syscall 禁止列表的组合不同。[Isolate 手册](https://www.ucw.cz/isolate/isolate.1.html)

因此，各项目不能简单按“使用了几种安全机制”排序；应检查边界、默认配置及逃逸后可接触的权限。

## 3. Vertex 当前设计的问题

本节依据当前仓库，而非外部项目。

- **底层实现维护面大。** 自研 C++ 同时承担文件策略、系统调用策略、降权、cgroup 生命周期、计量、输出与目录限制、进程清理。参见 [security.cpp](../../sandbox/src/internal/security.cpp)、[cgroup.cpp](../../sandbox/src/internal/cgroup.cpp)、[supervisor.cpp](../../sandbox/src/internal/supervisor.cpp)。
- **为规避容器内 mount 而改用 Landlock，仍保留宿主 cgroup 迁移。** 结果没有获得普通容器可运行的部署体验，却承担了另一套隔离策略的维护成本。设计历史见[当前沙箱文档](../02-judge-sandbox.md)。
- **旧设计的外层与内层资源边界不一致。** 执行进程移入宿主 sibling 子树后，不能再依赖 Worker 容器的 CPU/内存上限覆盖这些进程；需要额外聚合预算。后续已改为容器内部子树，见 [当前部署配置](../../docker-compose.yml)。
- **Worker 业务代码依赖具体 Sandbox 和本地 box 路径。** `compile`、`executor`、`builder` 使用 `*run.Sandbox`；替换执行器还需替换文件传递语义，不能只替换一条 exec 命令。见 [执行封装](../../worker/internal/run/sandbox.go)。
- **调研时部署验收不完整。** 当时本机只运行 Web/API，不能代表整个 OJ 可用。后续 Worker 的 ARM64 编译、核心判题、出题构建和沙箱冒烟已验证，部署入口以 [Compose 文档](../06-deployment.md) 为准。

值得保留的部分：任务 lease/fencing、HTTP 长轮询、无数据库凭据的 Worker、编译缓存、testlib/出题流程、中性的 Execution/Limits 与结果判定分离、既有安全冒烟用例。隔离验收目标也应保留；它们不必与 C++ 实现绑在一起。

## 4. 重设计候选

建议的逻辑边界：

```text
Server（数据库、题目、任务租约）
  ↑ claim / heartbeat / result
Worker（编译流程、测试点、checker、结果映射）
  ↓ 受限执行请求：命令、文件句柄、预算、取消
Runner（成熟执行后端、工具链、资源统计、执行环境回收）
```

这是职责分离，不要求额外部署一个队列、数据库或集群控制器。Runner 可以作为同机独立进程或配套服务发布。

### 候选 A：go-judge 适配器

优先理由是减少底层隔离维护并匹配现有多程序执行需求。先验证部署权限是否可接受，再决定投入接入。不要把需要较高权限的 Runner 与持有业务凭据的 Worker 混在一个权限边界内；即使分开，privileged Runner 被攻破仍可能影响节点，不能宣称仅靠分容器消除风险。

适配层要负责：

1. 把当前 `Execution/Limits` 转换为执行器输入，统一 ns/ms、byte/KiB 等单位。
2. 使用受控文件上传/缓存 ID；不让用户直接传执行器宿主绝对路径、挂载配置或任意文件导出目录。
3. 将编译器、checker、validator、generator、交互两端都纳入受限执行。
4. 定义 CPU/墙钟 soft/hard 语义、内存统计口径、输出截断与结果错误分类。无法无损映射的语义显式修改或拒绝，不伪造旧 meta。
5. 管理缓存文件、并发环境、任务取消和 Runner 重启后的残留清理。
6. 读取能力与版本，要求关键能力实际有效；不把 cgroup 回退视作完整隔离。

go-judge API 的管道支持只是交互题的基础；Vertex 现有 idle timeout、双向限流、transcript 和取消联动仍须逐项适配。[API](https://docs.goj.ac/api)

### 候选 B：DMOJ 风格执行器

在硬性禁止 privileged 与宿主 cgroup 委派时优先验证。应先确定能否复用成熟执行模块或进行小范围适配，避免重新实现一个 ptrace tracer。源码许可和分发方式须在引入时核对，不在本调研中作许可证兼容性的法律结论。

必须先确定支持的语言、线程/子进程模型及内存计量契约；不能承诺用 rlimit 完整复刻当前 cgroup 进程树预算。容器对 ptrace 的允许方式也要在本机验证，普通用户支持不等于默认 seccomp 容器零配置支持。

### 暂不优先的方向

- 每个测试点创建集群任务：把调度、文件输送、权限、回收与指标映射引入热点路径。本次调研没有给出其适合 Vertex 的性能证据。
- 每个测试点启动独立 Docker 容器：可能实现，但需要额外管理生命周期；不能凭直觉判定开销可接受，也不应把 Docker socket 暴露给持有用户输入的通用执行接口。
- 直接删除 cgroup/Landlock 检查：不能解决完整进程树预算和隔离语义问题。
- 继续给自研沙箱叠加多个自动降级模式：部署容易表面成功，实际能力却不一致。能力不足应在接收任务前发现。

## 5. 选型必须回答的部署问题

“不调环境”要落实为验收条件，而不能只停留在口号：

| 条件 | 影响 |
|---|---|
| 不修改宿主内核启动参数、不重建 VM、不替换 containerd | 应作为当前本机 PoC 的目标 |
| 允许安装包/Compose/Chart 自动设置容器级权限 | 某些成熟 Runner 有机会满足，但需实测 |
| 连 privileged、额外 capability、seccomp profile 调整也不允许 | go-judge 官方启动方式不满足；DMOJ 也不能未经验证承诺满足 |
| 必须支持多进程总内存硬限制与准确任务清理 | 不能把 rlimit/rusage 回退冒充同等 cgroup 能力 |
| 必须抵御共享内核漏洞 | 本报告中的普通 Linux 进程/容器方案均不构成虚拟机级保证 |

前面对“普通容器里无需调整即可拥有完整评测隔离”的说法过于乐观。能合理承诺的是：收窄支持矩阵、由安装流程封装已验证的配置，让用户无需理解底层细节；不能在 PoC 前承诺任意容器运行时都无差别运行。

## 6. 最小验收实验

在改造业务代码之前，锁定候选执行器版本，在**现有 ARM64 Podman**上验证；保持现有 Web/API/数据库部署可用。调查阶段没有对外部候选执行器执行以下实验，也没有新增高权限服务。

- 启动：不重建 VM、不更改内核启动参数；完整记录必需的容器权限、挂载、运行时设置。若必须改运行时，明确判定“不满足当前目标”。
- 语言：C、C++、Python 编译/执行；非法源码、头文件探测、编译资源超限。
- 判定：真实 AC、WA、CE、RE、CPU TLE、墙钟 TLE、MLE、OLE。
- 进程：多线程、多进程、主进程提前退出、fork 放大、任务取消、Runner 被终止后无残留。
- 文件：读取凭据/其他任务数据失败；路径穿越与软链接导出失败；文件数量、容量和日志有界。
- 网络：TCP、UDP、Unix socket/VSOCK 等适用接口按明确策略检查，不能只测试一个 TCP 请求。
- 多程序：checker、generator、validator 和交互通信；两端失败、死锁、输出超限和取消一致。
- 稳定性：连续执行与并发执行的资源回收、文件缓存清理；比较短任务延迟和现有 verdict，记录实际数据，不预设吞吐结论。

通过后才替换 `run.Sandbox` 的耦合点，先迁移普通编译/评测，再迁移出题构建与交互功能。最终验收应是安装后 Worker 正常领取任务并完成真实评测，而不只是进程启动。

## 7. 安全维护启示

Judge0 官方曾披露 SSRF 与沙箱逃逸组合的安全问题，并说明 privileged 容器会扩大后果。这是历史修复事项，不表示当前版本仍存在该漏洞。对 Vertex 的启示是：执行 API 的文件/网络输入校验、凭据边界和执行器更新同样重要，不能仅统计启用了多少 Linux 安全机制。[官方安全公告](https://github.com/judge0/judge0/security/advisories/GHSA-q7vg-26pg-v5hr)

最终建议：**先验证成熟 Runner 的实际部署体验，再重构执行接口；不要先重写一整套沙箱，然后再处理部署兼容性。**
