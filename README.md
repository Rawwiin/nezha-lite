# Nezha 精简版

> 基于 [Nezha Monitoring](https://github.com/nezhahq/nezha) 的精简分支，**保留核心主机监控与服务延迟探测能力**，移除交互式命令、自更新、远程配置推送及 Dashboard 侧的高风险功能（MCP、Terminal、OAuth2、告警通知等）。适合追求最小攻击面、仅需「看状态 + 测延迟」的场景。

## 快速开始

### 环境要求

- Go 1.23+
- 精简版已内置管理前端（admin-frontend）和用户前端（nezha-dash-v2），编译时自动嵌入

## 编译

> **关于 `-tags modernc`**：Dashboard 使用 SQLite 作为默认数据库。原版依赖 `mattn/go-sqlite3`（需要 CGO），交叉编译时必须开启 `CGO_ENABLED=1` 并配置对应平台的 C 编译器链，操作复杂。精简版通过 `modernc.org/sqlite`（纯 Go 实现）替代，只需在编译 Dashboard 时添加 `-tags modernc`，即可在 `CGO_ENABLED=0` 环境下完成任意平台的交叉编译。Agent 不涉及 SQLite，无需此标签。

### Windows 本地编译

> **注意**：如果之前执行过交叉编译，请先重置环境变量，否则编译出的二进制无法在 Windows 上运行：
> ```powershell
> $env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
> ```

```powershell
# Agent
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
cd agent
go build -ldflags "-s -w" -o nezha-agent.exe ./cmd/agent

# Dashboard（-tags modernc 使用纯 Go SQLite，无需 CGO）
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
cd nezha
go build -tags modernc -ldflags "-s -w" -o nezha-dashboard.exe ./cmd/dashboard
```

### Linux 交叉编译

```powershell
# Linux amd64 Agent
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags "-s -w" -o nezha-agent ./cmd/agent

# Linux amd64 Dashboard（-tags modernc 使用纯 Go SQLite，无需 CGO）
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -tags modernc -ldflags "-s -w" -o nezha-dashboard ./cmd/dashboard
```

### 更多平台交叉编译

| 目标平台 | GOOS | GOARCH |
|----------|------|--------|
| Linux amd64 | linux | amd64 |
| Linux arm64 | linux | arm64 |
| Windows amd64 | windows | amd64 |
| macOS amd64 | darwin | amd64 |
| macOS arm64 | darwin | arm64 |

```powershell
# 示例：编译 Linux arm64 Agent
$env:GOOS="linux"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"
go build -ldflags "-s -w" -o nezha-agent ./cmd/agent

# Dashboard 交叉编译示例（必须加 -tags modernc）
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -tags modernc -ldflags "-s -w" -o nezha-dashboard ./cmd/dashboard
```

### 体积压缩（可选）

```bash
# 使用 UPX 压缩二进制（约压缩到 30-40%）
upx --best nezha-agent
upx --best nezha-dashboard
```

### 更新前端（可选）

精简版编译时通过 `//go:embed *-dist` 自动嵌入 `cmd/dashboard/admin-dist/` 和 `cmd/dashboard/user-dist/` 目录中的前端构建产物。如需更新前端版本：

```bash
# 更新管理端前端（admin-frontend）
cd cmd/dashboard/admin-dist
# 从 https://github.com/nezhahq/admin-frontend/releases 下载 dist.zip
unzip dist.zip -d admin-dist && mv admin-dist/dist/* admin-dist/ && rmdir admin-dist/dist

# 更新用户端前端（nezha-dash-v2）
cd ../user-dist
# 从 https://github.com/hamster1963/nezha-dash-v2/releases 下载 dist.zip
unzip dist.zip -d user-dist && mv user-dist/dist/* user-dist/ && rmdir user-dist/dist
```

> 也可以使用 `frontend-templates.yaml` 中列出的其他前端（如 nazhua、nezha-ascii），将构建产物放入对应目录即可。

## 打包

### 目录结构

```
nezha-release/
├── dashboard/
│   ├── nezha-dashboard         # Linux 二进制
│   ├── nezha-dashboard.exe     # Windows 二进制
│   └── data/
│       └── config.yaml         # 服务端配置文件
│
└── agent/
    ├── nezha-agent              # Linux 二进制
    ├── nezha-agent.exe          # Windows 二进制
    └── config.yml               # 客户端配置文件
```

### 一键打包脚本（PowerShell）

```powershell
$releaseDir = "./release"
New-Item -ItemType Directory -Force -Path "$releaseDir/dashboard/data"
New-Item -ItemType Directory -Force -Path "$releaseDir/agent"

# 编译 Dashboard（-tags modernc 使用纯 Go SQLite，支持无 CGO 交叉编译）
cd nezha
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -tags modernc -ldflags "-s -w" -o ../$releaseDir/dashboard/nezha-dashboard ./cmd/dashboard
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -tags modernc -ldflags "-s -w" -o ../$releaseDir/dashboard/nezha-dashboard.exe ./cmd/dashboard
cd ..

# 编译 Agent
cd agent
$env:GOOS="linux"; $env:GOARCH="amd64"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags "-s -w" -o ../$releaseDir/agent/nezha-agent ./cmd/agent
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -ldflags "-s -w" -o ../$releaseDir/agent/nezha-agent.exe ./cmd/agent
cd ..

# 复制配置文件
Copy-Item "./config/dashboard-config.yaml" "$releaseDir/dashboard/data/config.yaml" -ErrorAction SilentlyContinue
Copy-Item "./config/agent-config.yml" "$releaseDir/agent/config.yml" -ErrorAction SilentlyContinue

Write-Host "打包完成: $releaseDir"
```

## 部署

### Dashboard 服务端部署

#### 1. 准备目录和配置文件

```bash
mkdir -p /opt/nezha/data
cp nezha-dashboard /opt/nezha/
cp config.yaml /opt/nezha/data/
chmod +x /opt/nezha/nezha-dashboard
```

#### 2. 最小配置文件（`data/config.yaml`）

```yaml
site_name: "Nezha"
language: "zh_CN"
location: "Asia/Shanghai"
listen_port: 8008
jwt_secret_key: ""
debug: false
enable_mcp: false

tsdb:
  data_path: "./data/tsdb"
  retention_days: 30

```

> 首次启动后，`jwt_secret_key` 会自动生成。数据库固定为同目录 `data/sqlite.db`，自动创建。

> **与原版配置兼容性**：原版的 `config.yaml` 可直接复用。精简版支持原版的字段（如 `tls`, `listen_port`, `jwt_timeout`, `jwt_secret_key_last_rotated_version`, `web_real_ip_header`, `user_template`, `memory`, `tsdb` 等），无需修改。注意：精简版 HTTP 和 gRPC 共用 `listen_port`，原版中若使用 `grpc_port` 需要在 Agent 中将 `server` 改为 `<host>:<listen_port>`。

#### 3. 启动方式

```bash
./nezha-dashboard    # 直接运行，自动读取同目录下 data/config.yaml 和 data/sqlite.db
```

> 精简版已移除 `-c` / `-db` / `-v` 命令行参数。配置文件和数据库固定放在可执行文件同目录的 `data/` 下。

#### 4. systemd 服务（推荐）

创建 `/etc/systemd/system/nezha-dashboard.service`：

```ini
[Unit]
Description=Nezha Dashboard
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/nezha
ExecStart=/opt/nezha/nezha-dashboard
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now nezha-dashboard
sudo systemctl status nezha-dashboard
```

#### 5. 验证部署

```bash
# 检查进程
ps aux | grep nezha-dashboard

# 检查端口
ss -tlnp | grep 5555

# 查看日志
journalctl -u nezha-dashboard -f
```

### Agent 客户端部署

#### 1. 准备目录和配置文件

```bash
mkdir -p /opt/nezha-agent
cp nezha-agent /opt/nezha-agent/
cp config.yml /opt/nezha-agent/
chmod +x /opt/nezha-agent/nezha-agent
```

#### 2. 最小配置文件（`config.yml`）

```yaml
server: "your-dashboard-domain:5555"
client_secret: "your-secret-from-dashboard"
tls: false
insecure_tls: false
disable_send_query: false    # 若设为 true，Agent 将拒绝执行 HTTP/TCP/ICMP 探测
skip_connection_count: false
skip_procs_count: false
gpu: false
temperature: false
dns: []
```

> 精简版 Agent 移除了 `disable_command_execute`、`disable_nat`、`disable_auto_update`、`disable_force_update`、`use_gitee_to_upgrade`、`use_atomgit_to_upgrade`、`ip_report_period`、`self_update_period` 等字段，原配置文件中若包含会被忽略。

> `client_secret` 需在 Dashboard Web 界面中创建服务器后获取。

#### 3. systemd 服务（推荐）

创建 `/etc/systemd/system/nezha-agent.service`：

```ini
[Unit]
Description=Nezha Agent
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/nezha-agent
ExecStart=/opt/nezha-agent/nezha-agent
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now nezha-agent
sudo systemctl status nezha-agent
```

#### 4. 验证部署

```bash
# 检查进程
ps aux | grep nezha-agent

# 检查 gRPC 连接
ss -tn | grep <dashboard-ip>:5555

# 查看日志
journalctl -u nezha-agent -f
```

### Windows 部署

```powershell
# 后台运行 Dashboard
Start-Process -FilePath ".\nezha-dashboard.exe" -WindowStyle Hidden

# 后台运行 Agent
Start-Process -FilePath ".\nezha-agent.exe" -WindowStyle Hidden
```

或使用 [nssm](https://nssm.cc/) 注册为 Windows 服务。

### Docker 部署（可选）

```dockerfile
# Dockerfile.dashboard
FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY nezha-dashboard /app/
COPY data/config.yaml /app/data/
EXPOSE 5555
CMD ["./nezha-dashboard"]
```

```bash
docker build -f Dockerfile.dashboard -t nezha-dashboard:latest .
docker run -d -p 5555:5555 -v $(pwd)/data:/app/data nezha-dashboard:latest
```

## 从原版迁移

### 兼容性说明

| 组合 | 兼容性 | 说明 |
|------|--------|------|
| 精简 Agent + 精简 Dashboard | **完全兼容** | 推荐组合 |
| 精简 Agent + 原版 Dashboard | **兼容** | 主机监控与服务探测正常，但 Agent 不响应远程配置推送/服务器转移 |
| 原版 Agent + 精简 Dashboard | **兼容** | 主机监控正常，但 Dashboard 不下发 Terminal/FM/Command 等已被移除的任务 |

### 迁移步骤

1. **备份原版数据**
   ```bash
   cp /opt/nezha/data/nezha.db /opt/nezha/data/nezha.db.backup
   cp /opt/nezha/data/config.yaml /opt/nezha/data/config.yaml.backup
   ```

2. **停止原版服务**
   ```bash
   sudo systemctl stop nezha-dashboard nezha-agent
   ```

3. **替换为精简版二进制**
   ```bash
   cp nezha-dashboard /opt/nezha/nezha-dashboard
   cp nezha-agent /opt/nezha-agent/nezha-agent
   ```

4. **数据库直接复用**
   > 精简版 `AutoMigrate` 不会删除已有表，原版数据库可直接使用。精简版移除了部分表相关功能（如告警规则、Cron 任务），但数据保留，未来切回原版可恢复。

5. **启动服务**
   ```bash
   sudo systemctl start nezha-dashboard nezha-agent
   ```

## 与原版功能对比

### Agent 端变化

| 功能 | 原版 | 精简版 | 说明 |
|------|:--:|:--:|:--|
| `edit` 交互配置命令 | :white_check_mark: | :x: | 移除终端交互式配置，改为纯配置文件驱动 |
| `service` 系统服务命令 | :white_check_mark: | :x: | 移除 install/uninstall/start/stop/restart 系统服务封装 |
| 自更新（SelfUpdate） | :white_check_mark: | :x: | 移除自动下载替换二进制，更新由运维控制 |
| 远程配置推送（ApplyConfig） | :white_check_mark: | :x: | 移除 Dashboard 远程修改 Agent 配置 |
| 服务器转移（ServerTransfer） | :white_check_mark: | :x: | 移除密钥轮换与服务器转移流程 |
| 主机状态上报 | :white_check_mark: | :white_check_mark: | **保留** CPU/内存/磁盘/网络/负载/温度/GPU 完整采集 |
| 服务延迟探测 | :white_check_mark: | :white_check_mark: | **保留** HTTP/TCP/ICMP 探测与结果回传 |
| gRPC 双向流 | :white_check_mark: | :white_check_mark: | **保留** RequestTask + ReportSystemState |
| 心跳保活 | :white_check_mark: | :white_check_mark: | **保留** Keepalive + 定时状态上报 |

**依赖精简**：移除了 `survey/v2`、`go-github-selfupdate`、`service`、`fsnotify`、`cli/v2` 等库，二进制体积更小。

### Dashboard 端变化

| 功能 | 原版 | 精简版 | 说明 |
|------|:--:|:--:|:--|
| 命令行参数（-c / -db / -v） | :white_check_mark: | :x: | 移除 flag 解析，配置路径固定为 `data/config.yaml` 和 `data/sqlite.db` |
| MCP 端点 | :white_check_mark: | :x: | 移除 LLM 远程执行接口 |
| NAT 内网穿透 | :white_check_mark: | :x: | 移除内网 TCP 转发 |
| DDNS 动态域名 | :white_check_mark: | :white_check_mark: | **保留** IP 变更自动更新 DNS |
| 服务器转移 | :white_check_mark: | :x: | 移除所有权转移流程 |
| Terminal / FM WebSocket | :white_check_mark: | :x: | 移除交互式 Shell 与文件管理 |
| Cron 远程执行 | :white_check_mark: | :x: | 移除定时任务下发 |
| OAuth2 第三方登录 | :white_check_mark: | :x: | 移除 GitHub/Gitee 等 OAuth2 登录 |
| 告警规则与通知 | :white_check_mark: | :x: | 移除告警触发与通知渠道 |
| Debug Swagger / pprof | :white_check_mark: | :x: | 移除 API 文档与性能分析接口 |
| 主机监控展示 | :white_check_mark: | :white_check_mark: | **保留** CPU/内存/磁盘/网络/负载/GPU/温度 |
| 服务监控（延迟探测） | :white_check_mark: | :white_check_mark: | **保留** 配置服务 → Agent 执行 → 结果统计 |
| gRPC 服务 | :white_check_mark: | :white_check_mark: | **保留** Agent 连接、状态上报、任务下发 |
| JWT / PAT 认证 | :white_check_mark: | :white_check_mark: | **保留** 浏览器会话 + API Token |
| WAF / IP 封禁 | :white_check_mark: | :white_check_mark: | **保留** 访问控制与安全防护 |
| TSDB 时序数据库 | :white_check_mark: | :white_check_mark: | **保留** 指标存储与历史查询 |

### 核心设计差异

```diff
# Agent 启动方式
- nezha-agent edit              # 交互式配置（已移除）
- nezha-agent service install   # 注册系统服务（已移除）
+ nezha-agent -c config.yml     # 仅保留配置文件路径参数

# Dashboard 启动方式
- ./nezha-dashboard -c config.yaml -db sqlite.db  # 命令行参数（已移除）
+ ./nezha-dashboard                               # 固定路径，直接运行
```

### 保留的核心能力

- **主机监控**：完整的 CPU、内存、磁盘、网络、负载、温度、GPU 采集与展示
- **服务监控**：Dashboard 配置 HTTP/TCP/ICMP 探测任务，Agent 执行并回传延迟数据
- **gRPC 通信**：Agent 上报系统状态、接收探测任务；Dashboard 下发任务、收集结果
- **心跳保活**：`Keepalive` 任务 + `ReportSystemState` 流，确保节点在线状态准确
- **认证与权限**：JWT 浏览器会话、PAT API Token、用户管理、服务器组隔离
- **WAF 防护**：IP 封禁、在线用户追踪、请求频率限制
- **TSDB 时序存储**：监控指标持久化与历史趋势查询

## 项目结构

```
nezha/
├── agent/                  # 精简后的 Agent 客户端
├── nezha/                  # 精简后的 Dashboard 服务端
├── README.md               # 本文件
├── MIGRATION.md            # 迁移指南
└── SERVICE_RECOVERY_PLAN.md # Service 功能恢复方案
```

## 安全提示

- 即使代码已移除相关功能，仍建议在 Agent 配置中显式关闭 `disable_command_execute` 和 `disable_nat`，作为纵深防御。
- Dashboard 的 `debug` 务必保持 `false`。
- 建议将 Dashboard 部署在内网或配合反向代理 + IP 白名单使用。
- gRPC 端口（默认 5555）建议仅对 Agent 开放，不暴露到公网。

## 许可证

与原项目一致，遵循原仓库许可证条款。
