# Nezha Lite 构建指南

> 面向开发和运维人员的编译、打包、Docker 镜像构建指南。普通用户请参考 [README.md](README.md) 中的安装和部署章节。

## 编译

> **关于 `-tags modernc`**：Dashboard 使用 SQLite 作为默认数据库。原版依赖 `mattn/go-sqlite3`（需要 CGO），交叉编译时必须开启 `CGO_ENABLED=1` 并配置对应平台的 C 编译器链，操作复杂。精简版通过 `modernc.org/sqlite`（纯 Go 实现）替代，只需在编译时加上 `-tags modernc` 即可，`CGO_ENABLED=0` 静态链接，交叉编译零障碍。

### 环境要求

- Go 1.26+
- Agent 不需要 CGO，Dashboard 使用 modernc 也不需要 CGO

### Windows 本地编译

> ```powershell
> $env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
> ```

```powershell
# Agent
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
cd agent
go build -ldflags "-s -w" -o nezha-agent.exe ./cmd/agent

# Dashboard
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
cd nezha
go build -tags modernc -ldflags "-s -w" -o nezha-dashboard.exe ./cmd/dashboard
```

### Linux 交叉编译

```powershell
# Linux amd64 Agent
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags "-s -w" -o nezha-agent ./cmd/agent

# Linux amd64 Dashboard
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -tags modernc -ldflags "-s -w" -o nezha-dashboard ./cmd/dashboard
```

### 支持的目标平台

| 平台 | GOOS | GOARCH |
|------|------|--------|
| Linux amd64 | linux | amd64 |
| Linux arm64 | linux | arm64 |
| Windows amd64 | windows | amd64 |
| macOS amd64 | darwin | amd64 |
| macOS arm64 | darwin | arm64 |

```powershell
# Linux arm64 Agent
$env:GOOS="linux"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"
go build -ldflags "-s -w" -o nezha-agent ./cmd/agent

# Linux arm64 Dashboard
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -tags modernc -ldflags "-s -w" -o nezha-dashboard ./cmd/dashboard
```

### 压缩二进制（可选）

```bash
# 使用 UPX 压缩，可减小 50%-70% 体积
upx --best nezha-agent
upx --best nezha-dashboard
```

### 前端资源嵌入（仅 Dashboard）

Dashboard 编译时通过 `go:embed` 将前端资源嵌入二进制。如需更新前端：

```bash
# 下载前端资源到 embed 目录
cd cmd/dashboard/admin-dist
# 从 https://github.com/nezhahq/admin-frontend/releases 下载 dist.zip
unzip dist.zip -d admin-dist && mv admin-dist/dist/* admin-dist/ && rmdir admin-dist/dist

# 用户端前端
cd ../user-dist
# 从 https://github.com/hamster1963/nezha-dash-v2/releases 下载 dist.zip
unzip dist.zip -d user-dist && mv user-dist/dist/* user-dist/ && rmdir user-dist/dist
```

> 不下载前端资源也能编译，但 Dashboard 启动后前端页面会 404。

## 手动打包 Release

### 目录结构

```
nezha-release/
├── dashboard/
│   ├── nezha-dashboard          # Linux amd64
│   ├── nezha-dashboard.exe      # Windows amd64
│   └── data/                    # 空目录，运行时存放配置和数据库
└── agent/
    ├── nezha-agent              # Linux amd64
    └── nezha-agent.exe          # Windows amd64
```

### PowerShell 打包脚本

```powershell
$releaseDir = "./release"
New-Item -ItemType Directory -Force -Path "$releaseDir/dashboard/data"
New-Item -ItemType Directory -Force -Path "$releaseDir/agent"

# 编译 Dashboard
cd nezha
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -tags modernc -ldflags "-s -w" -o ../$releaseDir/dashboard/nezha-dashboard ./cmd/dashboard
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -tags modernc -ldflags "-s -w" -o ../$releaseDir/dashboard/nezha-dashboard.exe ./cmd/dashboard
cd ..

# 编译 Agent
cd agent
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags "-s -w" -o ../$releaseDir/agent/nezha-agent ./cmd/agent
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags "-s -w" -o ../$releaseDir/agent/nezha-agent.exe ./cmd/agent
cd ..

# 打包成 zip
Compress-Archive -Path "$releaseDir/dashboard" -DestinationPath "$releaseDir/dashboard.zip"
Compress-Archive -Path "$releaseDir/agent" -DestinationPath "$releaseDir/agent.zip"
```

## Docker 镜像构建

### 方式一：Docker 多阶段构建（推荐，需要服务器有 Docker）

在 Linux 服务器上直接用 Dockerfile 多阶段构建，自动下载前端 + 编译 + 打包：

```bash
# 在服务器上
git clone https://github.com/Rawwiin/nezha-lite.git
cd nezha-lite

# 多阶段构建（自动下载前端 + 编译 + 打包）
docker build -f nezha/Dockerfile -t ghcr.io/rawwiin/nezha-lite:latest .
```

**Dockerfile 构建流程：**

| 阶段 | 基础镜像 | 作用 |
|------|----------|------|
| `frontend` | `alpine` | 下载 4 个前端资源（admin-frontend、nezha-dash-v2、nazhua、nezha-ascii） |
| `builder` | `golang:1.26-alpine` | `CGO_ENABLED=0 -tags modernc` 编译纯 Go 二进制 |
| `runtime` | `alpine` | 仅包含二进制 + ca-certificates + tzdata，体积约 30MB |

> **与原版 Dockerfile 的区别**：原版依赖 goreleaser 预编译的二进制（需要 CGO 交叉编译工具链），精简版改为 Docker 内直接编译，使用 `modernc.org/sqlite` 纯 Go 实现，无需 CGO。

### 方式二：Windows 交叉编译 + Linux 服务器打包（无本地 Docker）

本地 Windows 没有 Docker 时，在 Windows 交叉编译 Linux 二进制，传到 Linux 服务器打包成镜像。

#### 一键脚本（推荐）

```powershell
# 仅编译 amd64 二进制（手动上传）
.\build-docker.ps1 -BuildOnly

# 编译并自动上传到服务器构建
.\build-docker.ps1 -Server user@1.2.3.4 -ImageName ghcr.io/rawwiin/nezha-lite:latest

# 编译 arm64 架构
.\build-docker.ps1 -Arch arm64 -Server user@1.2.3.4
```

#### 手动操作

**步骤 1：Windows 交叉编译 Linux 二进制**

```powershell
# amd64
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
cd C:\workspace\nezha\nezha
go build -tags modernc -ldflags "-s -w" -trimpath -o dashboard-linux-amd64 ./cmd/dashboard

# arm64
$env:GOARCH="arm64"
go build -tags modernc -ldflags "-s -w" -trimpath -o dashboard-linux-arm64 ./cmd/dashboard

# 恢复
$env:GOOS="windows"; $env:GOARCH="amd64"
```

**步骤 2：上传二进制和 Dockerfile.prebuilt 到服务器**

```bash
scp C:\workspace\nezha\nezha\dashboard-linux-amd64 user@server:/tmp/nezha-dashboard
scp C:\workspace\nezha\nezha\Dockerfile.prebuilt user@server:/tmp/Dockerfile
```

**步骤 3：在服务器上构建镜像**

```bash
# SSH 到服务器
ssh user@server

# 构建镜像
cd /tmp
docker build -f Dockerfile --build-arg BINARY=nezha-dashboard -t ghcr.io/rawwiin/nezha-lite:latest .

# 验证镜像
docker images | grep nezha-lite
```

**步骤 4：推送到 GHCR（GitHub Container Registry）**

```bash
# 登录 GHCR（用 GitHub Personal Access Token，需勾选 write:packages 权限）
echo "ghp_xxxxx" | docker login ghcr.io -u 用户名 --password-stdin

# 标记并推送
docker tag ghcr.io/rawwiin/nezha-lite:latest ghcr.io/rawwiin/nezha-lite:v1.0.0
docker push ghcr.io/rawwiin/nezha-lite:latest
docker push ghcr.io/rawwiin/nezha-lite:v1.0.0
```

**步骤 5：在服务器上运行**

```bash
# 准备配置
mkdir -p /opt/nezha/data
cat > /opt/nezha/data/config.yaml <<'EOF'
site_name: "Nezha Lite"
language: "zh_CN"
listen_port: 8008
tls: false
debug: false
EOF

# 运行容器
docker run -d \
  --name nezha-dashboard \
  --restart always \
  -p 8008:8008 \
  -v /opt/nezha/data:/dashboard/data \
  -e TZ=Asia/Shanghai \
  ghcr.io/rawwiin/nezha-lite:latest
```

### 多架构镜像构建（amd64 + arm64）

在 Linux 服务器上使用 buildx 构建多架构镜像：

```bash
# 创建 buildx builder（仅需一次）
docker buildx create --name nezha-builder --use

# 方式A：用预编译二进制分别构建
# 先在 Windows 编译 amd64 和 arm64 二进制并上传到服务器
docker buildx build --platform linux/amd64 \
  -f Dockerfile.prebuilt --build-arg BINARY=dashboard-linux-amd64 \
  -t ghcr.io/rawwiin/nezha-lite:amd64 --push .

docker buildx build --platform linux/arm64 \
  -f Dockerfile.prebuilt --build-arg BINARY=dashboard-linux-arm64 \
  -t ghcr.io/rawwiin/nezha-lite:arm64 --push .

# 方式B：用多阶段 Dockerfile 一次构建（需要服务器有 Go 环境）
docker buildx build --platform linux/amd64,linux/arm64 \
  -f nezha/Dockerfile \
  -t ghcr.io/rawwiin/nezha-lite:latest --push .
```

## GoReleaser 自动发布

推送 `v*` 格式的 tag 时，GitHub Actions 自动编译 Agent（20 平台）和 Dashboard（4 平台）并上传到 GitHub Release。

### 触发方式

```bash
# 打 tag 并推送
git tag v1.0.0
git push origin v1.0.0

# GitHub Actions 自动执行：
# 1. 编译 Agent（20 平台：linux/darwin/windows × amd64/arm64/arm/386）
# 2. 编译 Dashboard（4 平台：linux/darwin/windows × amd64/arm64）
# 3. 生成 checksums.txt
# 4. 上传到 GitHub Release
```

### Agent 支持的编译目标（20 平台）

| GOOS | GOARCH | 说明 |
|------|------|------|
| linux | amd64 | 64 位 Linux（最常用） |
| linux | arm64 | ARM64 Linux（树莓派 4、AWS Graviton） |
| linux | arm | ARMv7 Linux（树莓派 3） |
| linux | 386 | 32 位 Linux |
| darwin | amd64 | Intel Mac |
| darwin | arm64 | Apple Silicon Mac |
| windows | amd64 | 64 位 Windows |
| windows | arm64 | ARM64 Windows |
| windows | 386 | 32 位 Windows |

### Dashboard 支持的编译目标（4 平台）

| GOOS | GOARCH | 说明 |
|------|------|------|
| linux | amd64 | 64 位 Linux |
| linux | arm64 | ARM64 Linux |
| darwin | amd64 | Intel Mac |
| darwin | arm64 | Apple Silicon Mac |

### 构建标签

| 参数 | 标签 | 说明 |
|---|---|---|
| `tags` | `go_json` | 使用 fast JSON 编解码 |
| `tags` | `modernc` | 使用纯 Go SQLite 驱动（无需 CGO） |

### 版本号注入

| 来源 | 说明 |
|------|------|
| git tag | `v1.0.0` → 版本号 `1.0.0` |
| ldflags | `-X .../singleton.Version=1.0.0` 注入到二进制 |
