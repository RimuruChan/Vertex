# 后端业务上下文与持久化边界

后端按业务上下文组织，每个上下文内明确领域、应用、基础设施和 HTTP 适配职责。`identity` 管理账号与认证；`tenancy` 管理产品中的域、成员、角色和群组。内部使用 `tenancy` 区分业务上的“域”和 DDD 的 domain 层，HTTP 路由和 JSON 中仍使用原有的 domain 名称。

## 包结构

```text
internal/<context>/
  domain/                       # 模型、业务规则、repository 契约
  application/                  # 用例编排；依赖契约
  infrastructure/
    postgres/
      *_repository.go           # PostgreSQL 实现、事务与错误转换
      queries/                  # 固定、参数化的 SQL
      internal/dbgen/           # sqlc 生成；仅本实现可导入
    token/                      # 有需要时提供独立适配器，如 identity 的 JWT
    filesystem/                 # 有需要时提供文件适配器，如题目测试数据
  transport/http/
    *.go                        # 请求绑定、状态码、路由
    dto/                        # HTTP 请求与响应模型
```

领域层不依赖应用层、SQL 驱动、sqlc、具体 repository 或 HTTP 框架。应用层不依赖基础设施及 HTTP 适配。基础设施实现契约；进程入口负责实例化并注入依赖。

## Repository 的职责

接口表达实际业务操作，不按表名批量生成 `BaseRepository` 或为每条 SQL 增加透传接口。聚合写入和读模型查询可以分别定义契约。`sqlc` 生成的参数、结果和数据库行类型限制在对应 PostgreSQL 包内，转换为领域模型后才能返回应用层。

查询名应明确说明效果，例如 `CancelActiveJudgeJobs`、`ResetSubmissionForRejudge`、`EnqueueJudgeGeneration`。不采用“调用函数名＋表名＋序号”的命名。参数和计算列使用有意义的名称及明确类型，避免生成 `Column1` 或 `interface{}` 后在调用方猜测转换。

题目模块使用 `Queries` 和 `Repository` 区分读模型与写入。公开题目和工作副本分别有固定的列表及 count 查询，共用各自明确的筛选条件。ZIP 解包、内容寻址和目录删除由 `filesystem.TestdataStorage` 实现，进程入口通过 `ArtifactStorage` 契约注入；数据库 repo 在授权锁内协调文件操作、revision 和候选数据，不负责解包细节。

## 事务和权限

事务边界按业务原子性确定。身份注册的账号与初始域成员记录同时提交；刷新凭据的旧 hash 只能被成功消费一次。

域治理通过 `Repository.WithinGovernance` 执行。基础设施先按账号、域的顺序取得锁并重新读取权限，再把只在回调期间有效的 `Governance` 契约交给应用层。应用层通过该契约读取和修改成员、角色、群组并记录审计，不接触数据库连接或 SQL transaction。回调失败时全部回滚。

资源写入仍与域治理共用域锁：治理取得排他锁，资源写入取得共享锁。不能把事务内的权限重查移到事务外，也不能把评测结果、任务代次、逐点结果和计分更新拆成独立提交。

## SQL 和生成

Schema 继续维护在 `server/migrations`。各上下文查询文件直接作为 sqlc 输入，不引入生成 SQL 的自定义模板或运行时字符串拼接层。

从仓库根目录运行：

```powershell
go -C server generate ./internal/database
go -C server test ./...
```

sqlc 固定为 1.31.1；生成工具使用它所需的 Go toolchain，不改变服务的 Go 版本声明。生成代码提交到仓库，不手工修改。生成结果应可重复，CI 需要检查重新生成后的差异。

## 测试组织

`service.go` 的单测放在 `service_test.go`，`user_repository.go` 的数据库验证放在 `user_repository_test.go`。没有必要为简单映射、构造函数或接口实现声明单独增加测试文件；Ginkgo 入口也放在对应测试文件中，不另外散落 suite 文件。

单测保留业务规则、错误传播和关键边界。数据库原子性必须在真实 PostgreSQL 上验证，不能用对 fake repository 的并发调用证明真实数据库安全。权限撤销、跨域关联、lease/generation、事务回滚和榜单一致性等验证不能因包重组而丢失。
