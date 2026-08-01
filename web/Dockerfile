# Web API 镜像(Go + Gin)
FROM golang:1.25-bookworm AS build

WORKDIR /src/web
COPY web/go.mod web/go.sum ./
RUN go mod download
COPY web/ ./
RUN CGO_ENABLED=0 go build -o /out/vertex-web ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
COPY --from=build /out/vertex-web /usr/local/bin/vertex-web
COPY web/migrations /web/migrations

ENV MIGRATIONS_DIR=/web/migrations
ENTRYPOINT ["vertex-web"]
