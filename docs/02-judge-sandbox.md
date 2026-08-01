# Vertex OJ — 判题沙箱设计

> 本页是 Vertex 安全模型的核心。执行不可信代码本就是产品,沙箱逃逸=宿主机沦陷。

## 1. 隔离模型

```
宿主(部署机,仅受信管理员)
└── Docker 容器: judge worker   ←── 第一层:进程级容器隔离
    ├── 非特权:无 --privileged,无额外 capabilities
    ├── 只读 rootfs(read_only: true)
    ├── 唯一挂载:testdata(ro)、scratch、cache、isolatebox、cgroupfs
    └── 仅 CAP_SYS_ADMIN(isolate 创建 mount namespace 所需)
        └── isolate(--cg, cgroup v2)  ←── 第二层:每测试点的命名空间+资源沙箱
            ├── mount / PID / IPC / network 命名空间(默认断网,loopback only)
            ├── 私有文件系统视图(/bin /usr /lib + /box rw)
            ├── 降权:box 内程序以低权限 box UID 运行
            └── cgroup v2:memory.max、pids.max、swap 禁用
```

**关键安全论证**:
- 不用 `--privileged`:Judge0 CVE-2024-28185 根因即是特权容器内逃逸 → 直接获得宿主机 root。
- 本设计即使 isolate 被攻破,攻击者获得的是**容器内 root**,而非宿主机 root(Docker 的 namespaces + seccomp 兜底)。
- 不用 per-submission 容器:创建/销毁开销占 77% 延迟;长驻 worker + isolate 每测试点 sub-10 ms。

## 2. 资源限制(五限 + 断网)

| 维度 | 实现 | 说明 |
|---|---|---|
| CPU 时间 | `--time`(isolate) | 按语言倍率换算(见 §4) |
| 墙钟时间 | `--wall-time` = CPU×2 | 防 `sleep()` 躲避时间限制 |
| 内存 | cgroup v2 `memory.max` + OOM 事件 | 峰值= `cg-mem` 与 `max-rss` 较大者 |
| 输出大小 | `--fsize` + 有界 stdout | 截断判 **OLE**,不判 WA |
| 进程数 | `--processes`(pids.max) | 防 fork 炸弹;注意线程也算进程 |
| 网络 | isolate 默认不 `--share-net` | 目标代码断网 |

## 3. 判定分类学(信号 → 判定)

isolate meta 关键字段:`status`(RE/SG/TO/XX)、`time`、`time-wall`、`max-rss`、`exitcode`、`exitsig`、`cg-oom-killed`、`killed`。

映射规则(`judge/internal/verdict/verdict.go`):

| meta 状态 | 判定 | 说明 |
|---|---|---|
| `cg-oom-killed=1` | **MLE** | 内存打爆 ≠ TLE! |
| `status=TO` 或墙钟超限 | **TLE** | |
| `status=SG`(被信号杀) | **RE** | 附信号号(SIGSEGV=11 等) |
| `status=RE`(非零退出) | **RE** | 附退出码 |
| `status=XX` | **SE** | isolate 内部错误 |
| 正常退出 | diff checker 决定 | AC/WA |

最终判定 = 第一个非 AC 测试点;后续测试点标 `Skipped`。

## 4. 语言配置与倍率

`judge/internal/compile/compile.go` 的 `Supported` 注册表:

| 语言 | 编译 | 运行 | CPU 倍率 | 内存倍率 | 进程数 |
|---|---|---|---|---|---|
| C | gcc -O2 -static | 直接 | 1.0 | 1.0 | 8 |
| C++ | g++ -O2 -std=c++17 -static | 直接 | 1.0 | 1.0 | 8 |
| Python | 无(解释执行) | python3 | 3.0 | 2.0(+64MB) | 32 |

- **编译也在沙箱内**,带自身时间(10s)/内存(512MB)/输出(8MB)上限,防 `#include </dev/random>` 类攻击。
- 编译产物按 `(语言, sha256(源码))` 缓存,rejudge/重复提交零编译。

## 5. 安全清单(实施核对)

- [ ] Worker 容器 unprivileged、只读 rootfs、无宿主 PID/网络挂载
- [ ] CPU ≠ 墙钟,双限都要
- [ ] 内存用 cgroup v2,禁用 swap
- [ ] 进程数上限(Java/Python 需放宽,线程算进程)
- [ ] 输出上限 + `--fsize`,截断按 OLE
- [ ] 清空环境变量与继承 FD(仅 0,1,2,剥 LD_PRELOAD)
- [ ] 每测试点后 `isolate --cleanup` + 重新 `--init`,防僵尸泄漏
- [ ] 编译在沙箱内
- [ ] 信号 vs 退出码映射正确(OOM→MLE,不要标成 TLE)
- [ ] 噪声宿主 CPU 限流膨胀会计(v1:除以速率因子)

## 6. 扩展预留

- **SPJ**:`checker` 字段已建模(diff/spj/interactive),统一 checker 接口在 `judge/internal/checker`。
- **交互题**:`problems.judge_type` 已建,交互器进程 + 管道 + 死锁处理留 v2。
- **子任务**:`problem_testdata.config_json` 预留 batched/dependency 声明。
- **更强的隔离**:接口保持 `copyIn/copyOut/limits`,可后插 Firecracker 微 VM 后端。
