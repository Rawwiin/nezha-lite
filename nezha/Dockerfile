# Nezha 精简版 Dashboard Dockerfile
# 多阶段构建：前端下载 → Go 编译 → 运行时镜像
# 使用 modernc.org/sqlite（纯 Go），无需 CGO 和交叉编译工具链

# ===== 阶段 1：下载前端资源 =====
FROM alpine:latest AS frontend

# 安装下载工具
RUN apk add --no-cache curl unzip

WORKDIR /build

# 下载前端资源（版本与 frontend-templates.yaml 保持一致）
# 管理端前端（admin-frontend）
RUN curl -L -o admin.zip https://github.com/nezhahq/admin-frontend/releases/download/v2.2.2/dist.zip && \
    unzip -q admin.zip && mv dist admin-dist && rm admin.zip

# 用户端前端（nezha-dash-v2）
RUN curl -L -o user.zip https://github.com/hamster1963/nezha-dash-v2/releases/download/v2.1.2/dist.zip && \
    unzip -q user.zip && mv dist user-dist && rm user.zip

# Nazhua 前端（可选）
RUN curl -L -o nazhua.zip https://github.com/hi2shark/nazhua/releases/download/v0.9.1/dist.zip && \
    unzip -q nazhua.zip && mv dist nazhua-dist && rm nazhua.zip

# Nezha-ASCII 前端（可选）
RUN curl -L -o ascii.zip https://github.com/hamster1963/nezha-ascii/releases/download/v1.1.0/dist.zip && \
    unzip -q ascii.zip && mv dist nezha-ascii-dist && rm ascii.zip

# ===== 阶段 2：编译 Dashboard 二进制 =====
FROM golang:1.26-alpine AS builder

# 安装编译依赖（git 用于 go mod 下载）
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# 先复制 go.mod/go.sum 利用 Docker 缓存层
COPY nezha/go.mod nezha/go.sum ./
RUN go mod download

# 复制源码
COPY nezha/ ./

# 将前端资源复制到 embed 目录
COPY --from=frontend /build/admin-dist ./cmd/dashboard/admin-dist
COPY --from=frontend /build/user-dist ./cmd/dashboard/user-dist
COPY --from=frontend /build/nazhua-dist ./cmd/dashboard/nazhua-dist
COPY --from=frontend /build/nezha-ascii-dist ./cmd/dashboard/nezha-ascii-dist

# 编译（使用 modernc 纯 Go SQLite，CGO_ENABLED=0，静态链接）
# -tags modernc: 使用 modernc.org/sqlite 替代 mattn/go-sqlite3
# -ldflags "-s -w": 去除调试信息，减小体积
# -trimpath: 去除编译路径信息
# VERSION 构建参数由 docker build --build-arg VERSION=xxx 注入，默认 dev
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -tags modernc \
    -ldflags "-s -w -X github.com/nezhahq/nezha/service/singleton.Version=${VERSION}" \
    -trimpath \
    -o nezha-dashboard \
    ./cmd/dashboard

# ===== 阶段 3：运行时镜像 =====
FROM alpine:latest AS runtime

# 安装运行时依赖：ca-certificates（HTTPS）、tzdata（时区）
RUN apk add --no-cache ca-certificates tzdata && \
    update-ca-certificates

# 创建数据目录
WORKDIR /dashboard
RUN mkdir -p /dashboard/data

# 从构建阶段复制二进制
COPY --from=builder /build/nezha-dashboard /dashboard/nezha-dashboard

# 暴露端口（HTTP + gRPC 复用）
EXPOSE 8008

# 数据卷：持久化 SQLite 数据库和配置
VOLUME ["/dashboard/data"]

# 时区设置
ENV TZ=Asia/Shanghai

# 入口点：直接运行二进制
ENTRYPOINT ["/dashboard/nezha-dashboard"]
