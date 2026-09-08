# Vertex Sandbox

原生 C++ 后端负责隔离环境的资源分配、进程执行和回收。Worker 通过 `worker/internal/run` 的环境/进程句柄调用它，不分配 UID、box ID 或 cgroup 路径。

## 应用接口

```go
client := run.NewClient("/vertex/sandbox", run.DefaultPolicy())
env, err := client.Create(ctx, run.EnvironmentPolicy{
    MemoryKB: 256 * 1024, Processes: 8,
})
if err != nil { return err }
defer env.Close()

err = env.PutFiles(ctx, map[string]run.InputFile{
    "prog": {Path: compiledFile, Executable: true},
    "input.txt": {Path: inputFile},
})
if err != nil { return err }
result, err := env.Run(ctx, run.Execution{
    Command: []string{"./prog"}, StdinFile: "input.txt",
    StdoutPath: outputFile, // 调用者拥有的目标文件，必须尚不存在
    Limits: limits,
})
```

- `Create / Close` 管理隔离环境，初始化、身份和目录由后端分配。
- `PutFiles` 校验输入文件，`Executable` 明确控制可执行权限。
- `Start / Wait / Cancel` 管理一次执行及其后代，`Run` 是普通文件 I/O 场景的 `Start + Wait`。
- 同一环境支持连续执行，文件在执行之间保留；同一时刻只允许一个活动 `Start`。程序自行派生的进程共享环境预算。
- `ExportFile` 导出普通文件，拒绝路径穿越、符号链接和超大产物。导出文件属于调用者，环境关闭不删除它们。
- `Result` 返回 meta、最多 8 KiB 的 stdout/stderr 预览，以及请求指定的外部输出文件路径，不暴露内部路径。
- 环境 context 取消会触发关闭；`Close` 可重复调用，会取消剩余执行、等待清理、关闭管道并释放租约。

## 交互题

选手和 interactor 使用两个独立环境，各自拥有文件、UID、cgroup 和预算。分别放入文件后调用 `RunDuplex(ctx, left, leftExecution, right, rightExecution, options)`。适配层使用 `Start` 返回的管道连接双方，处理双向输出限额、idle timeout、取消联动和有界 transcript。

自定义编排可以使用 `Start` 并设置 `Execution.Stream=true`，持续消费 `Process.Stdout`、写入/关闭 `Process.Stdin`，最后 `Wait`。流式 stdout 与 `StdoutPath` 互斥。需要干净环境时关闭并重新创建，不提供 Reset。

## 原生实现

容器需要 privileged、cgroup v2 和 Landlock ABI ≥ 1。内层进程仍降 UID、清空 capabilities、设置 no_new_privs，并施加 Landlock、seccomp、rlimit 和 cgroup 限制。

环境级 cgroup 约束总内存和进程数；每次执行在其子 cgroup 中统计 CPU/内存并处理预算。执行结束清理全部后代，环境关闭删除剩余 cgroup 和目录。

原生 `create` 分配空闲身份，通过本地 Unix socket 传递锁定的文件描述符。客户端持有这个能力句柄，`start` 继承它，`close` 验证后回收环境。进程仍持有租约时身份不会被复用；下次分配前回收异常退出遗留的环境。

`prepare/init/run/cleanup` 仍是原生引擎测试和诊断使用的底层原语，不是 Worker 应用接口。低层冒烟脚本必须在独立测试容器执行，避免绕开运行环境身份分配器。

## 容器目录

```text
/vertex/
├── vertex-worker          只读程序
├── vertex-sandbox         只读执行器
├── testlib.h              只读资源
├── licenses/              第三方许可证
├── testdata/              共享测试数据，Worker 只读
├── cache/                 共享编译缓存
├── scratch/               Worker 暂存，每个进程使用独立子目录
├── sandbox/               容器私有临时执行环境
└── run/                   容器私有租约与初始化锁
```

`sandbox/` 为允许执行的 tmpfs，默认总上限 512 MiB；每个环境仍受工作目录字节数和 inode watchdog 限制。`run/` 为 16 MiB tmpfs。容器重建不保留这两类状态。系统工具链使用基础镜像标准路径，内核 cgroup 由后端通过 `/sys/fs/cgroup` 管理。

## 验证

```bash
go -C worker test ./...
docker compose run --rm --no-deps --entrypoint /vertex/sandbox-smoke-test worker

# 在 Linux 构建环境编译，架构须匹配容器
CGO_ENABLED=0 go -C worker test -c -o /tmp/environment.test ./internal/run
docker cp /tmp/environment.test "$(docker compose ps -q worker)":/tmp/environment.test
docker compose exec -e VERTEX_SANDBOX_INTEGRATION=1 worker \
  /tmp/environment.test -test.run '^TestEnvironment' -test.v
```

接口测试覆盖文件保留与导出、身份分配、并发 Start 拒绝、取消与关闭、资源限额、丢失租约的回收及双环境交互。

原生库通过 CMake 构建，保留 `Vertex::Sandbox` 目标与底层 C++ 原语测试。完整边界见 [沙箱设计](../docs/02-judge-sandbox.md)。
