# Worker 任务上下文与验证边界

## 协议不变量

- Worker 只使用 Server 内部 HTTP API，不接触 PostgreSQL。`worker/cmd/worker` 的依赖图不包含 pgx/sqlx 或 Server database 包；Compose 不向 Worker 提供 `DATABASE_URL`，测试数据卷只读。
- 判题 job 包含域 ID、内部题目/提交 UUID、固定 `problemVersion`、generation、worker ID 和 lease token。Server 从 job 固定的发布版本读取限制和测试数据，而不是领取时追随最新工作副本。Worker 拒绝缺失域或非正发布版本的成功领取响应，不回退官方域。
- 构建在入队时封存材料、工作 revision、data revision 与域。领取时核对封存域与父题目实际域，以及题目/修订匹配；取消或过期 attempt 不能继续上报。后续编辑不会替换同一任务的输入。
- 心跳、结果与构建上传依据 job/generation/worker/lease 组合验证，结果不接受另一个 domain 作为重定向目标。内部服务可以领取各域任务，但外部用户不能借此跨域访问。
- 本地执行与测试数据目录使用全局唯一内部资源 ID、内容哈希和实例隔离，不使用可能在不同域重复的公开数字编号作为共享目录标识。sandbox、scratch、cgroup 的实例边界及 capability 白名单保持不变。

主要源码：`server/internal/judge/store.go`、`server/internal/authoring/store_build.go`、`worker/internal/client`、`worker/internal/scheduler`、`worker/internal/builder`。

## 已执行验证

- Windows 原生 Worker `go vet ./...` / `go test ./... -count=1`；领取响应上下文、路径限制、重试与租约失败等测试实际执行。
- 真实 PostgreSQL 的构建封存域不匹配回归：领取被拒绝，事务回滚，任务仍为 queued。
- `TestDomainAPIIntegration` 不启动服务进程、不开放端口，使用生产路由、真实认证/session、PostgreSQL 和临时文件目录，执行认证生命周期、站点治理、域资源流程及任务协议。
- 域流程覆盖 group 继承/移除、私有发布版本、可复用列表、公告草稿隔离、域治理权限、跨域复制与源删除后的来源记录。协议流程检查判题 claim 的域/版本、错误 worker 心跳拒绝、构建封存源码及取消后旧 lease 拒绝。
- 协议 fixture 不运行程序；判题终态明确为 System Error，并记录“未执行程序”。这不是实际判题成功的证据。

## CI 与未执行边界

CI 的正常 E2E 列表包含 `TestEndToEndDomainWorkflow`。停止 Worker 后先运行 `TestEndToEndDomainProtocol`，再运行旧 Judge fencing；后者故意留下 queued 重试，所以不能先执行它再让域协议测试领取队列。

当前本机没有可用 Docker CLI，启动隔离原生 API 服务进程的命令被执行策略拒绝；该命令未创建服务或测试库。已采用上述不启动进程/端口的 API 集成，但未宣称完成容器构建、安全冒烟、Worker 实际执行、网络恢复、完整 Compose/CI/E2E 或移动端验收。

完整外部验证仍按 [部署文档](06-deployment.md) 与 `.github/workflows/e2e.yml` 在可用的 Linux Docker 环境执行。本任务没有调用 WSL，也没有修改 Docker 默认 seccomp/AppArmor、只读 rootfs、cgroup 委派范围或现有 capability 白名单。
