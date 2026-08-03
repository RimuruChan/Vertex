# Judge worker image. Untrusted code is supervised by the native C++
# vertex-sandbox runner with Landlock, seccomp, cgroup v2, rlimits and a unique
# run UID. The runner does not call mount(2), so it does not require relaxing
# Docker's default AppArmor profile.

# ---- Go worker ----
FROM golang:1.25-bookworm AS build

WORKDIR /src/judge
COPY judge/go.mod judge/go.sum ./
RUN go mod download
COPY judge/ ./
RUN CGO_ENABLED=0 go build -o /out/judge-worker ./cmd/worker

# ---- Native sandbox ----
FROM debian:bookworm-slim AS sandbox-build

RUN apt-get update && apt-get install -y --no-install-recommends \
    cmake g++ make libseccomp-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src/sandbox
COPY judge/sandbox/ ./
RUN cmake -S . -B build -DCMAKE_BUILD_TYPE=Release \
    && cmake --build build --parallel \
    && cmake --install build --prefix /out

# ---- runtime ----
FROM debian:bookworm-slim

# Submission toolchains and native sandbox runtime dependency.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential python3 ca-certificates passwd \
    libseccomp2 \
    && rm -rf /var/lib/apt/lists/*

# Runtime state and volumes.
RUN mkdir -p /testdata /scratch /cache /var/local/lib/vertex-sandbox /vertex-cgroup \
    && chown 65532:65532 /testdata
VOLUME ["/testdata", "/scratch", "/cache", "/var/local/lib/vertex-sandbox"]

COPY --from=build /out/judge-worker /usr/local/bin/judge-worker
COPY --from=sandbox-build /out/bin/vertex-sandbox /usr/local/bin/vertex-sandbox
COPY --chmod=755 judge/docker-entrypoint.sh /usr/local/bin/judge-entrypoint
COPY --chmod=755 judge/sandbox/smoke-test.sh /usr/local/libexec/vertex-sandbox-smoke-test

# The trusted worker starts as container root to manage delegated cgroups and
# switch children to their dedicated run UIDs. The submission process itself
# has no capabilities.
ENTRYPOINT ["judge-entrypoint"]
CMD ["judge-worker"]
