# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:22-alpine AS webbuild

WORKDIR /src
COPY package.json ./
RUN npm install --no-audit --no-fund
COPY . .
# Source-contract web tests also read internal Go code, VERSION and test fixtures.
RUN npm run build:css

# For isolated alpha builds, make browser tests a build gate.
ARG CALDEN_RUN_TESTS=0
RUN if [ "$CALDEN_RUN_TESTS" = "1" ]; then npm run test:web; fi

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build

ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# Alpha testing fails closed if Go unit/contract tests do not pass.
ARG CALDEN_RUN_TESTS=0
RUN if [ "$CALDEN_RUN_TESTS" = "1" ]; then go test ./...; fi

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
COPY --from=webbuild /src/web /app/web
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
