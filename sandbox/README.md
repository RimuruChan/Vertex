# Vertex Sandbox

`sandbox` 是 Vertex 的原生 C++ 隔离执行器源码。它负责启动一个不受信任进程树、施加 Linux 安全与资源策略，并把退出状态和资源统计写入 verdict-neutral 结构化 meta。Judge、数据生成、validator、renderer 或对拍等业务语义由可信 Go 编排层解释。

它是独立的构建单元，但不是独立的网络服务。当前由 [`worker/Dockerfile`](../worker/Dockerfile) 在多阶段构建中编译，再复制到 Worker 镜像内由 Go Worker 调用。

## 安全与资源边界

- Landlock 文件访问白名单。
- 内层 seccomp 禁止网络及高风险 syscall。
- 每次运行使用独立低权限 UID 和 cgroup v2 子 cgroup。
- CPU 与 wall soft/hard timeout，hard limit 到达后使用 `cgroup.kill`。
- 内存、进程数、输出大小和 RLIMIT 兜底。
- workspace 聚合逻辑字节和 inode watchdog。
- 可选 cgroup cpuset 绑定。

Runner 不使用 mount namespace、chroot、`SYS_ADMIN` 或 `apparmor=unconfined`。完整威胁模型、meta 语义和已知限制见[Judge 沙箱设计](../docs/02-judge-sandbox.md)。

Meta 会区分 `termination-reason`、`time-result`（none/soft/hard）、CPU/wall 命中来源，并记录 stdout/stderr 字节。兼容的 `status` 和 limit flag 仍由 Judge verdict 映射使用。Runner 始终保持单进程树原语；可信 Go broker 可通过 `--stdin-fd` / `--stdout-fd` 连接多个独立 sandbox，而不是让多个角色共享 UID、workspace 或 cgroup。流式 FD 是能力传递接口，只允许可信编排层使用；方向字节上限和 idle timeout 由 broker 强制。设计见[通用执行内核演进计划](../docs/plans/2026-08-03-sandbox-generalization.md)。

## 环境要求

- Linux 5.13+ 或提供 Landlock ABI 1+ 的发行版内核。
- cgroup v2，并向调用方委派可写子树。
- CMake 3.18+、支持 C++20 的 GCC/Clang。
- libseccomp 开发包。

原生 Windows/macOS 不支持完整运行环境。Windows 开发者应使用 WSL2 Linux Docker。

## 构建

在仓库根目录执行：

```bash
cmake -S sandbox -B sandbox/build -DCMAKE_BUILD_TYPE=Release
cmake --build sandbox/build --parallel
```

CMake 默认以 `-O2 -Wall -Wextra -Wpedantic -Werror` 构建 `vertex-sandbox`。

## 验证

推荐通过完整 Worker 容器执行冒烟测试，因为 Compose 会配置所需的 UID、目录和 cgroup 子树：

```bash
docker compose up -d --build --wait --wait-timeout 120
docker compose exec -T worker /usr/local/libexec/vertex-sandbox-smoke-test
```

冒烟测试覆盖文件和网络隔离、soft/hard timeout、内存/输出限制、workspace bytes/inodes、单节点 inherited stream FD、双独立 sandbox 管道通信、信号与退出状态映射。

## 源码

```text
main.cpp       参数解析、隔离策略、监督循环与 meta 输出
CMakeLists.txt C++20 构建定义和严格编译告警
smoke-test.sh  容器内安全与资源限制冒烟测试
```
