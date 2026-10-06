FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/calden ./cmd/calden

FROM alpine:3.22
RUN addgroup -S calden && adduser -S -G calden calden
WORKDIR /app
COPY --from=build /out/calden /app/calden
COPY web /app/web
RUN mkdir -p /data && chown -R calden:calden /data /app
USER calden
ENV CALDEN_PORT=8787 CALDEN_DATA_DIR=/data CALDEN_WEB_DIR=/app/web
EXPOSE 8787
ENTRYPOINT ["/app/calden"]
