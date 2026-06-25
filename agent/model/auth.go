package model

import (
	"context"
	"errors"
)

// AuthHandler 将 Agent 身份凭据（ClientSecret + UUID）附加到每次出站 gRPC 调用。
// Credentials 闭包返回一致的 (secret, uuid) 对，供 gRPC dial 在建立连接时读取。
type AuthHandler struct {
	Credentials func() (secret, uuid string)
	// RequireTLS 报告 Agent 传输是否必须加密：TLS:false 的内网明文部署继续工作，
	// TLS:true 的 Agent 拒绝在明文信道上泄露凭据。nil 表示不要求 TLS（兼容旧行为）。
	RequireTLS func() bool
}

// ErrAuthCredentialsNotConfigured 在 AuthHandler 未配置 Credentials 闭包时返回。
// 返回 error 而非 panic 可保持 gRPC 客户端循环存活，让 supervisor 记录日志并重试——
// nil 解引用会崩溃 Agent 进程并导致无人值守主机反复抖动。
var ErrAuthCredentialsNotConfigured = errors.New("agent: AuthHandler.Credentials closure is not configured")

func (a *AuthHandler) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	if a == nil || a.Credentials == nil {
		return nil, ErrAuthCredentialsNotConfigured
	}
	secret, uuid := a.Credentials()
	return map[string]string{
		"client-secret": secret,
		"client-uuid":   uuid,
		"client_secret": secret,
		"client_uuid":   uuid,
	}, nil
}

func (a *AuthHandler) RequireTransportSecurity() bool {
	if a == nil || a.RequireTLS == nil {
		return false
	}
	return a.RequireTLS()
}
