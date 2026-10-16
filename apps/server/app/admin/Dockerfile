# 多阶段构建：admin 服务
# 使用方法：在 v2/TwinServer 根目录执行 docker build -f app/admin/Dockerfile -t framework-admin .

# ---------- 阶段1：构建 ----------
FROM golang:1.26 AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GOPROXY=https://goproxy.cn,direct

WORKDIR /build

# 先拷依赖文件，利用 docker 层缓存
COPY go.mod go.sum ./
RUN go mod download

# 拷源码
COPY . .

# 构建 admin 二进制
RUN go build -o /out/admin ./app/admin/cmd

# ---------- 阶段2：运行 ----------
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

WORKDIR /app

# 拷贝二进制
COPY --from=builder /out/admin /app/admin

# 拷贝配置文件（公共 conf/ + 私有 app/admin/config/）
# conf/ 目录含 proto 生成的 .go 文件，docker 运行时只需要 yaml，用通配过滤
COPY conf/*.yaml /app/conf/
COPY app/admin/config/ /app/app/admin/config/

# 拷贝 OPA 策略和种子数据（按需）
COPY app/admin/assets/ /app/app/admin/assets/

# 日志目录
RUN mkdir -p /app/logs
VOLUME ["/app/logs"]

# REST 7788
EXPOSE 7788

# 配置路径：公共配置 ../../conf（相对 cmd）→ /app/conf，私有 ../config → /app/app/admin/config
CMD ["/app/admin", "-conf-common", "/app/conf", "-conf", "/app/app/admin/config"]
