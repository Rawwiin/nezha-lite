package main

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// isInternalIP 判断 IP 是否为内网/私有/回环/链路本地等不应被探测的保留地址。
// 覆盖范围：
//   - 回环: 127.0.0.0/8, ::1
//   - 私有: 10/8, 172.16/12, 192.168/16, fc00::/7
//   - 链路本地: 169.254/16（含云元数据 169.254.169.254）, fe80::/10
//   - 多播: 224.0.0.0/4, ff00::/8
//   - 未指定: 0.0.0.0, ::
//   - CGNAT: 100.64.0.0/10
//   - 基准测试: 198.18.0.0/15
func isInternalIP(ip net.IP) bool {
	if ip == nil {
		return true // nil IP 视为不安全
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// CGNAT 100.64.0.0/10 和基准测试网段 198.18.0.0/15 不被 Go 标准库 IsPrivate 覆盖
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
		if v4[0] == 198 && (v4[1] == 18 || v4[1] == 19) {
			return true
		}
	}
	return false
}

// validateResolvedIP 校验已解析的 IP 地址是否为内网/保留地址。
// 用于 TCPPing 和 ICMPPing：lookupIP 已返回 IP，直接检查即可，无需二次 DNS 解析。
func validateResolvedIP(ipAddr string) error {
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return fmt.Errorf("无效的 IP 地址: %s", ipAddr)
	}
	if isInternalIP(ip) {
		return fmt.Errorf("目标地址 %s 为内网/保留地址，已拒绝探测", ipAddr)
	}
	return nil
}

// validateProbeURL 校验 HTTP 探测目标 URL 的协议和主机地址。
// 仅允许 http/https 协议；解析主机名并检查解析出的所有 IP 是否为内网地址。
// 用于 HTTPGet：httpClient 内部自行做 DNS 解析，此处做独立校验以拦截内网目标。
func validateProbeURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL 解析失败: %v", err)
	}

	// 仅允许 http/https 协议，阻止 file://、gopher://、ftp:// 等
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("不允许的协议 %q，仅支持 http/https", scheme)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("URL 缺少主机名")
	}

	// 如果主机名本身是 IP，直接校验
	if ip := net.ParseIP(hostname); ip != nil {
		if isInternalIP(ip) {
			return fmt.Errorf("目标地址 %s 为内网/保留地址，已拒绝探测", hostname)
		}
		return nil
	}

	// 域名：解析后检查所有 IP，任一为内网即拒绝
	addrs, err := net.LookupIP(hostname)
	if err != nil {
		// DNS 解析失败时交由 httpClient 的正常错误处理，不在此阻断
		return nil
	}
	for _, addr := range addrs {
		if isInternalIP(addr) {
			return fmt.Errorf("域名 %s 解析到内网地址 %s，已拒绝探测", hostname, addr.String())
		}
	}
	return nil
}
