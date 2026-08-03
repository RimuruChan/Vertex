# Vertex Judge

`judge` 是 Vertex 的独立判题 Worker。它通过 Web 内部 API 长轮询领取带租约的任务，完成编译、逐测试点隔离执行和 checker 判定，再回传结构化结果。Judge 不连接 PostgreSQL。

Go module：`github.com/RimuruChan/Vertex/judge`

## 运行要求

执行不受信任代码必须使用满足以下条件的 Linux 环境：

- cgroup v2。
- Landlock ABI 1 或更高版本。
- rootful Docker Engine 与 Docker Compose。

普通 Go 单元测试可以在 Windows/macOS 运行，但原生 `vertex-sandbox` 和完整 Worker 不能在原生 Windows 容器中运行。推荐使用仓库根目录的 Docker Compose 配置，不要直接以宿主 root 身份运行提交代码。

## 工作流程

```text
claim job ──> compile/cache ──> vertex-sandbox ──> checker ──> report result
    │                                  │
    └──────── heartbeat/lease ─────────┘
```

- `internal/client` 实现 claim、heartbeat 和 result HTTP 协议。
- `internal/scheduler` 管理 worker 循环、租约续期和结果回传。
- `internal/compile` 编译 C/C++，缓存键包含源码、编译命令与真实工具链版本。
- `internal/run` 将评测策略转换为 `vertex-sandbox` 参数并解析 meta。
- `internal/checker` 与 `internal/verdict` 负责输出比较和判定分类。

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
docker compose logs -f judge
```

主要配置：

| 环境变量 | 默认值 | 说明 |
|---|---:|---|
| `JUDGE_API_URL` | Compose 内部地址 | Web Judge API base URL |
| `JUDGE_API_TOKEN` | 必填 | Web/Judge service token |
| `JUDGE_WORKER_ID` | hostname/PID | Worker identity；横向扩展时应显式唯一 |
| `JUDGE_WORKERS` | `2` | 单容器并发 worker 循环数 |
| `JUDGE_LONG_POLL_TIMEOUT` | `25s` | claim 服务端等待时间 |
| `JUDGE_HTTP_TIMEOUT` | `40s` | HTTP 请求总超时 |
| `SANDBOX_TIME_OVERSHOOT_MS` | `1000` | soft limit 到 hard kill 的 grace |
| `SANDBOX_WORKSPACE_BYTES` | `67108864` | 单次 workspace 聚合逻辑字节上限 |
| `SANDBOX_WORKSPACE_INODES` | `4096` | 单次 workspace 目录项上限 |
| `SANDBOX_CPUSET` | 空 | 可选 Linux cpulist，例如 `0-3,6` |

完整配置和横向扩展注意事项见[部署文档](../docs/06-deployment.md)。

## 开发与测试

Go 部分：

```bash
go vet ./...
go test ./...
```

Linux 上单独编译原生 runner：

```bash
cmake -S sandbox -B sandbox/build -DCMAKE_BUILD_TYPE=Release
cmake --build sandbox/build --parallel
```

完整镜像会使用 `-Wall -Wextra -Wpedantic -Werror` 构建 runner。启动 Compose 后运行安全冒烟测试：

```bash
docker compose exec -T judge /usr/local/libexec/vertex-sandbox-smoke-test
```

## 目录结构

```text
cmd/worker/         Worker 进程入口
internal/client/    Web Judge API 客户端
internal/config/    环境配置解析与验证
internal/scheduler/ 租约、编译、执行与回传编排
internal/compile/   工具链与版本化编译缓存
internal/run/       vertex-sandbox Go 封装
internal/executor/  逐测试点执行
internal/checker/   输出 checker
internal/verdict/   verdict 与 sandbox meta 映射
sandbox/            C++ runner、CMake 与 smoke test
```
