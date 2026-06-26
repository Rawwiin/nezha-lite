// Modified by Nezha Lite for simplified dashboard functionality.
// Original source: https://github.com/nezhahq/nezha
// Licensed under the Apache License, Version 2.0

package model

// Oauth2Bind OAuth2 账号绑定记录
type Oauth2Bind struct {
	Common

	UserID   uint64 `gorm:"uniqueIndex:u_p_o" json:"user_id,omitempty"`
	Provider string `gorm:"uniqueIndex:u_p_o" json:"provider,omitempty"`
	OpenID   string `gorm:"uniqueIndex:u_p_o" json:"open_id,omitempty"`
}

// Oauth2LoginType OAuth2 操作类型
type Oauth2LoginType uint8

const (
	_ Oauth2LoginType = iota
	RTypeLogin // 登录
	RTypeBind  // 绑定
)

// Oauth2State OAuth2 state 缓存结构（CSRF 防护）
type Oauth2State struct {
	Action      Oauth2LoginType
	Provider    string
	State       string
	RedirectURL string
}