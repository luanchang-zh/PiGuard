# 编译阶段使用纯 Go SQLite 驱动，不需要开启 CGO。
FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

# 运行阶段只保留二进制和默认配置。数据目录由 Compose 卷挂载到 /app/data。
FROM alpine:3.21

WORKDIR /app
COPY --from=build /out/server /app/server
COPY config.yaml /app/config.yaml
EXPOSE 8080
ENTRYPOINT ["/app/server"]
CMD ["-config", "/app/config.yaml"]
