# Vertex worker image. Untrusted judge code is supervised by the native C++
# vertex-sandbox runner with Landlock, seccomp, cgroup v2, rlimits and a unique
# run UID. The privileged supervisor prepares nested cgroups automatically;
# submitted programs retain their own Landlock/seccomp and credential boundary.

# ---- Go worker ----
FROM golang:1.25-bookworm AS build

WORKDIR /src/worker
COPY worker/go.mod worker/go.sum ./
RUN go mod download
COPY worker/ ./
RUN CGO_ENABLED=0 go build -o /out/vertex-worker ./cmd/worker

# ---- Native sandbox ----
FROM debian:bookworm-slim AS sandbox-build

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates cmake g++ make libseccomp-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src/sandbox
COPY sandbox/ ./
RUN cmake -S . -B build -DCMAKE_BUILD_TYPE=Release \
    && cmake --build build --parallel \
    && ctest --test-dir build --output-on-failure \
    && cmake --install build --prefix /out

# ---- testlib ----
# Pinned by commit and verified by digest: the build must never silently pick
# up a different testlib than the one problem packages were authored against.
FROM debian:bookworm-slim AS testlib

ARG TESTLIB_COMMIT=1e4e8a24c79c6bad3becbdb5a332ffc352b7d5dd
ARG TESTLIB_SHA256=bb323e3c89285214966076e0d23d5a295c5f6126da7ff198c1276ddb95ecb1a0
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL -o /testlib.h \
      "https://raw.githubusercontent.com/MikeMirzayanov/testlib/${TESTLIB_COMMIT}/testlib.h" \
    && echo "${TESTLIB_SHA256}  /testlib.h" | sha256sum -c -

# ---- runtime ----
FROM debian:bookworm-slim

# Submission toolchains and native sandbox runtime dependency.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential python3 ca-certificates passwd \
    libseccomp2 \
    && rm -rf /var/lib/apt/lists/*

# Runtime state and volumes.
WORKDIR /vertex
ENV PATH=/vertex:/usr/local/bin:/usr/bin:/bin
RUN mkdir -p testdata scratch cache sandbox run \
    && chown 65532:65532 testdata

COPY --from=build /out/vertex-worker /vertex/vertex-worker
# testlib.h is copied into each sandbox workspace before a checker, validator
# or generator is compiled, so no include path escapes the box.
COPY --from=testlib /testlib.h /vertex/testlib.h
COPY --from=sandbox-build /out/bin/vertex-sandbox /vertex/vertex-sandbox
COPY --from=sandbox-build /out/share/licenses/vertex-sandbox/CLI11-LICENSE \
    /vertex/licenses/CLI11-LICENSE
COPY --chmod=755 sandbox/smoke-test.sh /vertex/sandbox-smoke-test

# The worker uses environment and process handles. The native sandbox
# owns cgroups and child credentials; submitted programs have no capabilities.
ENTRYPOINT ["/vertex/vertex-worker"]
