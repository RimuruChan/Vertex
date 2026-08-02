# Vertex OJ — 判题沙箱设计

> 本页描述当前实现的真实安全边界。执行不可信代码始终有内核攻击面；这里采用纵深防御，不承诺“绝对不可逃逸”。

## 1. 威胁模型

不可信对象包括用户源码、编译器处理该源码时产生的行为，以及程序派生的全部进程。受信对象包括宿主管理员、镜像与工具链、Go worker 和 C++ `vertex-sandbox` 监督进程。

沙箱要阻止不可信进程：

- 读写测试数据、缓存、数据库凭据和其他 box；
- 使用网络、创建新 namespace、mount、ptrace 或调用常见高风险内核接口；
- 通过 CPU、内存、输出或 fork 耗尽节点；
- 在主进程退出后留下后台进程；
- 用继承 FD、环境变量或软链接诱导受信 worker 越权操作。

内核漏洞、受信 runner/worker 漏洞、恶意编译器或镜像供应链不在单层沙箱可完全消除的范围内。高风险公网部署仍应使用独立 Judge 节点，并限制其网络与凭据权限。

## 2. 隔离结构

```text
Linux 宿主
└── Docker judge 容器
    ├── privileged=false、只读 rootfs、默认 Docker seccomp/AppArmor
    ├── capability: CHOWN/DAC_OVERRIDE/FOWNER/KILL/SETGID/SETUID
    ├── /sys/fs/cgroup: Docker 默认只读
    ├── /vertex-cgroup: 当前 Compose 项目专属子树，可写
    └── vertex-sandbox（受信 C++ watchdog）
        ├── 创建每次运行的 cgroup、输出与 meta 控制文件
        ├── fork 后先把 child 加入 cgroup，再允许其启动
        └── 不可信 child
            ├── 独立数值 UID/GID（60000 + box id），无 capabilities
            ├── no_new_privs + Landlock 文件白名单
            ├── 内层 seccomp：断网并拒绝 mount/namespace/ptrace 等接口
            ├── cgroup v2 + rlimit 五限
            └── 空环境、关闭继承 FD、独立工作目录
```

`cgroup: host` 只让 runner 看见可绑定的宿主 cgroup 层级；Compose 只将 `/sys/fs/cgroup/vertex-<project>` 绑定为可写，宿主其余 cgroupfs 仍是只读。它不是 `--privileged`，但会暴露 cgroup 层级元数据，因此应视为有意识的部署权衡。

## 3. 为什么不再使用 mount namespace

`vertex-sandbox` 不调用 `mount(2)`、`unshare(CLONE_NEWNS)` 或 chroot。文件边界改由 [Landlock](https://www.kernel.org/doc/html/latest/userspace-api/landlock.html) 强制：

- workspace 可读写执行，但禁止创建设备节点；
- `/usr`、`/bin`、`/lib*` 仅可读与执行，`/etc` 只读；
- 只开放 `/dev/null`、`zero`、`random`、`urandom` 所需权限；
- `/proc`、`/sys`、`/testdata`、`/scratch`、`/cache` 与 worker 其他路径不在白名单；
- Landlock 不可用时 runner **失败关闭**，不会降级为裸执行。

这直接消除了 isolate 设置 private mount tree 时与 Docker 默认 AppArmor 的冲突，也不再需要 `SYS_ADMIN` 或 `apparmor=unconfined`。Landlock 可控制的权限随 ABI 增加；runner 探测实际 ABI，只启用内核支持的权限位。ABI 1 可以运行，但较新 ABI 对 `REFER`、`TRUNCATE` 等操作覆盖更完整。

进程监督思路参考了 [DOMjudge judgehost](https://www.domjudge.org/docs/manual/8.0/install-judgehost.html) 的成熟结构：专用运行用户、外层 watchdog、rlimit/cgroup 计量和结束后清理进程树。Vertex 是独立实现，没有复制 GPL 的 `runguard` 源码；主要差异是使用 Landlock，不构建 chroot，也不在容器内 mount。

## 4. 一次运行的顺序

1. Go worker 将可信输入复制到单层文件名的 workspace；旧目标先删除，再以 `O_EXCL` 创建，避免跟随用户软链接。
2. runner 创建 root-only `control/`，stdout、stderr 和 meta 不暴露给运行 UID。
3. runner 创建 cgroup，写入 `memory.max`、`memory.swap.max=0`、`memory.oom.group=1` 与 `pids.max`。
4. fork 后 child 先阻塞在同步管道；父进程将其写入 `cgroup.procs` 后才放行。
5. child 设置 rlimit、工作目录、空环境、`no_new_privs` 和 Landlock，再切换 UID/GID、清空 capability、加载内层 seccomp，最后 `execve`。
6. 父进程每 5 ms 采样 cgroup CPU/内存与输出大小，同时执行墙钟 watchdog。
7. 主进程退出或触限后，优先用 `cgroup.kill` 清除所有后代；旧内核回退为重复枚举 `cgroup.procs`，防止 fork 竞态。
8. 原子写 meta；Go worker读取结果并在每个测试点后 cleanup + init workspace。

## 5. 资源限制（五限 + 断网）

| 维度 | 实现 | 判定/说明 |
|---|---|---|
| CPU 时间 | cgroup `cpu.stat usage_usec` + `RLIMIT_CPU` 兜底 | 按语言倍率换算，超限 TLE |
| 墙钟时间 | watchdog，通常为 CPU 限制 ×2 | 防 sleep/阻塞绕过，超限 TLE |
| 内存 | `memory.max`、禁 swap、读取 `memory.events` | `oom_kill>0` 优先判 MLE |
| 输出 | stdout/stderr 分别受 `RLIMIT_FSIZE` 与父进程监控 | 达限判 OLE |
| 进程/线程 | cgroup `pids.max` + `RLIMIT_NPROC` | 限制 fork bomb；线程同样计数 |
| 网络 | 内层 seccomp 拒绝 socket/connect/bind/listen 等调用 | 不依赖 `NET_ADMIN` 或 network namespace |

环境从空集合开始，只加入 `PATH`、`LANG`、`HOME`、`TMPDIR` 和受信配置显式传入的变量；`PATH`、`LD_*`、`DYLD_*`、`GLIBC_TUNABLES` 不能覆盖。FD 只保留标准输入输出和一个 `CLOEXEC` 的 setup-error 管道。

## 6. 判定分类学

meta 契约为简单 `key:value`：`status`、`time`、`time-wall`、`max-rss`、`cg-mem`、`exitcode`、`exitsig`、`cg-oom-killed`、`output-limit`、`killed`、`message`。

| meta 状态 | 判定 |
|---|---|
| `cg-oom-killed=1` | MLE |
| `status=TO` | TLE |
| `output-limit=1` | OLE |
| 其他 `status=SG` | RE |
| `status=RE` | RE |
| `status=XX` | SE |
| 正常退出 | diff checker 决定 AC/WA |

最终判定为第一个非 AC 测试点，后续测试点标记 `Skipped`。

## 7. 语言配置

| 语言 | 编译/运行 | CPU 倍率 | 内存倍率 | pids |
|---|---|---:|---:|---:|
| C | `gcc -O2 -std=c11` / 直接运行 | 1.0 | 1.0 | 8 |
| C++ | `g++ -O2 -std=c++17` / 直接运行 | 1.0 | 1.0 | 8 |
| Python | 无编译 / `python3` | 3.0 | 2.0 + 64 MiB | 32 |

编译同样在沙箱内，默认限制 10 秒、512 MiB、8 MiB 输出。编译产物按 `(语言, sha256(源码))` 缓存。

## 8. 明确的边界与运维要求

- 这不是 VM：目标进程与 runner 共享宿主 Linux 内核，也没有 per-run PID namespace。Docker PID namespace、独立运行 UID、cgroup 与进程清杀共同限制进程影响域。
- Go worker 以容器 root 运行并持有数据库凭据和六项 capability。Landlock/seccomp 在 `execve` 前作用于不可信 child；如果受信 worker/runner 本身被攻破，影响比普通 submission 更大。
- 项目 cgroup 子树可写是准确资源计量所必需；不要把整个 `/sys/fs/cgroup` 设为 rw。不同 Compose 项目自动使用不同子树。
- 同一 judge 容器内每个并发 worker 必须使用不同 box id。默认推荐增加 `JUDGE_WORKERS`，不要直接 `docker compose --scale judge`；多容器部署必须另行分配不重叠的 box id/子树。
- Landlock 主要限制路径访问，某些 metadata 查询不等同于内容读取。需要更强内核隔离时应把 Judge 放到独立节点或微 VM。

## 9. 自动验证

CI 会在支持 AppArmor 的 Ubuntu runner 上断言 `docker-default`、非 privileged/只读 rootfs、capability 白名单与 cgroup 绑定。容器内冒烟测试验证 Landlock 拒绝越界文件、网络拒绝、环境清空、真实 C++ 编译运行、CPU TLE、输出 OLE、内存 MLE，以及运行后 cgroup 清理。WSL 内核可能未启用 AppArmor，此时本地 `AppArmorProfile` 为空，但 Compose 仍不能配置 `apparmor=unconfined`。

```bash
docker compose up -d --build --wait
docker compose exec -T judge /usr/local/libexec/vertex-sandbox-smoke-test
```

后续可选加固包括把 runner 拆成无数据库凭据的最小 `sandboxd`、为 Judge 编写更窄的专用 AppArmor profile，以及提供 Firecracker 后端。专用 AppArmor 是额外纵深防御，不再是解决 mount 的运行前提。
