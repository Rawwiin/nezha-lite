#!/bin/sh

# Nezha 精简版 Agent 安装脚本（Linux/macOS/FreeBSD）
# 自包含：支持 GitHub 公共代理加速，通过 service 命令注册系统服务
#
# 前端生成的安装命令：
#   curl -L https://raw.githubusercontent.com/Rawwiin/nezha-lite/main/agent/install.sh -o nezha.sh && chmod +x nezha.sh && sudo ./nezha.sh
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
#   CN=true            - 强制使用 GitHub 代理

NZ_BASE_PATH="/opt/nezha"
NZ_AGENT_PATH="${NZ_BASE_PATH}/agent"

# GitHub 仓库（用于下载 Release 二进制）
GITHUB_REPO="Rawwiin/nezha-lite"
GITHUB_URL="github.com"

# GitHub 公共代理列表（中国地区自动使用，用于加速 GitHub 文件下载）
GITHUB_PROXIES="https://ghfast.top https://gh-proxy.com https://mirror.ghproxy.com"

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

err() {
    printf "${red}%s${plain}\n" "$*" >&2
}

success() {
    printf "${green}%s${plain}\n" "$*"
}

info() {
    printf "${yellow}%s${plain}\n" "$*"
}

sudo() {
    myEUID=$(id -ru)
    if [ "$myEUID" -ne 0 ]; then
        if command -v sudo > /dev/null 2>&1; then
            command sudo "$@"
        else
            err "错误: 您的系统未安装 sudo，因此无法进行该项操作。"
            exit 1
        fi
    else
        "$@"
    fi
}

deps_check() {
    deps="curl unzip grep"
    for dep in $deps; do
        if ! command -v "$dep" >/dev/null 2>&1; then
            err "未找到依赖 $dep，请先安装。"
            exit 1
        fi
    done
}

# 检测是否在中国，自动选择 GitHub 代理
geo_check() {
    api_list="https://blog.cloudflare.com/cdn-cgi/trace https://developers.cloudflare.com/cdn-cgi/trace"
    ua="Mozilla/5.0 (X11; Linux x86_64; rv:60.0) Gecko/20100101 Firefox/81.0"
    for url in $api_list; do
        text="$(curl -A "$ua" -m 10 -s "$url")"
        if echo "$text" | grep -qw 'CN'; then
            isCN=true
            break
        fi
    done
}

env_check() {
    mach=$(uname -m)
    case "$mach" in
        amd64|x86_64)
            os_arch="amd64"
            ;;
        i386|i686)
            os_arch="386"
            ;;
        aarch64|arm64)
            os_arch="arm64"
            ;;
        *arm*)
            os_arch="arm"
            ;;
        s390x)
            os_arch="s390x"
            ;;
        riscv64)
            os_arch="riscv64"
            ;;
        mips)
            os_arch="mips"
            ;;
        mipsel|mipsle)
            os_arch="mipsle"
            ;;
        loongarch64)
            os_arch="loong64"
            ;;
        *)
            err "未知架构：$mach"
            exit 1
            ;;
    esac

    system=$(uname)
    case "$system" in
        *Linux*)
            os="linux"
            ;;
        *Darwin*)
            os="darwin"
            ;;
        *FreeBSD*)
            os="freebsd"
            ;;
        *)
            err "不支持的系统：$system"
            exit 1
            ;;
    esac
}

# 尝试通过代理下载文件（依次尝试多个代理）
# 用法: github_download "输出文件" "原始GitHub URL"
github_download() {
    _output="$1"
    _url="$2"

    if [ -n "$CN" ]; then
        # 依次尝试公共代理
        for proxy in $GITHUB_PROXIES; do
            _proxy_url="${proxy}/${_url}"
            echo "尝试下载: ${_proxy_url}"
            if wget --timeout=60 -qO "$_output" "$_proxy_url" >/dev/null 2>&1; then
                return 0
            fi
            info "代理 ${proxy} 不可用，尝试下一个..."
        done
        err "所有公共代理均不可用，尝试直连 GitHub..."
    fi

    # 直连 GitHub
    echo "尝试下载: ${_url}"
    if wget --timeout=60 -qO "$_output" "$_url" >/dev/null 2>&1; then
        return 0
    fi
    err "下载失败: ${_url}"
    return 1
}

init() {
    deps_check
    env_check

    # 中国地区检测：自动选择 GitHub 代理
    if [ -z "$CN" ]; then
        geo_check
        if [ -n "$isCN" ]; then
            echo "根据 GeoIP 检测，当前 IP 可能在中国，将使用 GitHub 代理加速下载"
            CN=true
        fi
    fi
}

install() {
    echo "> 安装 Agent"

    # 下载二进制
    NZ_AGENT_URL="https://${GITHUB_URL}/${GITHUB_REPO}/releases/latest/download/nezha-agent_${os}_${os_arch}.zip"

    if ! github_download /tmp/nezha-agent_${os}_${os_arch}.zip "$NZ_AGENT_URL"; then
        err "下载 nezha-agent 失败，请检查网络连接"
        exit 1
    fi

    sudo mkdir -p $NZ_AGENT_PATH

    sudo unzip -qo /tmp/nezha-agent_${os}_${os_arch}.zip -d $NZ_AGENT_PATH &&
        sudo rm -rf /tmp/nezha-agent_${os}_${os_arch}.zip

    # 确定配置文件路径（已存在则用随机后缀避免覆盖）
    path="$NZ_AGENT_PATH/config.yml"
    if [ -f "$path" ]; then
        random=$(LC_ALL=C tr -dc a-z0-9 </dev/urandom | head -c 5)
        path=$(printf "%s" "$NZ_AGENT_PATH/config-$random.yml")
    fi

    # 校验必填参数
    if [ -z "$NZ_SERVER" ]; then
        err "NZ_SERVER 不能为空"
        exit 1
    fi

    if [ -z "$NZ_CLIENT_SECRET" ]; then
        err "NZ_CLIENT_SECRET 不能为空"
        exit 1
    fi

    # 构造环境变量（仅传递精简版支持的配置项）
    env="NZ_SERVER=$NZ_SERVER NZ_CLIENT_SECRET=$NZ_CLIENT_SECRET NZ_TLS=$NZ_TLS NZ_INSECURE_TLS=$NZ_INSECURE_TLS NZ_UUID=$NZ_UUID NZ_GPU=$NZ_GPU NZ_TEMPERATURE=$NZ_TEMPERATURE NZ_SKIP_CONNECTION_COUNT=$NZ_SKIP_CONNECTION_COUNT NZ_SKIP_PROCS_COUNT=$NZ_SKIP_PROCS_COUNT NZ_DISABLE_SEND_QUERY=$NZ_DISABLE_SEND_QUERY NZ_ALLOW_PROBE_INTERNAL=$NZ_ALLOW_PROBE_INTERNAL NZ_DEBUG=$NZ_DEBUG"

    # 先卸载旧服务（如果存在），再安装新服务
    sudo "${NZ_AGENT_PATH}"/nezha-agent service -c "$path" uninstall >/dev/null 2>&1
    _cmd="sudo env $env $NZ_AGENT_PATH/nezha-agent service -c $path install"
    if ! eval "$_cmd"; then
        err "安装 nezha-agent 服务失败"
        sudo "${NZ_AGENT_PATH}"/nezha-agent service -c "$path" uninstall >/dev/null 2>&1
        exit 1
    fi

    success "nezha-agent 安装成功"
    info "配置文件: $path"
    info "服务管理: ${NZ_AGENT_PATH}/nezha-agent service start|stop|restart|uninstall"
}

uninstall() {
    echo "> 卸载 Agent"

    # 遍历所有配置文件，逐个卸载对应的服务
    find "$NZ_AGENT_PATH" -type f -name "*config*.yml" 2>/dev/null | while read -r file; do
        sudo "$NZ_AGENT_PATH/nezha-agent" service -c "$file" uninstall
        sudo rm -f "$file"
    done

    # 清理二进制
    sudo rm -f "$NZ_AGENT_PATH/nezha-agent"

    success "nezha-agent 卸载完成"
}

show_usage() {
    echo "哪吒监控精简版 Agent 安装脚本使用方法:"
    echo "--------------------------------------------------------"
    echo "./nezha.sh            - 安装 Agent（需通过环境变量传参）"
    echo "./nezha.sh uninstall  - 卸载 Agent"
    echo "--------------------------------------------------------"
    echo "环境变量:"
    echo "  NZ_SERVER          - Dashboard 地址（必填）"
    echo "  NZ_CLIENT_SECRET   - 客户端密钥（必填）"
    echo "  NZ_TLS             - 启用 TLS（true/false）"
    echo "  NZ_INSECURE_TLS    - 跳过 TLS 验证（true/false）"
    echo "  NZ_UUID            - Agent UUID（可选）"
    echo "  NZ_GPU             - GPU 监控（true/false）"
    echo "  NZ_TEMPERATURE     - 温度监控（true/false）"
    echo "  NZ_DISABLE_SEND_QUERY - 禁止探测（true/false）"
    echo "  CN=true            - 强制使用 GitHub 代理"
    echo "--------------------------------------------------------"
}

if [ "$1" = "uninstall" ]; then
    uninstall
    exit
fi

if [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
    show_usage
    exit
fi

init
install
