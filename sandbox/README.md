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

原生 Runner 不创建 mount namespace 或 chroot。外层 Worker 使用 privileged 自动准备容器内部 cgroup；提交程序清空 capabilities 并保留 Landlock/seccomp，不因监督进程权限扩大而获得相同权限。完整威胁模型、meta 语义和已知限制见[Judge 沙箱设计](../docs/02-judge-sandbox.md)。

Meta 会区分 `termination-reason`、`time-result`（none/soft/hard）、CPU/wall 命中来源，并记录 stdout/stderr 字节。兼容的 `status` 和 limit flag 仍由 Judge verdict 映射使用。Runner 始终保持单进程树原语；可信 Go broker 可通过 `--stdin-fd` / `--stdout-fd` 连接多个独立 sandbox，而不是让多个角色共享 UID、workspace 或 cgroup。流式 FD 是能力传递接口，只允许可信编排层使用；方向字节上限和 idle timeout 由 broker 强制。设计见[通用执行内核演进计划](../docs/plans/2026-08-03-sandbox-generalization.md)。

## 环境要求

- Linux 5.13+ 或提供 Landlock ABI 1+ 的发行版内核。
- cgroup v2；Worker 容器启动时自动创建可写子树。独立调用原生 runner 时需设置 `VERTEX_CGROUP_ROOT` 指向已启用 cpu/memory/pids 的子树。
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
CLI 参数解析使用 CMake `FetchContent` 获取固定版本的 CLI11；归档版本和
SHA-256 均写在 `CMakeLists.txt` 中，不依赖系统中的 CLI11 包。首次配置需要
访问 GitHub；同一 build 目录会复用已下载源码，Docker 构建可复用对应层。
CLI11 许可证会随安装产物一并安装。

## C++ API

可信 C++ 编排器可以链接 `Vertex::Sandbox`，直接使用公开能力而不经过 CLI：

```cmake
add_subdirectory(sandbox)
target_link_libraries(my-orchestrator PRIVATE Vertex::Sandbox)
```

```cpp
#include <vertex/sandbox.hpp>

vertex::sandbox::SandboxConfig config;
config.box_id = 7;
vertex::sandbox::Sandbox box(config);
box.initialize();

vertex::sandbox::RunOptions run;
run.time_ms = 1000;
run.wall_ms = 2000;
run.memory_kb = 262144;
run.output_bytes = 32 * 1024 * 1024;
run.command = {"./solution"};
const auto artifacts = box.run(run);
```

`Sandbox::probe` 返回内核能力，`CancellationToken` 可由可信调用方传入
`Sandbox::run`。CLI 只是上述 API 的适配器；CLI11 不会成为核心库的传递依赖。

## 验证

推荐通过完整 Worker 容器执行冒烟测试，因为 Compose 会配置所需的 UID、目录和 cgroup 子树：

```bash
docker compose up -d --build --wait --wait-timeout 120
docker compose exec -T worker /usr/local/libexec/vertex-sandbox-smoke-test
```

冒烟测试覆盖文件和网络隔离、soft/hard timeout、内存/输出限制、workspace bytes/inodes、单节点 inherited stream FD、双独立 sandbox 管道通信、信号与退出状态映射。

## 源码

```text
include/vertex/sandbox.hpp     公开 C++ API
include/vertex/internal/*.hpp 各实现模块的私有头文件
src/main.cpp                  最小进程入口
src/sandbox.cpp               公开 Sandbox 能力实现
src/internal/*.cpp            与 internal 头文件对应的私有实现
tests/api_test.cpp            公开 API consumer test
CMakeLists.txt                核心库、CLI 和 FetchContent 定义
smoke-test.sh                 容器内安全与资源限制冒烟测试
```
