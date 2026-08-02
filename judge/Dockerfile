# Judge worker 镜像
#
# 安全模型(sandbox checklist):
#   - 容器本身不使用 --privileged:docker-compose 仅添加 isolate 所需的
#     CAP_SYS_ADMIN,保留 Docker 默认 seccomp,rootfs 只读。
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

# 编译工具链(C++/C/Python)与 isolate 运行时依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential python3 ca-certificates passwd \
    libcap2 libseccomp2 libsystemd0 \
    && rm -rf /var/lib/apt/lists/*

# 安装 isolate(ioi/isolate):需要 libcap + libseccomp + libsystemd 开发头
ARG ISOLATE_VERSION=v2.6
RUN apt-get update && apt-get install -y --no-install-recommends \
    git libcap-dev libseccomp-dev libsystemd-dev asciidoc \
    && git clone --depth 1 --branch "${ISOLATE_VERSION}" https://github.com/ioi/isolate.git /tmp/isolate \
    && cd /tmp/isolate && make && make install \
    && rm -rf /tmp/isolate \
    && apt-get purge -y git libcap-dev libseccomp-dev libsystemd-dev asciidoc \
    && apt-get autoremove -y && rm -rf /var/lib/apt/lists/* \
    && useradd -r -M isolate \
    && usermod --add-subuids 60000-62555 isolate \
    && usermod --add-subgids 60000-62555 isolate \
    && sed -i 's|^cg_root = .*|cg_root = /sys/fs/cgroup/isolate|' /usr/local/etc/isolate

# 工作目录与卷
RUN mkdir -p /testdata /scratch /cache /var/local/lib/isolate
VOLUME ["/testdata", "/scratch", "/cache", "/var/local/lib/isolate"]

COPY --from=build /out/judge-worker /usr/local/bin/judge-worker
COPY --chmod=755 judge/docker-entrypoint.sh /usr/local/bin/judge-entrypoint

# 容器内 root 运行(供 isolate 使用);对不可信代码的隔离由 isolate 完成
ENTRYPOINT ["judge-entrypoint"]
CMD ["judge-worker"]
