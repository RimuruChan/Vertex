# Vertex Worker

`worker` 是 Vertex 的后台任务执行进程。它通过 Server 内部 API 长轮询领取带租约的任务，在原生沙箱中执行，再回传结构化结果；Worker 不连接 PostgreSQL。当前实现两类任务：

- **判题**：编译、逐测试点隔离执行、checker 判定。
- **题目包构建**：编译 testlib checker/validator/generator 与各解，生成并校验输入，用标程产出答案，构建期对拍，最后打包上传测试数据。

Go module：`github.com/RimuruChan/Vertex/worker`

目录使用通用的 Worker 命名；两条任务循环共用同一套沙箱、编译缓存与租约围栏模型，按需创建独立隔离环境。

## 运行要求

执行不受信任代码必须使用满足以下条件的 Linux 环境：

- cgroup v2。
- Landlock ABI 1 或更高版本。
- rootful Docker Engine 与 Docker Compose。

普通 Go 单元测试可以在 Windows/macOS 运行，但原生 `vertex-sandbox` 和完整 Worker 不能在原生 Windows 容器中运行。推荐使用仓库根目录的 Docker Compose 配置，不要直接以宿主 root 身份运行提交代码。

## 工作流程

```text
judge:  claim job ───> compile/cache ──> vertex-sandbox ──> checker ──> report result
                              │                  │
                              └──── heartbeat/lease ───┘

build:  claim build ─> compile 源文件 ─> generate ─> validate ─> 标程答案 ─> 自检 ─> 对拍 ─> 上传产物
                              │                                                          │
                              └──────────────── progress/lease ──────────────────────────┘
```

- `internal/client` 实现 claim、heartbeat 和 result HTTP 协议。
- 判题 claim 带 `domainId` 与固定 `problemVersion`，构建 claim 带域和封存修订；缺失域或无效判题版本会失败关闭，不回退官方域。结果按 job/generation/worker/lease 围栏写回，不允许客户端重定向资源。
- `internal/scheduler` 管理 worker 循环、租约续期和结果回传。
- `internal/compile` 编译 C/C++，缓存键包含源码、编译命令与真实工具链版本。
- `internal/run` 提供 verdict-neutral `Execution`/`Limits`、安全 artifact I/O，并解析原生 meta。
- `internal/checker` 负责输出比较：内置归一化 diff、testlib checker 的沙箱执行与退出码映射，以及用 testlib 编译打包 checker。
- `internal/builder` 实现题目包构建流水线并打包产物。
- `internal/verdict` 只负责沙箱 meta 到判定的映射。

## 沙箱边界

原生 C++ runner 组合使用：

- Landlock 文件访问白名单。
- 内层 seccomp 网络与高风险 syscall 禁止规则。
- 每个环境独立的低权限 UID 与 cgroup v2；每次执行使用环境下的子 cgroup。
- CPU/wall soft 与 hard timeout、内存、进程数和输出限制。
- workspace 聚合字节与 inode watchdog。
- 可选 cpuset 绑定。

镜像直接启动 `/vertex/vertex-worker`。Worker 使用 `Client.Create` 创建环境，通过 `PutFiles`、`Start/Wait/Cancel` 或 `Run` 执行，再导出产物并 `Close`。cgroup、UID 和目录布局由原生 sandbox 管理；Worker 负责拉取、续租、编排、缓存和回传。环境总内存/进程数预算与每次执行预算分离。同一环境支持连续执行，同一时刻一个活动进程句柄；交互两端使用独立环境并发运行。详见 [Sandbox API](../sandbox/README.md)。

## 通过 Compose 运行

从仓库根目录配置 `.env` 后：

```bash
docker compose up -d --build --wait --wait-timeout 120
docker compose logs -f worker
```

主要配置：

| 环境变量 | 默认值 | 说明 |
|---|---:|---|
| `JUDGE_API_URL` | Compose 内部地址 | Server Judge API base URL |
| `JUDGE_API_TOKEN` | 必填 | Server/Worker service token |
| `JUDGE_WORKER_ID` | hostname/PID | Worker identity；横向扩展时应显式唯一 |
| `JUDGE_WORKERS` | `2` | 单容器并发 worker 循环数 |
| `JUDGE_LONG_POLL_TIMEOUT` | `25s` | claim 服务端等待时间 |
| `JUDGE_HTTP_TIMEOUT` | `40s` | HTTP 请求总超时 |
| `SANDBOX_TIME_OVERSHOOT_MS` | `1000` | soft limit 到 hard kill 的 grace |
| `SANDBOX_WORKSPACE_BYTES` | `67108864` | 节点允许的单次 workspace 聚合逻辑字节 ceiling |
| `SANDBOX_WORKSPACE_INODES` | `4096` | 节点允许的单次 workspace 目录项 ceiling |
| `SANDBOX_CPUSET` | 空 | 可选 Linux cpulist，例如 `0-3,6` |
| `BUILD_WORKER_ENABLED` | `true` | 是否在该节点运行题目包构建循环 |
| `BUILD_PROGRESS_INTERVAL` | `15s` | 构建进度上报（同时续租）间隔 |
| `TESTLIB_PATH` | `/vertex/testlib.h` | testlib 头文件路径；缺失时 Worker 拒绝启动 |

镜像按 commit 固定并校验 sha256 下载 `testlib.h`。编译 checker/validator/generator 时把它作为输入放入环境并用 `-I.` 引用；编译缓存键包含 testlib 摘要。构建与判题按需创建环境，不预留固定 box。

Compose 可以直接 `--scale worker=2`。每个容器的 sandbox/run 使用私有 tmpfs，原生分配器通过能力句柄租约保证并发环境不会复用身份；Worker 自动创建独立 scratch 暂存目录，编译缓存和只读测试数据继续共享。无须配置 box 或 sandbox instance 编号。Worker 退出会取消执行并关闭环境，sandbox 回收进程和临时资源。

完整配置和横向扩展注意事项见[部署文档](../docs/06-deployment.md)。

## 开发与测试

Go 部分：

```bash
go vet ./...
go test ./...
```

原生 runner 已拆分到仓库顶层 [`sandbox/`](../sandbox/README.md)。完整 Worker 镜像会使用 `-Wall -Wextra -Wpedantic -Werror` 构建它。启动 Compose 后运行安全冒烟测试：

```bash
docker compose run --rm --no-deps --entrypoint /vertex/sandbox-smoke-test worker
```

## 通用任务演进

`internal/run` 不包含 AC/WA 等业务语义。每次执行可以独立指定 soft/hard CPU 与 wall budget、较小的 workspace budget、环境和命令；执行结束后可通过受限 `CopyOut` 导出单个普通文件。这套契约可直接复用于 generator、validator、renderer 和对拍节点。

交互题的基础双节点拓扑已由 `RunDuplex` 提供：两个角色使用不同 sandbox，可信 broker 负责双向管道、持续 drain、backpressure、每方向输出上限、idle/deadline 和统一取消，并可保存总字节有界的方向视图与跨方向有序事件，不把多个不可信角色塞进同一个 UID/cgroup。完整 Judge adapter 和通信题 manager 多通道仍按[Sandbox 通用执行内核演进计划](../docs/plans/2026-08-03-sandbox-generalization.md)继续实现。

## 目录结构

```text
cmd/worker/         Worker 进程入口
internal/client/    Server Judge API 客户端
internal/config/    环境配置解析与验证
internal/scheduler/ 租约、编译、执行与回传编排
internal/compile/   工具链与版本化编译缓存
internal/run/       通用 Execution/Limits、artifact I/O、双向 broker 与 native meta
internal/executor/  逐测试点执行与判定策略选择
internal/builder/   题目包构建流水线
internal/checker/   内置 diff 与 testlib checker
internal/verdict/   Judge verdict 映射
```
