# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build

ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    VERSION="$(cat VERSION)" && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/calden ./cmd/calden

FROM alpine:3.22

RUN apk add --no-cache ca-certificates postgresql-client tzdata \
    && addgroup -S calden \
    && adduser -S -G calden calden \
    && mkdir -p /data /app \
    && chown -R calden:calden /data /app

WORKDIR /app

COPY --from=build /out/calden /app/calden
COPY web /app/web
COPY docker-entrypoint.sh /app/docker-entrypoint.sh

USER root
RUN chmod 0755 /app/docker-entrypoint.sh /app/calden \
    && chown -R calden:calden /app /data

USER calden

ENV CALDEN_PORT=8787 \
    CALDEN_DATA_DIR=/data \
    CALDEN_WEB_DIR=/app/web

EXPOSE 8787

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=5 \
    CMD wget -q -O - http://127.0.0.1:8787/api/health | grep -q '"ok":true' || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
