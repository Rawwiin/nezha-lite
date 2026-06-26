# Nezha 精简版 Docker 镜像构建脚本（Windows → Linux 服务器）
# 在 Windows 上交叉编译 Linux 二进制，传到服务器打包成 Docker 镜像
#
# 用法：
#   .\build-docker.ps1                              # 编译 amd64 并打包
#   .\build-docker.ps1 -Arch arm64                  # 编译 arm64 并打包
#   .\build-docker.ps1 -Arch amd64 -BuildOnly       # 仅编译，不传到服务器
#   .\build-docker.ps1 -Server user@1.2.3.4         # 指定服务器地址
#   .\build-docker.ps1 -ImageName ghcr.io/user/nezha-lite:latest  # 指定镜像名
#
# 镜像推送到 GHCR（GitHub Container Registry）：
#   服务器上执行：echo "ghp_xxx" | docker login ghcr.io -u 用户名 --password-stdin
#   然后推送：docker push ghcr.io/user/nezha-lite:latest

param(
    [string]$Arch = "amd64",
    [string]$Server = "",
    [string]$ImageName = "ghcr.io/rawwiin/nezha-lite:latest",
    [switch]$BuildOnly = $false
)

$ErrorActionPreference = "Stop"

# 项目路径
$ProjectRoot = "C:\workspace\nezha"
$DashboardDir = "$ProjectRoot\nezha"
$BinaryName = "dashboard-linux-$Arch"

Write-Host "===== Nezha Dashboard Docker 镜像构建 =====" -ForegroundColor Cyan
Write-Host "架构: $Arch"
Write-Host "镜像名: $ImageName"
Write-Host ""

# ===== 步骤 1：交叉编译 Linux 二进制 =====
Write-Host "[1/4] 交叉编译 Linux $Arch 二进制..." -ForegroundColor Yellow

$env:GOOS = "linux"
$env:GOARCH = $Arch
$env:CGO_ENABLED = "0"

Push-Location $DashboardDir
try {
    go build -tags modernc -ldflags "-s -w" -trimpath -o $BinaryName ./cmd/dashboard
    if ($LASTEXITCODE -ne 0) {
        Write-Host "编译失败！" -ForegroundColor Red
        exit 1
    }
    Write-Host "编译成功: $BinaryName" -ForegroundColor Green
} finally {
    Pop-Location
}

# 重置环境变量
$env:GOOS = "windows"
$env:GOARCH = "amd64"

if ($BuildOnly) {
    Write-Host ""
    Write-Host "仅编译模式，二进制位于: $DashboardDir\$BinaryName" -ForegroundColor Green
    Write-Host "手动传到服务器后执行:" -ForegroundColor Cyan
    Write-Host "  scp $BinaryName user@server:/tmp/" -ForegroundColor Gray
    Write-Host "  scp nezha/Dockerfile.prebuilt user@server:/tmp/Dockerfile" -ForegroundColor Gray
    Write-Host "  ssh user@server 'cd /tmp && docker build -f Dockerfile --build-arg BINARY=$BinaryName -t $ImageName .'" -ForegroundColor Gray
    exit 0
}

# ===== 步骤 2：检查服务器地址 =====
if ([string]::IsNullOrWhiteSpace($Server)) {
    Write-Host ""
    Write-Host "[2/4] 未指定服务器地址 (-Server)，跳过上传" -ForegroundColor Yellow
    Write-Host "二进制位于: $DashboardDir\$BinaryName" -ForegroundColor Green
    Write-Host ""
    Write-Host "请手动上传到 Linux 服务器后执行:" -ForegroundColor Cyan
    Write-Host "  scp `"$DashboardDir\$BinaryName`" user@server:/tmp/" -ForegroundColor Gray
    Write-Host "  scp `"$DashboardDir\..\nezha\Dockerfile.prebuilt`" user@server:/tmp/Dockerfile" -ForegroundColor Gray
    Write-Host "  ssh user@server `"cd /tmp && docker build --build-arg BINARY=$BinaryName -t $ImageName .`"" -ForegroundColor Gray
    exit 0
}

# ===== 步骤 3：上传到服务器 =====
Write-Host "[2/4] 上传二进制和 Dockerfile 到 $Server ..." -ForegroundColor Yellow

$RemoteDir = "/tmp/nezha-build"
scp "$DashboardDir\$BinaryName" "${Server}:$RemoteDir/$BinaryName"
scp "$DashboardDir\Dockerfile.prebuilt" "${Server}:$RemoteDir/Dockerfile"

if ($LASTEXITCODE -ne 0) {
    Write-Host "上传失败！请检查 SSH 连接和路径" -ForegroundColor Red
    exit 1
}
Write-Host "上传成功" -ForegroundColor Green

# ===== 步骤 4：在服务器上构建镜像 =====
Write-Host "[3/4] 在服务器上构建 Docker 镜像..." -ForegroundColor Yellow

ssh $Server "cd $RemoteDir && docker build --build-arg BINARY=$BinaryName -t $ImageName ."

if ($LASTEXITCODE -ne 0) {
    Write-Host "Docker 构建失败！" -ForegroundColor Red
    exit 1
}
Write-Host "镜像构建成功: $ImageName" -ForegroundColor Green

# ===== 步骤 5：推送到 GHCR（可选）=====
Write-Host "[4/4] 完成" -ForegroundColor Green
Write-Host ""
Write-Host "镜像已在服务器上构建完成: $ImageName" -ForegroundColor Cyan
Write-Host ""
Write-Host "推送到 GHCR（GitHub Container Registry）:" -ForegroundColor Cyan
Write-Host "  # 服务器上先登录 GHCR（用 GitHub Personal Access Token，需勾选 write:packages 权限）" -ForegroundColor Gray
Write-Host "  ssh $Server 'echo \"ghp_xxx\" | docker login ghcr.io -u 用户名 --password-stdin'" -ForegroundColor Gray
Write-Host "  ssh $Server 'docker push $ImageName'" -ForegroundColor Gray
Write-Host ""
Write-Host "在服务器上运行:" -ForegroundColor Cyan
Write-Host "  ssh $Server 'docker run -d --name nezha-dashboard --restart always -p 8008:8008 -v /opt/nezha/data:/dashboard/data $ImageName'" -ForegroundColor Gray
