#!/bin/sh

# Nezha 精简版 Dashboard 安装脚本
# 自包含：不依赖外部脚本仓库，配置和服务文件均内联生成
# 支持 Docker 和独立安装两种方式，支持中国地区 GitHub 代理加速

NZ_BASE_PATH="/opt/nezha"
NZ_DASHBOARD_PATH="${NZ_BASE_PATH}/dashboard"
NZ_DASHBOARD_SERVICE="/etc/systemd/system/nezha-dashboard.service"
NZ_DASHBOARD_SERVICERC="/etc/init.d/nezha-dashboard"

# ===== 可配置变量 =====
# GitHub 仓库（用于下载 Release 二进制）
GITHUB_REPO="Rawwiin/nezha-lite"
GITHUB_URL="github.com"

# Docker 镜像（GHCR 镜像名，格式：ghcr.io/<用户名>/nezha-lite）
Docker_IMG="ghcr.io/rawwiin/nezha-lite"

# GitHub 公共代理列表（中国地区自动使用，用于加速 GitHub 文件下载）
# 按优先级排列，脚本会依次尝试直到成功
GITHUB_PROXIES="https://ghfast.top https://gh-proxy.com https://mirror.ghproxy.com"

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

err() {
    printf "${red}%s${plain}\n" "$*" >&2
}

warn() {
    printf "${red}%s${plain}\n" "$*"
}

success() {
    printf "${green}%s${plain}\n" "$*"
}

info() {
    printf "${yellow}%s${plain}\n" "$*"
}

println() {
    printf "$*\n"
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
    deps="curl wget unzip grep"
    for dep in $deps; do
        if ! command -v "$dep" >/dev/null 2>&1; then
            err "未找到依赖 $dep，请先安装。"
            exit 1
        fi
    done
}

check_init() {
    init=$(readlink /sbin/init)
    case "$init" in
        *systemd*)
            INIT=systemd
            ;;
        *openrc-init*|*busybox*)
            INIT=openrc
            ;;
        *)
            err "不支持的 init 系统: $init"
            exit 1
            ;;
    esac
}

env_check() {
    uname=$(uname -m)
    case "$uname" in
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
        *)
            err "未知架构：$uname"
            exit 1
            ;;
    esac
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

# 检测已安装的 Docker 环境
installation_check() {
    IS_DOCKER_NEZHA=""
    DOCKER_COMPOSE_COMMAND=""

    if docker compose version >/dev/null 2>&1; then
        DOCKER_COMPOSE_COMMAND="docker compose"
        if sudo $DOCKER_COMPOSE_COMMAND ls 2>/dev/null | grep -qw "$NZ_DASHBOARD_PATH/docker-compose.yaml" >/dev/null 2>&1; then
            IS_DOCKER_NEZHA=1
            FRESH_INSTALL=0
            return
        fi
    elif command -v docker-compose >/dev/null 2>&1; then
        DOCKER_COMPOSE_COMMAND="docker-compose"
        if sudo $DOCKER_COMPOSE_COMMAND -f "$NZ_DASHBOARD_PATH/docker-compose.yaml" config >/dev/null 2>&1; then
            IS_DOCKER_NEZHA=1
            FRESH_INSTALL=0
            return
        fi
    fi

    if [ -f "$NZ_DASHBOARD_PATH/nezha-dashboard" ]; then
        IS_DOCKER_NEZHA=0
        FRESH_INSTALL=0
    else
        FRESH_INSTALL=1
    fi
}

# 选择安装方式（Docker 或独立安装）
select_version() {
    if [ -z "$IS_DOCKER_NEZHA" ]; then
        info "请自行选择您的安装方式："
        info "1. Docker"
        info "2. 独立安装"
        while true; do
            printf "请输入选择 [1-2]："
            read -r option
            case "${option}" in
                1)
                    IS_DOCKER_NEZHA=1
                    break
                    ;;
                2)
                    IS_DOCKER_NEZHA=0
                    break
                    ;;
                *)
                    err "请输入正确的选择 [1-2]"
                    ;;
            esac
        done
    fi
}

init() {
    deps_check
    check_init
    env_check
    installation_check

    # 中国地区检测：自动选择 GitHub 代理
    if [ -z "$CN" ]; then
        geo_check
        if [ -n "$isCN" ]; then
            echo "根据 GeoIP 检测，当前 IP 可能在中国"
            printf "是否使用 GitHub 代理加速下载？[Y/n] (自定义代理输入 3)："
            read -r input
            case $input in
            [nN][oO] | [nN])
                echo "不使用代理"
                ;;
            [3])
                echo "使用自定义代理"
                printf "请输入 GitHub 代理地址 (例如: https://ghfast.top)，留空为不使用："
                read -r input
                if [ -n "$input" ]; then
                    CUSTOM_PROXY=$input
                fi
                ;;
            *)
                echo "使用公共 GitHub 代理"
                CN=true
                ;;
            esac
        fi
    fi
}

# 构建带代理的下载 URL
# 用法: build_github_url "https://github.com/owner/repo/releases/download/v1.0/file.zip"
build_github_url() {
    _url="$1"
    if [ -n "$CUSTOM_PROXY" ]; then
        echo "${CUSTOM_PROXY}/${_url}"
    elif [ -n "$CN" ]; then
        echo "${GITHUB_PROXIES%% *}/${_url}"
    else
        echo "$_url"
    fi
}

# 尝试通过代理下载文件（依次尝试多个代理）
# 用法: github_download "输出文件" "原始GitHub URL"
github_download() {
    _output="$1"
    _url="$2"

    if [ -n "$CUSTOM_PROXY" ]; then
        # 使用自定义代理
        _proxy_url="${CUSTOM_PROXY}/${_url}"
        echo "尝试下载: ${_proxy_url}"
        if sudo wget -t 2 -T 60 -qO "$_output" "$_proxy_url" >/dev/null 2>&1; then
            return 0
        fi
        err "自定义代理下载失败: ${CUSTOM_PROXY}"
        return 1
    elif [ -n "$CN" ]; then
        # 依次尝试公共代理
        for proxy in $GITHUB_PROXIES; do
            _proxy_url="${proxy}/${_url}"
            echo "尝试下载: ${_proxy_url}"
            if sudo wget -t 2 -T 60 -qO "$_output" "$_proxy_url" >/dev/null 2>&1; then
                return 0
            fi
            warn "代理 ${proxy} 不可用，尝试下一个..."
        done
        err "所有公共代理均不可用，尝试直连 GitHub..."
        # 代理全部失败后回退到直连
        echo "尝试下载: ${_url}"
        if sudo wget -t 2 -T 60 -qO "$_output" "$_url" >/dev/null 2>&1; then
            return 0
        fi
        err "直连 GitHub 也失败，请检查网络或手动下载"
        return 1
    else
        # 直连 GitHub
        echo "尝试下载: ${_url}"
        if sudo wget -t 2 -T 60 -qO "$_output" "$_url" >/dev/null 2>&1; then
            return 0
        fi
        err "下载失败: ${_url}"
        return 1
    fi
}

before_show_menu() {
    echo && info "* 按回车返回主菜单 *" && read temp
    show_menu
}

# 生成 config.yaml 配置文件（内联，不依赖外部下载）
# 预填 custom_code / custom_code_dashboard 精简版品牌标识，
# 让用户安装后即可区分精简版与原版 nezha。
# 用户可在管理端「设置」页面修改或清空这些字段。
generate_config() {
    cat <<EOF
site_name: "${nz_site_title}"
language: "${nz_lang}"
location: "Asia/Shanghai"
listen_port: ${nz_port}
jwt_secret_key: ""
debug: false
enable_mcp: false
tls: ${nz_tls}

# 精简版品牌标识（用户端 nezha-dash-v2）
# 在页面右下角显示"Nezha Lite"水印，可自行修改或删除
custom_code: |
  <style>
  .nezha-lite-badge {
    position: fixed;
    bottom: 8px;
    right: 12px;
    font-size: 11px;
    font-weight: 500;
    letter-spacing: 0.5px;
    opacity: 0.35;
    pointer-events: none;
    z-index: 9999;
    user-select: none;
    color: currentColor;
  }
  .nezha-lite-badge:hover { opacity: 0.6; }
  </style>
  <div class="nezha-lite-badge">Nezha Lite</div>

# 精简版品牌标识（管理端 admin-frontend）
# 在页面右下角显示"Nezha Lite"水印，可自行修改或删除
custom_code_dashboard: |
  <style>
  .nezha-lite-badge {
    position: fixed;
    bottom: 8px;
    right: 12px;
    font-size: 11px;
    font-weight: 500;
    letter-spacing: 0.5px;
    opacity: 0.35;
    pointer-events: none;
    z-index: 9999;
    user-select: none;
    color: currentColor;
  }
  .nezha-lite-badge:hover { opacity: 0.6; }
  </style>
  <div class="nezha-lite-badge">Nezha Lite</div>

tsdb:
  data_path: "./data/tsdb"
  retention_days: 30
EOF
}

# 生成 docker-compose.yaml（内联，不依赖外部下载）
generate_docker_compose() {
    cat <<EOF
version: "3"
services:
  nezha-dashboard:
    image: ${Docker_IMG}
    container_name: nezha-dashboard
    restart: always
    volumes:
      - ${NZ_DASHBOARD_PATH}/data:/app/data
    ports:
      - "${nz_port}:${nz_port}"
EOF
}

# 生成 systemd 服务文件（内联，不依赖外部下载）
generate_systemd_service() {
    cat <<EOF
[Unit]
Description=Nezha Dashboard
After=network.target

[Service]
Type=simple
WorkingDirectory=${NZ_DASHBOARD_PATH}
ExecStart=${NZ_DASHBOARD_PATH}/nezha-dashboard
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
}

# 生成 openrc 服务文件（内联）
generate_openrc_service() {
    cat <<EOF
#!/sbin/openrc-run

description="Nezha Dashboard"
command="${NZ_DASHBOARD_PATH}/nezha-dashboard"
command_background=true
directory="${NZ_DASHBOARD_PATH}"
pidfile="/run/nezha-dashboard.pid"

depend() {
    need net
    after firewall
}
EOF
}

install() {
    echo "> 安装"

    if [ ! "$FRESH_INSTALL" = 0 ]; then
        sudo mkdir -p $NZ_DASHBOARD_PATH
    else
        echo "您可能已经安装过面板端，重复安装会覆盖数据，请注意备份。"
        printf "是否退出安装? [Y/n] "
        read -r input
        case $input in
        [yY][eE][sS] | [yY])
            echo "退出安装"
            exit 0
            ;;
        [nN][oO] | [nN])
            echo "继续安装"
            ;;
        *)
            echo "退出安装"
            exit 0
            ;;
        esac
    fi

    modify_config 0

    if [ $# = 0 ]; then
        before_show_menu
    fi
}

modify_config() {
    echo "> 修改配置"

    # Docker 模式检查 docker-compose 命令
    if [ "$IS_DOCKER_NEZHA" = 1 ] && [ -z "$DOCKER_COMPOSE_COMMAND" ]; then
        err "未检测到 docker compose 或 docker-compose 命令，请先安装 Docker Compose"
        err "安装文档: https://docs.docker.com/compose/install/"
        before_show_menu
        return 1
    fi

    printf "请输入站点标题: "
    read -r nz_site_title
    printf "请输入暴露端口: (默认 8008) "
    read -r nz_port
    printf "请指定安装命令中预设的 nezha-agent 连接地址（例如 example.com:443）: "
    read -r nz_hostport
    printf "是否希望通过 TLS 连接 Agent？[y/N] "
    read -r input
    case $input in
    [yY][eE][sS] | [yY])
        nz_tls=true
        ;;
    *)
        nz_tls=false
        ;;
    esac
    println "请指定后台语言"
    println "1. 中文（简体）"
    println "2. 中文（台灣）"
    println "3. English"
    while true; do
        printf "请输入选项 [1-3]: "
        read -r option
        case "${option}" in
            1)
                nz_lang=zh_CN
                break
                ;;
            2)
                nz_lang=zh_TW
                break
                ;;
            3)
                nz_lang=en_US
                break
                ;;
            *)
                err "请输入正确的选项 [1-3]"
                ;;
        esac
    done

    if [ -z "$nz_lang" ] || [ -z "$nz_site_title" ] || [ -z "$nz_hostport" ]; then
        err "所有选项都不能为空"
        before_show_menu
        return 1
    fi

    if [ -z "$nz_port" ]; then
        nz_port=8008
    fi

    # 生成配置文件（内联生成，不依赖外部下载）
    sudo mkdir -p $NZ_DASHBOARD_PATH/data
    generate_config | sudo tee ${NZ_DASHBOARD_PATH}/data/config.yaml > /dev/null

    if [ "$IS_DOCKER_NEZHA" = 1 ]; then
        # Docker 模式：生成 docker-compose.yaml
        generate_docker_compose | sudo tee ${NZ_DASHBOARD_PATH}/docker-compose.yaml > /dev/null
        success "已生成 docker-compose.yaml，镜像地址: ${Docker_IMG}"
    else
        # 独立安装模式：生成服务文件
        if [ "$INIT" = "systemd" ]; then
            generate_systemd_service | sudo tee $NZ_DASHBOARD_SERVICE > /dev/null
        elif [ "$INIT" = "openrc" ]; then
            generate_openrc_service | sudo tee $NZ_DASHBOARD_SERVICERC > /dev/null
            sudo chmod +x $NZ_DASHBOARD_SERVICERC
        fi
    fi

    success "Dashboard 配置修改成功，请稍等 Dashboard 重启生效"

    restart_and_update 0

    if [ $# = 0 ]; then
        before_show_menu
    fi
}

restart_and_update() {
    echo "> 重启并更新"

    _ok=0
    if [ "$IS_DOCKER_NEZHA" = 1 ]; then
        if eval "restart_and_update_docker"; then
            _ok=1
        fi
    else
        if eval "restart_and_update_standalone"; then
            _ok=1
        fi
    fi

    if [ "$_ok" = 1 ]; then
        success "哪吒监控 重启成功"
        info "默认地址：域名:站点访问端口"
    else
        err "重启失败，可能是因为启动时间超过了两秒，请稍后查看日志信息"
    fi

    if [ $# = 0 ]; then
        before_show_menu
    fi
}

restart_and_update_docker() {
    sudo $DOCKER_COMPOSE_COMMAND -f ${NZ_DASHBOARD_PATH}/docker-compose.yaml pull
    sudo $DOCKER_COMPOSE_COMMAND -f ${NZ_DASHBOARD_PATH}/docker-compose.yaml down
    sleep 2
    sudo $DOCKER_COMPOSE_COMMAND -f ${NZ_DASHBOARD_PATH}/docker-compose.yaml up -d
}

restart_and_update_standalone() {
    # 获取最新版本号（通过 GitHub API，中国地区自动使用代理）
    _api_url="https://api.${GITHUB_URL}/repos/${GITHUB_REPO}/releases/latest"
    _version=$(curl -m 10 -sL "$_api_url" | grep "tag_name" | head -n 1 | awk -F ":" '{print $2}' | sed 's/\"//g;s/,//g;s/ //g')

    # 备用：通过 jsdelivr CDN 获取版本号
    if [ -z "$_version" ]; then
        _version=$(curl -m 10 -sL "https://fastly.jsdelivr.net/gh/${GITHUB_REPO}/" | grep "option\.value" | awk -F "'" '{print $2}' | sed 's/nezhahq\/nezha@/v/g')
    fi
    if [ -z "$_version" ]; then
        _version=$(curl -m 10 -sL "https://gcore.jsdelivr.net/gh/${GITHUB_REPO}/" | grep "option\.value" | awk -F "'" '{print $2}' | sed 's/nezhahq\/nezha@/v/g')
    fi

    if [ -z "$_version" ]; then
        err "获取 Dashboard 版本号失败，请检查本机能否连接 https://api.${GITHUB_URL}/repos/${GITHUB_REPO}/releases/latest"
        return 1
    else
        echo "当前最新版本为：${_version}"
    fi

    if [ "$INIT" = "systemd" ]; then
        sudo systemctl daemon-reload
        sudo systemctl stop nezha-dashboard
    elif [ "$INIT" = "openrc" ]; then
        sudo rc-service nezha-dashboard stop
    fi

    # 下载二进制（中国地区自动使用 GitHub 代理加速）
    _github_url="https://${GITHUB_URL}/${GITHUB_REPO}/releases/download/${_version}/dashboard-linux-${os_arch}.zip"

    if github_download $NZ_DASHBOARD_PATH/app.zip "$_github_url"; then
        sudo unzip -qq -o $NZ_DASHBOARD_PATH/app.zip -d $NZ_DASHBOARD_PATH
        sudo mv -f $NZ_DASHBOARD_PATH/dashboard-linux-$os_arch $NZ_DASHBOARD_PATH/nezha-dashboard
        sudo rm -f $NZ_DASHBOARD_PATH/app.zip
        sudo chmod +x $NZ_DASHBOARD_PATH/nezha-dashboard
    else
        err "二进制下载失败，请检查网络或手动下载: ${_github_url}"
        return 1
    fi

    sleep 2

    if [ "$INIT" = "systemd" ]; then
        sudo systemctl enable nezha-dashboard
        sudo systemctl restart nezha-dashboard
    elif [ "$INIT" = "openrc" ]; then
        sudo rc-update add nezha-dashboard
        sudo rc-service nezha-dashboard restart
    fi
}

show_log() {
    echo "> 获取日志"

    if [ "$IS_DOCKER_NEZHA" = 1 ]; then
        sudo $DOCKER_COMPOSE_COMMAND -f ${NZ_DASHBOARD_PATH}/docker-compose.yaml logs -f
    else
        if [ "$INIT" = "systemd" ]; then
            sudo journalctl -xf -u nezha-dashboard.service
        elif [ "$INIT" = "openrc" ]; then
            sudo tail -n 10 /var/log/nezha-dashboard.err
        fi
    fi

    if [ $# = 0 ]; then
        before_show_menu
    fi
}

uninstall() {
    echo "> 卸载"

    warn "警告：卸载前请备份您的文件。"
    printf "继续？[y/N] "
    read -r input
    case $input in
    [yY][eE][sS] | [yY])
        info "卸载中…"
        ;;
    *)
        return
        ;;
    esac

    if [ "$IS_DOCKER_NEZHA" = 1 ]; then
        # Docker 模式卸载
        if [ -n "$DOCKER_COMPOSE_COMMAND" ]; then
            sudo $DOCKER_COMPOSE_COMMAND -f ${NZ_DASHBOARD_PATH}/docker-compose.yaml down 2>/dev/null
        fi
        sudo rm -rf $NZ_DASHBOARD_PATH
        # 尝试清理 Docker 镜像
        sudo docker rmi -f "$Docker_IMG" >/dev/null 2>&1
    else
        # 独立安装模式卸载
        sudo rm -rf $NZ_DASHBOARD_PATH
        if [ "$INIT" = "systemd" ]; then
            sudo systemctl disable nezha-dashboard
            sudo systemctl stop nezha-dashboard
            sudo rm -f $NZ_DASHBOARD_SERVICE
            sudo systemctl daemon-reload
        elif [ "$INIT" = "openrc" ]; then
            sudo rc-update del nezha-dashboard
            sudo rc-service nezha-dashboard stop
            sudo rm -f $NZ_DASHBOARD_SERVICERC
        fi
    fi

    success "卸载成功"

    if [ $# = 0 ]; then
        before_show_menu
    fi
}

show_usage() {
    echo "哪吒监控精简版 管理脚本使用方法:"
    echo "--------------------------------------------------------"
    echo "./nezha.sh                    - 显示管理菜单"
    echo "./nezha.sh install            - 安装面板端"
    echo "./nezha.sh modify_config      - 修改面板配置"
    echo "./nezha.sh restart_and_update - 重启并更新面板"
    echo "./nezha.sh show_log           - 查看面板日志"
    echo "./nezha.sh uninstall          - 卸载管理面板"
    echo "--------------------------------------------------------"
    echo "环境变量:"
    echo "  CN=true                     - 强制使用中国代理"
    echo "  Docker_IMG=your/image       - 自定义 Docker 镜像"
    echo "--------------------------------------------------------"
}

show_menu() {
    println "${green}哪吒监控精简版管理脚本${plain}"
    echo "--- https://github.com/Rawwiin/nezha-lite ---"
    println "${green}1.${plain}  安装面板端"
    println "${green}2.${plain}  修改面板配置"
    println "${green}3.${plain}  重启并更新面板"
    println "${green}4.${plain}  查看面板日志"
    println "${green}5.${plain}  卸载管理面板"
    echo "--------------------------------------------------------"
    println "${green}0.${plain}  退出脚本"

    echo && printf "请输入选择 [0-5]: " && read -r num
    case "${num}" in
        0)
            exit 0
            ;;
        1)
            install
            ;;
        2)
            modify_config
            ;;
        3)
            restart_and_update
            ;;
        4)
            show_log
            ;;
        5)
            uninstall
            ;;
        *)
            err "请输入正确的数字 [0-5]"
            ;;
    esac
}

init

if [ $# -gt 0 ]; then
    case $1 in
        "install")
            select_version
            install 0
            ;;
        "modify_config")
            modify_config 0
            ;;
        "restart_and_update")
            restart_and_update 0
            ;;
        "show_log")
            show_log 0
            ;;
        "uninstall")
            uninstall 0
            ;;
        *)
            show_usage
            ;;
    esac
else
    select_version
    show_menu
fi
