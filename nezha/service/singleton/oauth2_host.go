// Modified by Nezha Lite for simplified dashboard functionality.
// Original source: https://github.com/nezhahq/nezha
// Licensed under the Apache License, Version 2.0

package singleton

import (
	"net"
	"net/netip"
	"strings"
)

// IsReservedDashboardHost 判断给定的域名是否是 Dashboard 保留 host。
// 用于 OAuth2 回调 URL 构造时验证请求 Host 头是否可信，
// 防止攻击者伪造 Host 头劫持 OAuth2 authorization code。
//
// 保留 host 列表：InstallHost、DashboardHost、ListenHost + 运维配置的 ReservedHosts
func IsReservedDashboardHost(domain string) bool {
	if Conf == nil {
		return false
	}

	target := splitDashboardHostname(domain)
	if target == "" {
		return false
	}

	hosts := []string{Conf.InstallHost, Conf.DashboardHost, Conf.ListenHost}
	hosts = append(hosts, strings.Split(Conf.ReservedHosts, ",")...)
	for _, host := range hosts {
		if reserved := splitDashboardHostname(host); reserved != "" && reserved == target {
			return true
		}
	}
	return false
}

// splitDashboardHostname 归一化为小写 hostname，对 bracketed IPv6（[::1]、
// [::1]:8008）与裸 host:port 一视同仁，避免候选与保留解析形态不一致导致漏拦。
func splitDashboardHostname(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil && h != "" {
		host = h
	} else {
		host = strings.Trim(host, "[]")
	}
	host = strings.TrimSuffix(host, ".")
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.String()
	}
	return host
}