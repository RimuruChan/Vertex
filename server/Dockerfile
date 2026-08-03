# Vertex server image (Go + Gin).
FROM golang:1.25-bookworm AS build

WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -o /out/vertex-server ./cmd/server
RUN mkdir -p /out/testdata

FROM gcr.io/distroless/base-debian12:nonroot
COPY --from=build /out/vertex-server /usr/local/bin/vertex-server
COPY server/migrations /server/migrations
COPY --from=build --chown=65532:65532 /out/testdata /testdata

ENV MIGRATIONS_DIR=/server/migrations
VOLUME ["/testdata"]
HEALTHCHECK --interval=5s --timeout=3s --retries=10 CMD ["vertex-server", "healthcheck"]
ENTRYPOINT ["vertex-server"]
