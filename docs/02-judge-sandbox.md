# Vertex OJ — 判题沙箱设计

> 本页描述当前实现的真实安全边界。执行不可信代码始终有内核攻击面；这里采用纵深防御，不承诺“绝对不可逃逸”。

## 1. 威胁模型

不可信对象包括用户源码、编译器处理该源码时产生的行为，以及程序派生的全部进程。受信对象包括宿主管理员、镜像与工具链、Go worker 和 C++ `vertex-sandbox` 监督进程。

沙箱要阻止不可信进程：

- 读写测试数据、缓存、数据库凭据和其他 box；
- 使用网络、创建新 namespace、mount、ptrace 或调用常见高风险内核接口；
- 通过 CPU、内存、输出、workspace 文件/目录项或 fork 耗尽节点；
- 在主进程退出后留下后台进程；
- 用继承 FD、环境变量或软链接诱导受信 worker 越权操作。

内核漏洞、受信 runner/worker 漏洞、恶意编译器或镜像供应链不在单层沙箱可完全消除的范围内。高风险公网部署仍应使用独立 Judge 节点，并限制其网络与凭据权限。

## 2. 隔离结构

```text
Linux 宿主（cgroup v2 + Landlock）
└── Worker 容器（Docker / Podman）
    ├── privileged=true、只读 rootfs，不注入数据库凭据
    ├── Worker 通过 Client.Create 获取环境句柄
    ├── vertex-manager：可信 Worker 和监督进程
    ├── vertex-jobs/instance-environments/environment-N：环境总预算
    │   └── run：一次执行的进程树及统计
    └── vertex-sandbox（受信 C++ watchdog）
        ├── 创建每次运行的 cgroup、输出与 meta 控制文件
        ├── fork 后先把 child 加入 cgroup，再允许其启动
        └── 不可信 child
            ├── 原生分配器租用的独立 UID/GID，无 capabilities
            ├── no_new_privs + Landlock 文件白名单
            ├── 内层 seccomp：断网并拒绝 mount/namespace/ptrace 等接口
            ├── cgroup v2 + rlimit + workspace watchdog
            └── 空环境、关闭继承 FD、独立工作目录
```

`Create` 在原生后端内部完成必要的容器资源初始化，再分配空闲身份、环境 cgroup 和文件目录。环境总内存/进程数预算在多次执行之间持续生效，每次 `Start` 的 CPU、输出等预算作用于本次执行。当前同一环境同时只支持一个活动 Start，独立环境可以并发；交互题使用两个环境。

环境身份由原生分配器通过锁定的 FD 租约分配，经本地 Unix socket 传给适配层；每次 Start 继承该能力，Close 验证后清理并释放。原生进程仍运行时不会复用身份，异常遗留资源在再次分配前回收。Worker 只持有环境和进程句柄，不知道内部路径或编号。题目下载、编译缓存和结果暂存仍由 Worker 管理。

外层 privileged 是为可移植地初始化 cgroup 而接受的权限取舍，不再承诺外层默认 AppArmor/seccomp 的限制。可信 Worker/监督进程被攻破后具有更大的节点访问能力；内层提交进程依然降权、清空 capabilities、设置 no_new_privs 并强制执行 Landlock/seccomp。不能把保留内层防护表述为外层安全边界完全不变。

## 3. 为什么不再使用 mount namespace

`vertex-sandbox` 不调用 `mount(2)`、`unshare(CLONE_NEWNS)` 或 chroot。文件边界改由 [Landlock](https://www.kernel.org/doc/html/latest/userspace-api/landlock.html) 强制：

- workspace 可读写执行，但禁止创建设备节点；
- `/usr`、`/bin`、`/lib*` 仅可读与执行，`/etc` 只读；
- 只开放 `/dev/null`、`zero`、`random`、`urandom` 所需权限；
- `/proc`、`/sys`、`/vertex/testdata`、`/vertex/scratch`、`/vertex/cache`、`/vertex/run` 与其他环境路径不在白名单；
- Landlock 不可用时 runner **失败关闭**，不会降级为裸执行。

该选择使原生执行器无需创建挂载树；AppArmor 不是原生执行器的功能依赖。Landlock 可控制的权限随 ABI 增加；runner 探测实际 ABI，只启用内核支持的权限位。ABI 1 可以运行，但较新 ABI 对 `REFER`、`TRUNCATE` 等操作覆盖更完整。

进程监督思路参考了 [DOMjudge judgehost](https://www.domjudge.org/docs/manual/9.0/install-judgehost.html) 的成熟结构：专用运行用户、外层 watchdog、soft/hard 时间预算、rlimit/cgroup 计量和结束后清理进程树。Vertex 是独立实现，没有复制 GPL 的 `runguard` 源码；主要差异是使用 Landlock，不构建 chroot，也不在容器内 mount。

## 4. 一次运行的顺序

1. Worker 调用 Create，原生后端分配环境及其总预算。通过 PutFiles 放入普通文件，接口验证文件名、大小和执行权限，调用者不修改内部路径。
2. Start 验证环境租约及执行预算；环境已在运行时拒绝第二个 Start。控制目录为 root-only，不暴露给运行 UID。
3. runner 在环境 cgroup 内创建执行子组，设置本次 memory/pids 限额；两层预算同时约束进程。
4. fork 后 child 先阻塞在同步管道；父进程将其写入 `cgroup.procs` 后才放行。
5. child 设置 rlimit、工作目录、空环境、`no_new_privs` 和 Landlock，再切换 UID/GID、清空 capability、加载内层 seccomp，最后 `execve`。
6. 父进程每 5 ms 采样 cgroup CPU/内存、输出大小和 workspace 逻辑字节/目录项，同时执行墙钟 watchdog。workspace 扫描基于 `openat`/`fstatat(AT_SYMLINK_NOFOLLOW)`，不跟随软链接，并容忍文件被并发删除。
7. 主进程退出或触限后，优先用 `cgroup.kill` 清除所有后代；旧内核回退为重复枚举 `cgroup.procs`，防止 fork 竞态。
8. 原子写 meta 并清理本次执行子组。Wait 返回统计、有限日志预览和请求指定的外部输出文件；环境内的文件仍保留，可继续 Start 或 ExportFile。
9. Close 取消剩余执行，等待其后代被清理，删除环境 cgroup 和目录，释放租约。context 取消也触发关闭。需要干净环境时重新 Create，不提供 Reset。

## 5. 资源限制（六限 + 断网）

| 维度 | 实现 | 判定/说明 |
|---|---|---|
| CPU 时间 | cgroup `cpu.stat usage_usec` + `RLIMIT_CPU` 兜底 | soft 超限记录 TLE，hard 超限才杀进程树 |
| 墙钟时间 | watchdog，通常为 CPU 限制 ×2 | soft/hard 语义同 CPU；不包含结束后的后代清理时间 |
| 内存 | `memory.max`、禁 swap、读取 `memory.events` | `oom_kill>0` 优先判 MLE |
| 输出 | stdout/stderr 分别受 `RLIMIT_FSIZE` 与父进程监控 | 达限判 OLE |
| Workspace | 每 5 ms 汇总非目录项逻辑字节和全部目录项 | 默认 64 MiB / 4096 entries，达限杀进程树并暂判 RE |
| 进程/线程 | cgroup `pids.max` + `RLIMIT_NPROC` | 限制 fork bomb；线程同样计数 |
| 网络 | 内层 seccomp 拒绝 socket/connect/bind/listen 等调用 | 不依赖 `NET_ADMIN` 或 network namespace |

`--time-ms` / `--wall-ms` 是 soft limit。每次 `Execution` 可以显式设置 hard CPU/wall budget；未设置时，Go policy 默认生成高出 1000 ms 的 `--time-hard-ms` / `--wall-hard-ms`。soft 超限后程序若在 grace 内自行退出，meta 为 `status:TO`、`time-result:soft`、`killed:0`；监督循环观察到 hard 超限时触发 `cgroup.kill`，通常为 `time-result:hard`、`killed:1`。若主进程恰在最终采样前自行退出，仍记录 hard，但 `killed` 保持 0，避免把自然退出伪报为强杀。`RLIMIT_CPU` 按 hard limit 设置，主进程刚退出时还会做最后一次 cgroup CPU 采样，避免漏掉最后一个轮询周期。

Workspace watchdog 跨 overlayfs、tmpfs 和普通目录工作，但它不是 ext4 project quota：文件创建速度很快时可能产生约一个轮询周期的 overshoot。逻辑字节统计可抓住 sparse-file 扩张；目录项计数也会保守地计算 hard link。控制目录中的 stdout/stderr/meta 不计入 workspace。Worker policy 的 bytes/inodes 是节点 ceiling；单次 `Execution` 可以申请更小预算，省略时使用该 ceiling。

可选的 `SANDBOX_CPUSET` 使用 Linux cpulist 语法（如 `0-3,6`），应用于每个运行 cgroup。未配置时完全不触碰 cpuset；显式配置但父 cgroup 没有委派 `cpuset`、有效 NUMA mems 为空或 CPU 集无效时，worker/runner 失败关闭。

环境从空集合开始，只加入 `PATH`、`LANG`、`HOME`、`TMPDIR` 和受信配置显式传入的变量；`PATH`、`LD_*`、`DYLD_*`、`GLIBC_TUNABLES` 不能覆盖。FD 只保留标准输入输出和一个 `CLOEXEC` 的 setup-error 管道。

## 6. 通用结果与判定分类学

meta 契约为简单 `key:value`，并保持未知字段可忽略。中性字段包括 `termination-reason`、`time-result`、`time-limit`、`time`、`time-wall`、`max-rss`、`cg-mem`、`exitcode`、`exitsig`、`stdout-streamed`、`stdout-bytes`、`stderr-bytes`、`workspace-bytes`、`workspace-inodes`、`killed` 和 `message`。兼容字段 `status`、`cg-oom-killed`、`output-limit`、`workspace-limit` 继续保留给现有 Judge 映射。

`termination-reason` 的稳定值为 `exited`、`signal`、`time-limit`、`memory-limit`、`output-limit`、`workspace-limit`、`cancelled` 和 `setup-error`。`time-result` 为 `none`、`soft` 或 `hard`；`time-limit` 明确记录 `cpu`、`wall` 或 `cpu,wall`。非 Judge 任务直接消费这些字段，不依赖数据库 verdict。

| meta 状态 | 判定 |
|---|---|
| `cg-oom-killed=1` | MLE |
| `status=TO` | TLE |
| `output-limit=1` | OLE |
| `workspace-limit=1` | RE（并保留明确诊断；不新增数据库 verdict） |
| 其他 `status=SG` | RE |
| `status=RE` | RE |
| `status=XX` | SE |
| 正常退出 | diff checker 决定 AC/WA |

最终判定为第一个非 AC 测试点，后续测试点标记 `Skipped`。

## 7. 通用执行契约

Go `internal/run` 以 `Execution` 描述命令、显式环境、单文件 stdin 与完整 `Limits`，返回 verdict-neutral `Meta`。普通题、编译、generator 和 validator 使用同一个原语，上层各自决定业务结果。

可信输入只能通过单层文件名 `CopyIn` 进入 workspace。执行结束后，`CopyOut` 只允许导出单层普通文件，验证打开前后的文件身份、拒绝 symlink/目录、限制复制字节数，并通过临时文件发布到不存在的可信目标。未来目录树和题包使用 manifest 驱动的受限归档，不开放任意递归复制。

交互题和通信题不会把多个角色放进同一 UID/cgroup。`RunDuplex` 已能通过 inherited stdin/stdout FD 启动两个独立 box，由可信 Go broker 双向连接、持续 drain、实施每方向字节上限与 idle timeout，并在 deadline 或节点失败时统一取消。流式 stdout 不写 `control/stdout`，native meta 标记 `stdout-streamed:1`，最终字节数由 broker 回填；stderr 仍写受限控制文件。调用方可设置一个跨双方向共享的 transcript 总字节预算，超出后只标记截断而不增加不受控内存；捕获结果同时提供方向聚合视图和按 broker 观察顺序连续编号的跨方向事件。当前接口是交互题基础拓扑，不等同于完整 Judge adapter；manager 多通道和通信题图拓扑继续按[通用执行内核演进计划](plans/2026-08-03-sandbox-generalization.md)演进。

## 8. 语言配置

| 语言 | 编译/运行 | CPU 倍率 | 内存倍率 | pids |
|---|---|---:|---:|---:|
| C | `gcc -O2 -std=c11` / 直接运行 | 1.0 | 1.0 | 8 |
| C++ | `g++ -O2 -std=c++17` / 直接运行 | 1.0 | 1.0 | 8 |
| Python | 无编译 / `python3` | 3.0 | 2.0 + 64 MiB | 32 |

编译同样在沙箱内，默认限制 10 秒、512 MiB、8 MiB 输出。二进制缓存 fingerprint 包含缓存格式版本、语言、完整编译命令、工具链版本命令及其真实输出、源码哈希；升级编译参数或 gcc/g++ 后不会错误复用旧产物。Python 没有二进制编译缓存。

## 9. 明确的边界与运维要求

- 这不是 VM：目标进程与 runner 共享宿主 Linux 内核，也没有 per-run PID namespace。Docker PID namespace、独立运行 UID、cgroup 与进程清杀共同限制进程影响域。
- Go worker 以 privileged 容器 root 运行并持有内部 Judge API token，但不持有数据库凭据。Landlock/seccomp 在 `execve` 前作用于不可信 child；如果受信 worker/runner 本身被攻破，影响比普通 submission 更大。
- 容器运行时提供可写 cgroup v2 挂载；原生 sandbox 只在本容器层级内创建子树。配置容器总资源预算，避免多个任务的预算相加超过节点容量。
- Worker 不配置 box/instance 编号。原生分配器统一协调并发环境身份；sandbox/run 为容器私有 tmpfs，scratch 为 Worker 暂存，cache 和只读 testdata 继续共享。
- Landlock 主要限制路径访问，某些 metadata 查询不等同于内容读取。需要更强内核隔离时应把 Judge 放到独立节点或微 VM。

## 10. 自动验证

CI 在两个 Worker 中运行环境接口集成测试，验证连续执行、文件导出、身份租约、取消/关闭、跨执行内存预算和双环境交互。独立测试容器继续验证原生引擎的降权、Landlock、seccomp、CPU/wall 超时、输出/内存/进程数/工作目录限制及残留进程清理。

```bash
docker compose up -d --build --wait
docker compose run --rm --no-deps --entrypoint /vertex/sandbox-smoke-test worker
```

后续可选加固包括把双节点 broker 扩展为 manager 多通道图拓扑、把 runner 拆成无服务凭据的最小 `sandboxd`、为 Worker 编写更窄的专用 AppArmor profile，以及提供 Firecracker 后端。专用 AppArmor 是额外纵深防御，不再是解决 mount 的运行前提。
