# Vertex Worker

`worker` 是 Vertex 的后台任务执行进程。它通过 Server 内部 API 长轮询领取带租约的任务，在原生沙箱中执行，再回传结构化结果；Worker 不连接 PostgreSQL。当前实现两类任务：

- **判题**：编译、逐测试点隔离执行、checker 判定。
- **题目包构建**：编译 testlib checker/validator/generator 与各解，生成并校验输入，用标程产出答案，构建期对拍，最后打包上传测试数据。

Go module：`github.com/RimuruChan/Vertex/worker`

目录使用通用的 Worker 命名；两条任务循环共用同一套沙箱、编译缓存与租约围栏模型，各自占用独立的 sandbox box。

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
- 每次运行独立的低权限 UID 与 cgroup v2 子 cgroup。
- CPU/wall soft 与 hard timeout、内存、进程数和输出限制。
- workspace 聚合字节与 inode watchdog。
- 可选 cpuset 绑定。

容器保持 Docker 默认 seccomp/AppArmor、只读 rootfs 和最小 capability 白名单，不使用 `--privileged`、mount namespace、chroot 或 `SYS_ADMIN`。完整威胁模型与限制见[沙箱设计文档](../docs/02-judge-sandbox.md)。

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
| `SANDBOX_INSTANCE_ID` | 容器 hostname | 实例目录/cgroup 命名空间；仅允许 1–64 位字母、数字、`_`、`.`、`-`，首位必须是字母或数字 |
| `SANDBOX_BOX_ID` | `0` | 实例命名空间内的起始 box id；不同实例可安全复用同一范围 |
| `BUILD_WORKER_ENABLED` | `true` | 是否在该节点运行题目包构建循环 |
| `BUILD_PROGRESS_INTERVAL` | `15s` | 构建进度上报（同时续租）间隔 |
| `TESTLIB_PATH` | `/usr/local/share/vertex/testlib.h` | testlib 头文件路径；缺失时 Worker 拒绝启动 |

镜像按 commit 固定并校验 sha256 下载 `testlib.h`。编译 checker/validator/generator 时把它复制进沙箱 workspace 并用 `-I.` 引用，没有任何 include 路径指向沙箱之外；编译缓存键包含 testlib 摘要。启用构建的节点会多占用一个 sandbox box（`SANDBOX_BOX_ID + JUDGE_WORKERS`）。

Compose 横向扩展时保持 `SANDBOX_INSTANCE_ID` 为空，entrypoint 会使用每个容器唯一的 hostname，把 `SANDBOX_BASE`、`SCRATCH_ROOT` 和 `VERTEX_CGROUP_ROOT` 重定向到独立实例子树；`CACHE_ROOT` 与只读 `TESTDATA_ROOT` 仍在 replica 间共享。因此不同实例可以都从 box 0 开始，例如 `docker compose up -d --scale worker=2`。Go Worker 还会在共享 sandbox volume 上为 instance id 持有进程生命周期文件锁，重复 ID 会在领取任务前失败。显式设置 instance id 只适用于分别配置、能保证 ID 唯一的实例；同一 scaled service 不能共享一个显式值。

entrypoint 以 `exec` 启动 Go Worker，SIGTERM/SIGINT 仍直接进入已有的优雅退出与 box cleanup。实例根目录会在重启后保留并复用；native runner 在每次 box 初始化时回收该 box 的残留进程与 cgroup。缺少 cpu/memory/pids controller、cpuset 未委派、ID 非法或实例目录不可写时，Worker 会在领取任务前失败关闭。

完整配置和横向扩展注意事项见[部署文档](../docs/06-deployment.md)。

## 开发与测试

Go 部分：

```bash
go vet ./...
go test ./...
```

原生 runner 已拆分到仓库顶层 [`sandbox/`](../sandbox/README.md)。完整 Worker 镜像会使用 `-Wall -Wextra -Wpedantic -Werror` 构建它。启动 Compose 后运行安全冒烟测试：

```bash
docker compose exec -T worker /usr/local/libexec/vertex-sandbox-smoke-test
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
