# Judge worker 镜像
#
# 安全模型(sandbox checklist):
#   - 容器本身是 UNPRIVILEGED:docker-compose 中不设 privileged、不添加 capabilities、
#     配 seccomp profile 收紧、rootfs 只读、--network none。
#     沙箱逃逸 = 获得容器内 root,而非宿主机 root。
#   - 容器内以 root 运行:isolate --init/--run 需要容器内 root 来设置 cgroups 与命名空间。
#   - 不可信用户代码由 isolate 降权到 box UID 运行,网络隔离,资源受限。

# ---- 构建阶段:编译 Go worker 与编译 isolate ----
FROM golang:1.25-bookworm AS build

WORKDIR /src/judge
COPY judge/go.mod judge/go.sum ./
RUN go mod download
COPY judge/ ./
RUN CGO_ENABLED=0 go build -o /out/judge-worker ./cmd/worker

# ---- runtime ----
FROM debian:bookworm-slim

# 编译工具链(C++/C/Python)
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc g++ python3 build-essential \
    && rm -rf /var/lib/apt/lists/*

# 安装 isolate(ioi/isolate):需要 libcap + libseccomp + libsystemd 开发头
# 构建 man page 需要 asciidoc;这里跳过文档仅装二进制(经 make -C doc 规避成本高,
# 直接装 asciidoc 最简单)
RUN apt-get update && apt-get install -y --no-install-recommends \
    git make libcap-dev libseccomp-dev libsystemd-dev asciidoc \
    && git clone --depth 1 https://github.com/ioi/isolate.git /tmp/isolate \
    && cd /tmp/isolate && make && make install \
    && rm -rf /tmp/isolate && apt-get purge -y git make libcap-dev libseccomp-dev libsystemd-dev asciidoc \
    && apt-get autoremove -y && rm -rf /var/lib/apt/lists/*

# 工作目录与卷
RUN mkdir -p /testdata /scratch /cache /var/local/lib/isolate
VOLUME ["/testdata", "/scratch", "/cache", "/var/local/lib/isolate"]

COPY --from=build /out/judge-worker /usr/local/bin/judge-worker

# 容器内 root 运行(供 isolate 使用);对不可信代码的隔离由 isolate 完成
ENTRYPOINT ["judge-worker"]
