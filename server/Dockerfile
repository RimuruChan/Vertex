# Vertex server image (Go + Gin).
FROM golang:1.25-bookworm AS build

WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -o /out/vertex-server ./cmd/server
RUN mkdir -p /out/testdata

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /vertex
ENV PATH=/vertex:/usr/local/bin:/usr/bin:/bin
COPY --from=build /out/vertex-server /vertex/vertex-server
COPY server/migrations /vertex/migrations
COPY --from=build --chown=65532:65532 /out/testdata /vertex/testdata

ENV MIGRATIONS_DIR=/vertex/migrations
ENV TESTDATA_ROOT=/vertex/testdata
HEALTHCHECK --interval=5s --timeout=3s --retries=10 CMD ["/vertex/vertex-server", "healthcheck"]
ENTRYPOINT ["/vertex/vertex-server"]
