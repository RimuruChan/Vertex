# Sandbox 通用执行内核演进计划

## 目标

在不改变现有 container-first 安全边界的前提下，将 `vertex-sandbox` 从“普通题执行器”演进为 Worker 可复用的单进程隔离原语，为下列任务提供共同基础：

- 普通题、Special Judge 与 validator；
- 交互题和多进程通信题；
- 题目数据生成、题面/题包渲染；
- 标程与候选程序对拍、测试数据验证。

本计划不重新引入 isolate/runguard，不复制 DOMjudge GPL 源码，也不使用 mount namespace、chroot、`SYS_ADMIN` 或 `apparmor=unconfined`。

## 设计原则

### 1. Native runner 只理解执行，不理解判题

C++ runner 负责一个不可信进程树的身份切换、文件边界、系统调用限制、cgroup/rlimit、stdio、清理和原始执行元数据。它不产生 AC/WA/MLE 等业务判定，也不知道 solution、interactor、generator 等角色。

Go Worker 根据任务类型解释通用终止原因。普通题 verdict 映射继续位于 Judge 领域；数据生成和渲染任务可以直接消费相同的执行结果而不依赖 Judge 包。

### 2. 每个进程角色使用独立 sandbox

未来的交互题和通信题不把多个互不信任的角色塞入同一个 UID、workspace 或 cgroup。solution、interactor、manager 和通信节点分别使用独立 box，由可信 Go 编排层建立管道并统一取消。

这样可以避免同 UID 进程互相发信号、修改文件或共享未计量资源，也保留每个角色独立的 CPU、内存、输出和 workspace 诊断。

### 3. 任务拓扑属于可信编排层

Native runner 保持单进程树模型。未来由 Worker 中的通用 task orchestrator 描述节点、单向 channel、启动顺序和失败传播：

```text
                 trusted task orchestrator
                 /          |            \
          sandbox A     stream broker    sandbox B
          solution       byte/idle cap    interactor
```

stream broker 必须持续 drain 管道、实施每方向字节限制和空闲超时，并在任一节点失败时取消整个 task group。不能让不可信进程直接继承 Worker 的网络 socket、数据库凭据或控制 FD。

### 4. soft/hard 是每次执行的显式预算

每个 execution 可以显式提供 soft/hard CPU 与 wall limit；未提供 hard limit 时才使用 Worker policy 的默认 overshoot。Workspace 限制同样属于 execution budget，但不得超过节点 policy 的上限。

这既保留普通题的统一默认值，也允许 generator、validator 和 interactor 使用不同预算，而不用复制一套 runner。

### 5. 产物通过受控接口离开 workspace

Worker 不能直接信任用户生成的路径。通用 CopyOut/Collect 接口只接受单层文件名，拒绝 symlink 和非普通文件，限制复制字节数，并通过临时文件原子发布到不存在的可信目标。目录树、题包和大数据集后续使用 manifest 驱动的受限归档接口，不开放任意递归复制。

## 通用元数据

在保留现有 `status`、`killed` 和 limit flag 的同时，runner 输出稳定的中性字段：

- `termination-reason`：`exited`、`signal`、`time-limit`、`memory-limit`、`output-limit`、`workspace-limit`、`cancelled`、`setup-error`；
- `time-result`：`none`、`soft`、`hard`；
- `time-limit`：`cpu`、`wall` 或 `cpu,wall`；
- `stdout-bytes`、`stderr-bytes`；
- 原有 CPU、wall、memory、workspace、exit code/signal 数据。

这些字段用于调度、诊断和非 Judge 任务；数据库 verdict 仍由上层映射。

## 分阶段实施

### Phase 1：中性执行契约

- 将 meta 类型和解析从 `internal/verdict` 移入 `internal/run`；
- 引入 `Execution` + `Limits`，支持显式 soft/hard 时间和每次执行 workspace 预算；
- 增加结构化终止原因和 stdout/stderr 字节统计；
- 增加安全的单文件 CopyOut；
- 保持普通题行为和现有 meta 字段兼容。

### Phase 2：流式 I/O 与 task group

当前已完成的基础切片：

- runner 支持仅供可信调用方使用的 inherited stdin/stdout FD，stderr 继续写入受限控制文件；
- Go `RunDuplex` 启动两个独立 sandbox，并由中间 broker 双向转发；
- broker 持续 drain、保留管道 backpressure、按来源方向执行 output cap，并在 idle timeout、调用 deadline、节点资源失败或调用方取消时终止整个 task group；
- 返回左右节点独立 meta、双向字节数、task-level 终止原因，以及可选的总字节有界 transcript；transcript 同时提供方向视图和按 broker 观察顺序编号的跨方向事件；
- 单元测试覆盖双向转发、方向 output cap、idle timeout 和对端提前关闭；native smoke 覆盖 inherited FD 传递。

本阶段基础切片已完成：单元测试验证 broker 的转发、限额、超时、取消和有序 transcript，容器 smoke 验证两个独立 UID/workspace/cgroup 的 sandbox 能仅通过 inherited pipe 完成请求/响应。manager 多通道、通信题图拓扑和任务类型 adapter 属于 Phase 3。

### Phase 3：任务类型接入

- interactive/communication Judge adapter；
- generator/validator/package renderer adapter；
- stress runner 与可复现 seed、失败样例归档；
- 根据任务类型配置独立 policy ceiling 和调度队列。

## 不变量

- Docker 默认 seccomp/AppArmor、只读 rootfs 和 capability 白名单保持不变；
- Landlock 不可用、必要 cgroup controller 不可用或显式 cpuset 无法应用时失败关闭；
- 不可信进程不能访问 Server 凭据、Worker API token、testdata 根目录、cache 或其他 box；
- 每个 box 同一时间只允许一个 execution，结束后清理整个 cgroup 进程树；
- 新增能力必须有 Go 单元测试、native smoke test 和明确的 meta/错误语义。
