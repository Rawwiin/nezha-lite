# Nezha 精简版 Agent 安装脚本（Windows）
# 自包含：支持 GitHub 公共代理加速，通过 service 命令注册 Windows 服务
#
# 前端生成的安装命令：
#   curl -L https://raw.githubusercontent.com/Rawwiin/nezha-lite/main/agent/install.ps1 -o install.ps1 && powershell -ExecutionPolicy Bypass -File install.ps1
#
# 环境变量（由前端通过安装命令注入）：
#   NZ_SERVER          - Dashboard 地址（必填，例如 example.com:8008）
#   NZ_CLIENT_SECRET   - 客户端密钥（必填）
#   NZ_TLS             - 是否启用 TLS（true/false）
#   NZ_INSECURE_TLS    - 是否跳过 TLS 证书验证（true/false）
#   NZ_UUID            - Agent UUID（可选，留空自动生成）
#   NZ_GPU             - 启用 GPU 监控（true/false）
#   NZ_TEMPERATURE     - 启用温度监控（true/false）
#   NZ_SKIP_CONNECTION_COUNT - 跳过连接数统计（true/false）
#   NZ_SKIP_PROCS_COUNT     - 跳过进程数统计（true/false）
#   NZ_DISABLE_SEND_QUERY   - 禁止执行探测任务（true/false）
#   NZ_ALLOW_PROBE_INTERNAL - 允许探测内网地址（true/false）
#   NZ_DEBUG           - 调试日志（true/false）

# PowerShell 版本检查
if ($PSVersionTable.PSVersion.Major -lt 5) {
    Write-Host "需要 PowerShell >= 5，当前版本: $($PSVersionTable.PSVersion.Major)" -BackgroundColor DarkRed -ForegroundColor White
    exit 1
}

# GitHub 仓库（用于下载 Release 二进制）
$agentrepo = "Rawwiin/nezha-lite"

# GitHub 公共代理列表（中国地区自动使用）
$GITHUB_PROXIES = @(
    "https://ghfast.top",
    "https://gh-proxy.com",
    "https://mirror.ghproxy.com"
)

# 检测系统架构
if ([System.Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
        $file = "nezha-agent_windows_arm64.zip"
    } else {
        $file = "nezha-agent_windows_amd64.zip"
    }
} else {
    $file = "nezha-agent_windows_386.zip"
}

# 重复运行自动更新：先卸载旧服务
if (Test-Path "C:\nezha\nezha-agent.exe") {
    Write-Host "检测到已安装的 nezha-agent，将卸载旧版本并重新安装" -BackgroundColor DarkGreen -ForegroundColor White
    C:\nezha\nezha-agent.exe service uninstall 2>$null
    Remove-Item "C:\nezha" -Recurse -Force -ErrorAction SilentlyContinue
}

# TLS/SSL
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# 获取最新版本号
Write-Host "正在获取最新 nezha-agent 版本..." -BackgroundColor DarkGreen -ForegroundColor White
$agentreleases = "https://api.github.com/repos/$agentrepo/releases/latest"
try {
    $release = Invoke-RestMethod -Uri $agentreleases -UseBasicParsing -TimeoutSec 10
    $agenttag = $release.tag_name
} catch {
    $agenttag = $null
}

# 备用：通过 jsdelivr CDN 获取版本号
if ([string]::IsNullOrWhiteSpace($agenttag)) {
    $optionUrl = "https://fastly.jsdelivr.net/gh/$agentrepo/"
    try {
        $response = Invoke-WebRequest -Uri $optionUrl -UseBasicParsing -TimeoutSec 10
        if ($response.StatusCode -eq 200) {
            $versiontext = $response.Content | findstr /c:"option.value"
            $version = [regex]::Match($versiontext, "@(\d+\.\d+\.\d+)").Groups[1].Value
            $agenttag = "v" + $version
        }
    } catch {
        $optionUrl = "https://gcore.jsdelivr.net/gh/$agentrepo/"
        try {
            $response = Invoke-WebRequest -Uri $optionUrl -UseBasicParsing -TimeoutSec 10
            if ($response.StatusCode -eq 200) {
                $versiontext = $response.Content | findstr /c:"option.value"
                $version = [regex]::Match($versiontext, "@(\d+\.\d+\.\d+)").Groups[1].Value
                $agenttag = "v" + $version
            }
        } catch {
            $agenttag = $null
        }
    }
}

if ([string]::IsNullOrWhiteSpace($agenttag)) {
    Write-Host "获取 nezha-agent 版本号失败，请检查网络连接" -BackgroundColor DarkRed -ForegroundColor White
    exit 1
}

Write-Host "当前最新版本: $agenttag" -BackgroundColor DarkGreen -ForegroundColor White

# 中国地区检测
$region = "Unknown"
$ipapi = ""
foreach ($url in @("https://dash.cloudflare.com/cdn-cgi/trace", "https://developers.cloudflare.com/cdn-cgi/trace", "https://1.0.0.1/cdn-cgi/trace")) {
    try {
        $ipapi = Invoke-RestMethod -Uri $url -TimeoutSec 5 -UseBasicParsing
        if ($ipapi -match "loc=(\w+)") {
            $region = $Matches[1]
            break
        }
    } catch {
        # 继续尝试下一个
    }
}

# 构造下载 URL
$githubUrl = "https://github.com/$agentrepo/releases/download/$agenttag/$file"
$download = $null

if ($region -eq "CN") {
    Write-Host "检测到中国地区，使用 GitHub 代理加速下载" -BackgroundColor DarkGreen -ForegroundColor White
    # 依次尝试公共代理
    foreach ($proxy in $GITHUB_PROXIES) {
        $proxyUrl = "$proxy/$githubUrl"
        Write-Host "尝试下载: $proxyUrl"
        try {
            Invoke-WebRequest $proxyUrl -OutFile "C:\nezha.zip" -UseBasicParsing -TimeoutSec 120
            $download = $proxyUrl
            break
        } catch {
            Write-Host "代理 $proxy 不可用，尝试下一个..." -ForegroundColor Yellow
        }
    }
    # 代理全部失败后回退到直连
    if ($null -eq $download) {
        Write-Host "所有代理均不可用，尝试直连 GitHub..." -ForegroundColor Yellow
    }
} else {
    Write-Host "地区: $region，直连 GitHub" -BackgroundColor DarkGreen -ForegroundColor White
}

# 如果代理失败或非中国地区，直连下载
if ($null -eq $download) {
    Write-Host "尝试下载: $githubUrl"
    try {
        Invoke-WebRequest $githubUrl -OutFile "C:\nezha.zip" -UseBasicParsing -TimeoutSec 120
    } catch {
        Write-Host "下载失败: $githubUrl" -BackgroundColor DarkRed -ForegroundColor White
        Write-Host "请检查网络连接或手动下载" -BackgroundColor DarkRed -ForegroundColor White
        exit 1
    }
}

# 解压
Write-Host "正在解压..." -BackgroundColor DarkGreen -ForegroundColor White
Expand-Archive "C:\nezha.zip" -DestinationPath "C:\temp" -Force
if (!(Test-Path "C:\nezha")) { New-Item -Path "C:\nezha" -type directory }

# 整理文件
Move-Item -Path "C:\temp\nezha-agent.exe" -Destination "C:\nezha\nezha-agent.exe" -Force

# 清理临时文件
Remove-Item "C:\nezha.zip"
Remove-Item "C:\temp" -Recurse

# 生成配置文件（通过环境变量注入）
$configPath = "C:\nezha\config.yml"
$configContent = @"
server: "$env:NZ_SERVER"
client_secret: "$env:NZ_CLIENT_SECRET"
tls: $env:NZ_TLS
insecure_tls: $env:NZ_INSECURE_TLS
uuid: "$env:NZ_UUID"
gpu: $env:NZ_GPU
temperature: $env:NZ_TEMPERATURE
skip_connection_count: $env:NZ_SKIP_CONNECTION_COUNT
skip_procs_count: $env:NZ_SKIP_PROCS_COUNT
disable_send_query: $env:NZ_DISABLE_SEND_QUERY
allow_probe_internal: $env:NZ_ALLOW_PROBE_INTERNAL
debug: $env:NZ_DEBUG
"@

# 写入配置文件（仅当不存在时，避免覆盖已有配置）
if (!(Test-Path $configPath)) {
    $configContent | Out-File -FilePath $configPath -Encoding UTF8
    Write-Host "配置文件已生成: $configPath" -BackgroundColor DarkGreen -ForegroundColor White
} else {
    Write-Host "配置文件已存在，跳过生成: $configPath" -ForegroundColor Yellow
}

# 注册为 Windows 服务
Write-Host "正在注册 Windows 服务..." -BackgroundColor DarkGreen -ForegroundColor White
C:\nezha\nezha-agent.exe service install -c $configPath

Write-Host "nezha-agent 安装成功!" -BackgroundColor DarkGreen -ForegroundColor White
Write-Host "服务管理: C:\nezha\nezha-agent.exe service start|stop|restart|uninstall" -ForegroundColor Cyan
