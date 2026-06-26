# Nezha Lite

> 基于 [Nezha Monitoring](https://github.com/nezhahq/nezha) 的精简分支，**保留核心主机监控、服务延迟探测、告警通知与 OAuth2 第三方登录能力**，移除交互式命令、自更新、远程配置推送、IOStream（终端/文件管理/内网穿透/MCP）及 Dashboard 侧的部分高风险功能。适合追求最小攻击面、仅需「看状态 + 测延迟 + 收告警 + OAuth2 登录」的场景。

## 许可证

本项目基于 [Nezha Monitoring](https://github.com/nezhahq/nezha) 精简改编，遵循与原项目相同的 **[Apache License 2.0](LICENSE)**。

- 原项目版权：Copyright 2020 naiba
- 本项目修改：见 [NOTICE](NOTICE) 文件

根据 Apache 2.0 许可证要求，所有修改过的源文件已在头部添加修改声明。

---

## 快速开始

### 一键安装 Dashboard

```bash
curl -L https://raw.githubusercontent.com/Rawwiin/nezha-lite/main/nezha/install.sh -o nezha.sh && chmod +x nezha.sh && sudo ./nezha.sh
```

### 一键安装 Agent

**Linux / macOS / FreeBSD：**

```bash
curl -L https://raw.githubusercontent.com/Rawwiin/nezha-lite/main/agent/install.sh -o nezha.sh && chmod +x nezha.sh && sudo NZ_SERVER=example.com:8008 NZ_CLIENT_SECRET=your-secret NZ_TLS=false ./nezha.sh
```

**Windows（PowerShell）：**

```powershell
curl -L https://raw.githubusercontent.com/Rawwiin/nezha-lite/main/agent/install.ps1 -o install.ps1
$env:NZ_SERVER="example.com:8008"; $env:NZ_CLIENT_SECRET="your-secret"; $env:NZ_TLS="false"
powershell -ExecutionPolicy Bypass -File install.ps1
```

> 实际使用时，`NZ_SERVER`、`NZ_CLIENT_SECRET` 等参数由 Dashboard 前端在「添加服务器」时自动填充到安装命令中，用户无需手动修改。

> 编译、打包、Docker 镜像构建等开发相关操作请参考 [build.md](build.md)。

---

## Dashboard 安装

精简版提供自包含的安装脚本，无需依赖外部脚本仓库，配置文件和服务文件均内联生成。支持 **Docker** 和 **独立安装** 两种方式，自动检测中国地区并使用 GitHub 公共代理加速下载：

```bash
# 下载并运行安装脚本
curl -L https://raw.githubusercontent.com/Rawwiin/nezha-lite/main/nezha/install.sh -o nezha.sh && chmod +x nezha.sh && sudo ./nezha.sh
```

脚本支持交互式菜单和命令行直接调用：

```bash
./nezha.sh                    # 显示管理菜单（选择 Docker 或独立安装）
./nezha.sh install            # 安装面板端
./nezha.sh modify_config      # 修改面板配置
./nezha.sh restart_and_update # 重启并更新面板
./nezha.sh show_log           # 查看面板日志
./nezha.sh uninstall          # 卸载管理面板
```

### 安装方式

| 方式 | 说明 | 依赖 |
|------|------|------|
| **Docker** | 通过 docker-compose 编排，镜像从 DockerHub/GHCR 拉取 | Docker + Docker Compose |
| **独立安装** | 直接下载二进制运行，注册为 systemd/openrc 服务 | 无额外依赖 |

### 中国地区加速

脚本启动时通过 Cloudflare GeoIP 自动检测是否在中国大陆。检测到中国 IP 时会提示使用 GitHub 公共代理加速下载，支持以下代理（按优先级自动尝试）：

- `https://ghfast.top`
- `https://gh-proxy.com`
- `https://mirror.ghproxy.com`

也可手动指定代理或强制开启：

```bash
# 强制使用中国代理
CN=true sudo ./nezha.sh

# 指定自定义代理
# 安装时选择 "3. 使用自定义代理" 并输入代理地址
```

### 自定义镜像

脚本顶部可配置变量：

```bash
# Docker 镜像（上传 DockerHub 后修改为自己的镜像名）
Docker_IMG="rawwiin/nezha-lite"
# 例如: Docker_IMG="rawwiin/nezha-lite:latest"

# GitHub 仓库（用于下载 Release 二进制）
GITHUB_REPO="Rawwiin/nezha-lite"
GITHUB_URL="github.com"
```

也可通过环境变量覆盖：

```bash
# 使用自定义 Docker 镜像安装
Docker_IMG="rawwiin/nezha-lite:latest" sudo ./nezha.sh
```

> **与原版脚本的区别**：精简版脚本不依赖外部 `nezhahq/scripts` 仓库下载配置模板和服务文件，全部内联生成。移除了 Gitee 镜像和 `install_agent_v0` 跳转，改为 GitHub 公共代理方案。

---

## Dashboard 部署

### 1. 准备目录和配置文件

```bash
mkdir -p /opt/nezha/data
cp nezha-dashboard /opt/nezha/
cp config.yaml /opt/nezha/data/
chmod +x /opt/nezha/nezha-dashboard
```

### 2. 最小配置文件（`data/config.yaml`）

```yaml
site_name: "Nezha"
language: "zh_CN"
location: "Asia/Shanghai"
listen_port: 8008
jwt_secret_key: ""
debug: false
enable_mcp: false

tsdb:
  data_path: "data/tsdb"
  retention_days: 30
```

> 首次启动后，`jwt_secret_key` 会自动生成。数据库固定为同目录 `data/sqlite.db`，自动创建。

> **与原版配置兼容性**：原版的 `config.yaml` 可直接复用。精简版支持原版的字段（如 `tls`, `listen_port`, `jwt_timeout`, `jwt_secret_key_last_rotated_version`, `web_real_ip_header`, `user_template`, `memory`, `tsdb`, `https`, `oauth2` 等），无需修改。注意：精简版 HTTP 和 gRPC 共用 `listen_port`，原版中若使用 `grpc_port` 需要在 Agent 中将 `server` 改为 `<host>:<listen_port>`。

> **生产环境推荐**：通过环境变量 `NZ_JWTSECRETKEY` 注入 JWT 密钥，避免将密钥写入配置文件落盘。

### OAuth2 第三方登录配置（可选）

使用 OAuth2 登录前，必须配置 `dashboard_host`（防止 Host 头伪造劫持）。在 `config.yaml` 中添加：

```yaml
dashboard_host: "panel.example.com"  # Dashboard 对外访问域名（必须配置）

oauth2:
  github:
    client_id: "your_client_id"
    client_secret: "your_client_secret"  # 推荐改用环境变量注入（见下方）
    endpoint:
      auth_url: "https://github.com/login/oauth/authorize"
      token_url: "https://github.com/login/oauth/access_token"
    user_info_url: "https://api.github.com/user"
    user_id_path: "id"
    scopes:
      - "read:user"
```

> **安全推荐**：通过环境变量注入 ClientSecret，避免明文写入 config.yaml：
> ```bash
> export NZ_OAUTH2_GITHUB_CLIENT_SECRET="your_client_secret"
> ```
> 环境变量格式为 `NZ_OAUTH2_<PROVIDER>_CLIENT_SECRET`（provider 名称大写，`-` 替换为 `_`）。环境变量注入的值不会被 `Save()` 写回磁盘。

> **SSRF 防护**：`user_info_url` 会经过受限 HTTP 客户端校验（CIDR 黑名单 + IP 钉死 + 禁止重定向），无法访问内网地址。

### 3. 启动方式

```bash
# 直接运行（使用默认路径 data/config.yaml 和 data/sqlite.db）
./nezha-dashboard

# 查看版本号
./nezha-dashboard -v

# 指定配置文件和数据库路径
./nezha-dashboard -c /path/to/config.yaml -db /path/to/sqlite.db
```

**命令行参数**：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-v` | — | 查看当前版本号，打印后立即退出 |
| `-c` | `data/config.yaml` | 配置文件路径 |
| `-db` | `data/sqlite.db` | SQLite 数据库文件路径 |

> 如不指定参数，默认从可执行文件同目录的 `data/` 子目录读取配置和数据库。

### 4. systemd 服务（推荐）

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

### 5. 验证部署

```bash
# 检查进程
ps aux | grep nezha-dashboard

# 检查端口
ss -tlnp | grep 8008

# 查看日志
journalctl -u nezha-dashboard -f
```

### Docker 部署

使用预构建镜像（推荐）：

```bash
# 创建数据目录和配置文件
mkdir -p data
cat > data/config.yaml <<'EOF'
site_name: "Nezha Lite"
language: "zh_CN"
listen_port: 8008
tls: false
debug: false
EOF

# 拉取镜像并运行
docker run -d \
  --name nezha-dashboard \
  --restart always \
  -p 8008:8008 \
  -v $(pwd)/data:/dashboard/data \
  -e TZ=Asia/Shanghai \
  rawwiin/nezha-lite:latest
```

或使用 docker-compose：

```bash
# 下载 docker-compose.yaml
curl -L https://raw.githubusercontent.com/Rawwiin/nezha-lite/main/nezha/docker-compose.yaml -o docker-compose.yaml

# 修改 image 为你的 DockerHub 镜像名，然后启动
docker compose up -d
```

> 自行构建 Docker 镜像请参考 [build.md](build.md)。

---

## Agent 部署

### 1. 一键安装（推荐）

精简版提供自包含的 Agent 安装脚本，自动下载二进制并通过 `service` 命令注册系统服务。安装命令由 Dashboard 前端生成，包含连接参数：

**Linux / macOS / FreeBSD：**

```bash
curl -L https://raw.githubusercontent.com/Rawwiin/nezha-lite/main/agent/install.sh -o nezha.sh && chmod +x nezha.sh && sudo NZ_SERVER=example.com:8008 NZ_CLIENT_SECRET=your-secret NZ_TLS=false ./nezha.sh
```

**Windows（PowerShell）：**

```powershell
curl -L https://raw.githubusercontent.com/Rawwiin/nezha-lite/main/agent/install.ps1 -o install.ps1
$env:NZ_SERVER="example.com:8008"; $env:NZ_CLIENT_SECRET="your-secret"; $env:NZ_TLS="false"
powershell -ExecutionPolicy Bypass -File install.ps1
```

**安装脚本支持的环境变量：**

| 变量 | 必填 | 说明 |
|------|------|------|
| `NZ_SERVER` | 是 | Dashboard 地址（例如 `example.com:8008`） |
| `NZ_CLIENT_SECRET` | 是 | 客户端密钥（在 Dashboard 创建服务器时获取） |
| `NZ_TLS` | 否 | 是否启用 TLS（`true`/`false`，默认 `false`） |
| `NZ_INSECURE_TLS` | 否 | 是否跳过 TLS 证书验证（`true`/`false`） |
| `NZ_UUID` | 否 | Agent UUID（留空自动生成） |
| `NZ_GPU` | 否 | 启用 GPU 监控（`true`/`false`） |
| `NZ_TEMPERATURE` | 否 | 启用温度监控（`true`/`false`） |
| `NZ_SKIP_CONNECTION_COUNT` | 否 | 跳过连接数统计（`true`/`false`） |
| `NZ_SKIP_PROCS_COUNT` | 否 | 跳过进程数统计（`true`/`false`） |
| `NZ_DISABLE_SEND_QUERY` | 否 | 禁止执行探测任务（`true`/`false`） |
| `NZ_ALLOW_PROBE_INTERNAL` | 否 | 允许探测内网地址（`true`/`false`，默认拒绝） |
| `NZ_DEBUG` | 否 | 调试日志（`true`/`false`） |
| `CN=true` | 否 | 强制使用 GitHub 代理加速 |

**中国地区加速：** 脚本通过 Cloudflare GeoIP 自动检测中国地区，检测到时自动使用 GitHub 公共代理（`ghfast.top`、`gh-proxy.com`、`mirror.ghproxy.com`）加速下载，无需手动配置。

**卸载：**

```bash
# Linux / macOS
sudo ./nezha.sh uninstall

# Windows
C:\nezha\nezha-agent.exe service uninstall
```

### 2. 手动部署

如果不使用安装脚本，可手动下载二进制并配置：

```bash
# 准备目录
mkdir -p /opt/nezha-agent
cp nezha-agent /opt/nezha-agent/
cp config.yml /opt/nezha-agent/
chmod +x /opt/nezha-agent/nezha-agent
```

**配置文件（`config.yml`）：**

```yaml
# Dashboard 连接
server: "your-dashboard-domain:8008"
client_secret: "your-secret-from-dashboard"
tls: false
insecure_tls: false

# 监控开关
debug: true                      # false 时静默运行（不输出日志）
gpu: false                       # GPU 使用率监控
temperature: false               # 温度传感器监控
skip_connection_count: false     # 跳过 TCP/UDP 连接数统计
skip_procs_count: false          # 跳过进程数统计

# 探测控制
disable_send_query: false        # true 时拒绝执行 HTTP/TCP/ICMP 探测
allow_probe_internal: false      # true 时允许探测内网地址（默认拒绝，防 SSRF）

# 上报参数
report_delay: 3                  # 状态上报间隔（秒，范围 1-4）
ip_report_period: 1800           # IP 上报周期（秒，最小 30）
use_ipv6_country_code: false     # 优先展示 IPv6 地理位置旗帜

# 自定义 DNS 和 IP API（可选）
dns: []                          # 自定义 DNS 服务器列表
custom_ip_api: []                # 自定义外网 IP 查询 API
nic_allowlist: {}                # 网卡白名单（map[nic名]bool）
hard_drive_partition_allowlist: []  # 磁盘分区白名单
```

> `client_secret` 需在 Dashboard Web 界面中创建服务器后获取。

### 3. 启动方式

**方式一：service 命令（推荐）**

Agent 内置 `service` 子命令，可自动注册/管理 systemd（Linux）、Windows Service、launchd（macOS）系统服务：

```bash
# 注册为系统服务（自动生成 systemd unit / Windows 服务）
sudo ./nezha-agent service install

# 指定配置文件路径注册（多实例场景）
sudo ./nezha-agent service install -c /opt/nezha-agent/config.yml

# 启动 / 停止 / 重启 / 卸载
sudo ./nezha-agent service start
sudo ./nezha-agent service stop
sudo ./nezha-agent service restart
sudo ./nezha-agent service uninstall
```

> `service install` 会自动检测当前 init 系统（systemd / launchd / Windows Service）并生成对应配置。非默认配置路径时，服务名会追加路径哈希后缀以支持多实例。

**方式二：手动 systemd 服务**

如果不使用 `service` 命令，可手动创建 `/etc/systemd/system/nezha-agent.service`：

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

### 4. 验证部署

```bash
# 检查进程
ps aux | grep nezha-agent

# 检查 gRPC 连接
ss -tn | grep <dashboard-ip>:8008

# 查看日志
journalctl -u nezha-agent -f
```

### Windows 部署

```powershell
# 方式一：使用 service 命令注册为 Windows 服务（推荐）
.\nezha-agent.exe service install
.\nezha-agent.exe service start

# 方式二：后台运行
Start-Process -FilePath ".\nezha-agent.exe" -WindowStyle Hidden
```

或使用 [nssm](https://nssm.cc/) 注册为 Windows 服务。

---

## 从原版迁移

### 兼容性说明

| 组合 | 兼容性 | 说明 |
|------|--------|------|
| 精简 Agent + 精简 Dashboard | **完全兼容** | 推荐组合 |
| 精简 Agent + 原版 Dashboard | **兼容** | 主机监控与服务探测正常，但 Agent 不响应远程配置推送/服务器转移/IOStream |
| 原版 Agent + 精简 Dashboard | **兼容** | 主机监控正常，但 Dashboard 不下发 Terminal/FM/Command/IOStream 等已被移除的任务 |

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
   > 精简版 `AutoMigrate` 不会删除已有表，原版数据库可直接使用。精简版移除了部分表相关功能（如 Cron 任务），但告警规则、通知渠道等数据保留并可用，未来切回原版也可恢复。

5. **启动服务**
   ```bash
   sudo systemctl start nezha-dashboard nezha-agent
   ```

---

## 与原版功能对比

### Agent 端变化

| 功能 | 原版 | 精简版 | 说明 |
|------|:--:|:--:|:--|
| `edit` 交互配置命令 | :white_check_mark: | :x: | 移除终端交互式配置，改为纯配置文件驱动 |
| `service` 系统服务命令 | :white_check_mark: | :white_check_mark: | **保留** install/uninstall/start/stop/restart 系统服务封装 |
| 自更新（SelfUpdate） | :white_check_mark: | :x: | 移除自动下载替换二进制，更新由运维控制 |
| 远程配置推送（ApplyConfig） | :white_check_mark: | :x: | 移除 Dashboard 远程修改 Agent 配置 |
| 服务器转移（ServerTransfer） | :white_check_mark: | :x: | 移除密钥轮换与服务器转移流程 |
| Cron 定时任务 | :white_check_mark: | :x: | 移除定时任务下发 |
| 主机状态上报 | :white_check_mark: | :white_check_mark: | **保留** CPU/内存/磁盘/网络/负载/温度/GPU 完整采集 |
| 服务延迟探测 | :white_check_mark: | :white_check_mark: | **保留** HTTP/TCP/ICMP 探测与结果回传 |
| gRPC 双向流 | :white_check_mark: | :white_check_mark: | **保留** RequestTask + ReportSystemState |
| 心跳保活 | :white_check_mark: | :white_check_mark: | **保留** Keepalive + 定时状态上报 |
| GeoIP 上报 | :white_check_mark: | :white_check_mark: | **保留** 外网 IP 获取与地理位置上报 |
| uTLS 指纹模拟 | :white_check_mark: | :white_check_mark: | **保留** HTTP 探测使用 Chrome TLS 指纹 |
| 自定义 DNS | :white_check_mark: | :white_check_mark: | **保留** 可配置 DNS 服务器列表 |

**安全增强**：

| 安全特性 | 原版 | 精简版 | 说明 |
|---------|:--:|:--:|:--|
| SSRF 防护 | :x: | :white_check_mark: | HTTP/TCP/ICMP 探测默认拒绝内网/保留地址，可配置 `allow_probe_internal` 开启 |
| HTTP 响应体大小限制 | :x: | :white_check_mark: | 限制 1MB，防止恶意大文件导致内存耗尽 |
| ICMP 并发限制 | :x: | :white_check_mark: | 最多 3 个并发 ICMP 任务，防止 DDoS 放大 |
| 日志静默 | :x: | :white_check_mark: | `debug: false` 时静默运行，不输出连接地址/任务内容等敏感信息 |
| 配置文件权限 | :x: | :white_check_mark: | `config.yml` 强制 `chmod 0600`，防止 ClientSecret 泄露 |
| goroutine 泄漏修复 | :x: | :white_check_mark: | gRPC unary 调用使用带超时的 context，避免长期运行内存增长 |

**依赖精简**：移除了 `survey/v2`、`go-github-selfupdate`、`fsnotify`（直接依赖）、`cli/v2`、`go-gitee`、`go-github`、`go-update`、`go-gitconfig`、`go-shellquote`、`xsyslog`、`gomega`、`colorable`、`isatty`、`ansi` 等库，二进制体积更小，攻击面更小。保留了 `nezhahq/service` 用于系统服务管理。

### Dashboard 端变化

| 功能 | 原版 | 精简版 | 说明 |
|------|:--:|:--:|:--|
| 命令行参数（-c / -db / -v） | :white_check_mark: | :white_check_mark: | **保留** `-c` 配置路径、`-db` 数据库路径、`-v` 版本号 |
| IOStream（终端/文件管理/内网穿透） | :white_check_mark: | :x: | gRPC IOStream handler 改为 Unimplemented 桩 |
| MCP 端点 | :white_check_mark: | :x: | 移除 LLM 远程执行接口（`enable_mcp` 默认 false） |
| NAT 内网穿透 | :white_check_mark: | :x: | 移除内网 TCP 转发 |
| DDNS 动态域名 | :white_check_mark: | :white_check_mark: | **保留** IP 变更自动更新 DNS |
| 服务器转移 | :white_check_mark: | :x: | 移除所有权转移流程 |
| Terminal / FM WebSocket | :white_check_mark: | :x: | 移除交互式 Shell 与文件管理 |
| Cron 远程执行 | :white_check_mark: | :x: | 移除定时任务下发 |
| OAuth2 第三方登录 | :white_check_mark: | :white_check_mark: | **保留** GitHub/Gitee 等 OAuth2 登录，已加固 SSRF 防护、响应体限制、DashboardHost 强制、环境变量注入 ClientSecret |
| 告警规则与通知 | :white_check_mark: | :white_check_mark: | **保留** AlertSentinel 告警检查 + 通知渠道（HTTP/Webhook）完整可用 |
| 服务监控状态通知 | :white_check_mark: | :x: | 服务状态变更时不发送通知（`notifyCheck` 仅打日志），告警规则仍可监控服务状态 |
| Debug Swagger / pprof | :white_check_mark: | :x: | 移除 API 文档与性能分析接口 |
| ReportSystemInfo (v1) | :white_check_mark: | :x: | 改为 Unimplemented 桩，Agent 统一使用 ReportSystemInfo2 |
| 主机监控展示 | :white_check_mark: | :white_check_mark: | **保留** CPU/内存/磁盘/网络/负载/GPU/温度 |
| 服务监控（延迟探测） | :white_check_mark: | :white_check_mark: | **保留** 配置服务 → Agent 执行 → 结果统计 |
| gRPC 服务 | :white_check_mark: | :white_check_mark: | **保留** Agent 连接、状态上报、任务下发、GeoIP |
| JWT / PAT 认证 | :white_check_mark: | :white_check_mark: | **保留** 浏览器会话 + API Token |
| WAF / IP 封禁 | :white_check_mark: | :white_check_mark: | **保留** 访问控制与安全防护 |
| TSDB 时序数据库 | :white_check_mark: | :white_check_mark: | **保留** 指标存储与历史查询 |
| HTTPS 配置 | :white_check_mark: | :white_check_mark: | **保留** 独立 HTTPS 端口与证书配置 |

### 核心设计差异

```diff
# Agent 启动方式
- nezha-agent edit              # 交互式配置（已移除）
+ nezha-agent -c config.yml     # 配置文件路径参数
+ nezha-agent service install   # 注册为系统服务（保留）

# Dashboard 启动方式
- ./nezha-dashboard -c config.yaml -db sqlite.db  # 原版命令行参数（保留）
+ ./nezha-dashboard                               # 不带参数时使用默认路径 data/
```

### 保留的核心能力

- **主机监控**：完整的 CPU、内存、磁盘、网络、负载、温度、GPU 采集与展示
- **服务监控**：Dashboard 配置 HTTP/TCP/ICMP 探测任务，Agent 执行并回传延迟数据
- **告警通知**：AlertSentinel 告警规则检查 + 通知渠道（HTTP/Webhook）+ 防骚扰策略
- **gRPC 通信**：Agent 上报系统状态、接收探测任务；Dashboard 下发任务、收集结果
- **心跳保活**：`Keepalive` 任务 + `ReportSystemState` 流，确保节点在线状态准确
- **GeoIP**：Agent 获取外网 IP，Dashboard 查询地理位置数据库并更新 DDNS
- **认证与权限**：JWT 浏览器会话、PAT API Token、OAuth2 第三方登录、用户管理、服务器组隔离
- **WAF 防护**：IP 封禁、在线用户追踪、请求频率限制
- **TSDB 时序存储**：监控指标持久化与历史趋势查询

---

## Agent 配置字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `debug` | bool | false | 日志开关：`true` 输出全部日志，`false` 静默运行 |
| `server` | string | — | Dashboard 地址（`host:port`） |
| `client_secret` | string | — | 客户端密钥，在 Dashboard 创建服务器后获取 |
| `uuid` | string | 自动生成 | Agent 唯一标识，首次启动自动生成并写盘 |
| `tls` | bool | false | 是否使用 TLS 加密 gRPC 传输 |
| `insecure_tls` | bool | false | 是否跳过 TLS 证书验证（**不推荐**） |
| `gpu` | bool | false | GPU 使用率监控（NVIDIA/AMD/Intel/macOS） |
| `temperature` | bool | false | 温度传感器监控 |
| `skip_connection_count` | bool | false | 跳过 TCP/UDP 连接数统计 |
| `skip_procs_count` | bool | false | 跳过进程数统计 |
| `disable_send_query` | bool | false | 拒绝执行所有 HTTP/TCP/ICMP 探测任务 |
| `allow_probe_internal` | bool | false | 允许探测内网/保留地址（**默认拒绝，防 SSRF**） |
| `report_delay` | uint32 | 3 | 状态上报间隔（秒，范围 1-4） |
| `ip_report_period` | uint32 | 1800 | IP 上报周期（秒，最小 30） |
| `use_ipv6_country_code` | bool | false | 优先展示 IPv6 地理位置旗帜 |
| `dns` | []string | — | 自定义 DNS 服务器列表 |
| `custom_ip_api` | []string | — | 自定义外网 IP 查询 API 地址 |
| `nic_allowlist` | map[string]bool | — | 网卡流量统计白名单 |
| `hard_drive_partition_allowlist` | []string | — | 磁盘分区白名单 |

> 所有字段均支持通过 `NZ_` 前缀环境变量覆盖，例如 `NZ_SERVER`、`NZ_CLIENT_SECRET` 等。

---

## 项目结构

```
nezha/
├── agent/                      # Agent 客户端
│   ├── cmd/agent/              # 入口：gRPC 连接、状态上报、任务执行
│   │   ├── main.go             # 主循环、任务分发、service 命令
│   │   ├── probe_guard.go      # SSRF 防护：探测目标地址校验
│   │   └── commands/           # service 子命令（Program 结构体）
│   ├── model/                  # 配置、认证、数据结构
│   ├── pkg/
│   │   ├── monitor/            # 硬件监控（CPU/GPU/磁盘/网卡/温度/连接数/负载）
│   │   ├── utls/               # uTLS 指纹模拟 HTTP RoundTripper
│   │   ├── logger/             # 日志（支持 debug 开关）
│   │   └── util/               # DNS、HTTP 客户端等工具
│   ├── proto/                  # gRPC protobuf 生成代码
│   ├── install.sh              # Agent 一键安装脚本（Linux/macOS）
│   └── install.ps1             # Agent 一键安装脚本（Windows）
│
├── nezha/                      # Dashboard 服务端
│   ├── cmd/dashboard/          # 入口：HTTP + gRPC 复用端口
│   ├── service/rpc/            # gRPC 服务端（Agent 连接、状态接收、任务下发）
│   ├── model/                  # 数据模型与配置
│   ├── pkg/                    # TSDB、GeoIP、WAF 等工具包
│   ├── Dockerfile              # 多阶段构建（前端下载→Go编译→运行时）
│   ├── docker-compose.yaml     # docker-compose 示例
│   └── install.sh              # 一键安装脚本（自包含）
│
├── admin-frontend/             # 管理端前端源码
├── LICENSE                     # Apache License 2.0
├── NOTICE                      # 修改声明（Apache 2.0 要求）
├── README.md                   # 本文件（安装、部署、迁移、功能对比）
├── build.md                    # 构建指南（编译、打包、Docker 镜像）
└── MIGRATION.md                # 迁移指南
```

---

## 安全提示

- **SSRF 防护（Agent 探测）**：Agent 默认拒绝探测内网/保留地址（10/8、172.16/12、192.168/16、127/8、169.254/16 等）。如需监控内网服务，设置 `allow_probe_internal: true`。
- **SSRF 防护（告警通知）**：Dashboard 发送告警通知时使用受限 HTTP 客户端，同样拒绝内网/保留地址，并钉死目标 IP 防止 DNS 重绑定，禁止跟随重定向。
- **SSRF 防护（OAuth2 UserInfo）**：OAuth2 回调获取用户信息时使用受限 HTTP 客户端，对 `user_info_url` 做内网地址过滤 + IP 钉死 + 禁止重定向，防止恶意 IdP 配置导致 SSRF。
- **OAuth2 DashboardHost 强制**：使用 OAuth2 功能前必须配置 `dashboard_host`，否则 OAuth2 请求会返回错误。这防止了未配置时 Host 头透传导致的开放重定向漏洞。
- **OAuth2 ClientSecret 环境变量注入**：推荐通过环境变量 `NZ_OAUTH2_<PROVIDER>_CLIENT_SECRET` 注入 ClientSecret（如 `NZ_OAUTH2_GITHUB_CLIENT_SECRET`），避免明文写入 config.yaml。环境变量注入的值不会被 `Save()` 写回磁盘。
- **OAuth2 响应体限制**：UserInfo 响应体限制 1MB，防止恶意 IdP 返回超大响应导致 Dashboard OOM。
- **OAuth2 CSRF 防护**：OAuth2 流程使用 state 参数 + `nz-o2s` HttpOnly Cookie 双重校验，回调失败时触发 WAF IP 封禁。
- **通知 IP 打码**：默认 `enable_plain_ip_in_notification: false`，告警消息中服务器 IP 会被打码。如需显示完整 IP 请谨慎评估泄露风险。
- **通知凭据安全**：通知 URL/Header 中的 Webhook Token 等凭据以明文存储在 SQLite 数据库中。请确保 `data/sqlite.db` 文件权限 `0600`，并将 Dashboard 部署在内网。
- **日志安全**：生产环境建议设置 `debug: false`，避免日志泄露连接地址和任务内容。
- **TLS 配置**：推荐 `tls: true`，避免 `insecure_tls: true`（跳过证书验证存在 MITM 风险）。
- **配置文件权限**：Agent 自动将 `config.yml` 权限设为 `0600`，防止 ClientSecret 被同主机其他用户读取。
- **JWT 密钥**：生产环境推荐通过环境变量 `NZ_JWTSECRETKEY` 注入，避免写入配置文件落盘。
- **Dashboard 的 `debug`** 务必保持 `false`。
- 建议将 Dashboard 部署在内网或配合反向代理 + IP 白名单使用。
- gRPC 端口（默认 8008）建议仅对 Agent 开放，不暴露到公网。

---

## 致谢

本项目基于以下开源项目精简改编，感谢原作者及贡献者的辛勤工作：

- **[Nezha Monitoring](https://github.com/nezhahq/nezha)** — Copyright 2020 naiba，Apache License 2.0
- **[admin-frontend](https://github.com/nezhahq/admin-frontend)** — 管理端前端，Apache License 2.0
- **[nezha-dash-v2](https://github.com/hamster1963/nezha-dash-v2)** — 用户端前端，Apache License 2.0

## 许可证

[Apache License 2.0](LICENSE) — 与原项目一致。修改详情见 [NOTICE](NOTICE)。
