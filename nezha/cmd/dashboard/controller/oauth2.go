// Modified by Nezha Lite for simplified dashboard functionality.
// Original source: https://github.com/nezhahq/nezha
// Licensed under the Apache License, Version 2.0
//
// 精简版加固内容：
// 1. SSRF 防护：exchangeOpenId 使用受限 HTTP 客户端（CIDR 黑名单 + IP 钉死 + 禁止重定向）
// 2. 响应体大小限制：UserInfo 响应限制 1MB，防止恶意 IdP 导致 OOM
// 3. DashboardHost 强制：未配置 DashboardHost 时禁用 OAuth2，防止 Host 头伪造劫持

package controller

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/tidwall/gjson"
	"golang.org/x/oauth2"
	"gorm.io/gorm"

	"github.com/nezhahq/nezha/model"
	"github.com/nezhahq/nezha/pkg/utils"
	"github.com/nezhahq/nezha/service/singleton"
)

// oauth2UserInfoMaxBytes 限制 UserInfo 响应体最大 1MB，防止恶意 IdP 返回超大响应导致 OOM
const oauth2UserInfoMaxBytes = 1 << 20

// getRedirectURL 构造 OAuth2 回调 URL。
// GHSA-9rc6-8cjv-rcvx：回调 URL 会发送给 IdP 并接收 authorization code。
// 如果从原始 Host 头派生，攻击者可伪造 Host 将受害者的 code 重定向到自己的域名。
// 仅当 Host 是运维声明的 dashboard host 时才信任；否则钉死到 DashboardHost。
//
// 精简版加固：DashboardHost 为空时返回错误，强制运维配置此字段。
func getRedirectURL(c *gin.Context) (string, error) {
	// 精简版加固：强制要求配置 DashboardHost，防止未配置时 Host 头透传导致开放重定向
	if singleton.Conf == nil || singleton.Conf.DashboardHost == "" {
		return "", singleton.Localizer.ErrorT("dashboard_host must be configured to use OAuth2")
	}

	scheme := "http://"
	referer := c.Request.Referer()
	if forwardedProto := c.Request.Header.Get("X-Forwarded-Proto"); forwardedProto == "https" || strings.HasPrefix(referer, "https://") {
		scheme = "https://"
	}
	host := c.Request.Host
	if !singleton.IsReservedDashboardHost(host) {
		host = singleton.Conf.DashboardHost
	}
	return scheme + host + "/api/v1/oauth2/callback", nil
}

// oauth2redirect 处理 OAuth2 登录/绑定跳转请求
func oauth2redirect(c *gin.Context) (*model.Oauth2LoginResponse, error) {
	provider := c.Param("provider")
	if provider == "" {
		return nil, singleton.Localizer.ErrorT("provider is required")
	}

	rTypeInt, err := strconv.ParseUint(c.Query("type"), 10, 8)
	if err != nil {
		return nil, err
	}

	o2confRaw, has := singleton.Conf.Oauth2[provider]
	if !has {
		return nil, singleton.Localizer.ErrorT("provider not found")
	}
	redirectURL, err := getRedirectURL(c)
	if err != nil {
		return nil, err
	}
	o2conf := o2confRaw.Setup(redirectURL)

	randomString, err := utils.GenerateRandomString(32)
	if err != nil {
		return nil, err
	}
	state, stateKey := randomString[:16], randomString[16:]
	singleton.Cache.Set(fmt.Sprintf("%s%s", model.CacheKeyOauth2State, stateKey), &model.Oauth2State{
		Action:      model.Oauth2LoginType(rTypeInt),
		Provider:    provider,
		State:       state,
		RedirectURL: redirectURL,
	}, cache.DefaultExpiration)

	url := o2conf.AuthCodeURL(state, oauth2.AccessTypeOnline)
	writeOauth2StateCookie(c, stateKey)

	return &model.Oauth2LoginResponse{Redirect: url}, nil
}

// writeOauth2StateCookie 设置 OAuth2 state cookie（CSRF 防护）
// HttpOnly 始终启用——前端不读取此 cookie，仅 callback handler 使用
func writeOauth2StateCookie(c *gin.Context, stateKey string) {
	secure := c.Request.URL.Scheme == "https" || c.Request.TLS != nil
	c.SetCookie("nz-o2s", stateKey, 60*5, "", "", secure, true)
}

// unbindOauth2 解绑 OAuth2 账号
func unbindOauth2(c *gin.Context) (any, error) {
	provider := c.Param("provider")
	if provider == "" {
		return nil, singleton.Localizer.ErrorT("provider is required")
	}
	_, has := singleton.Conf.Oauth2[provider]
	if !has {
		return nil, singleton.Localizer.ErrorT("provider not found")
	}
	provider = strings.ToLower(provider)

	u := c.MustGet(model.CtxKeyAuthorizedUser).(*model.User)
	query := singleton.DB.Where("provider = ? AND user_id = ?", provider, u.ID)

	var bindCount int64
	if err := query.Model(&model.Oauth2Bind{}).Count(&bindCount).Error; err != nil {
		return nil, newGormError("%v", err)
	}

	// RejectPassword 用户不能解绑最后一个 OAuth2 绑定，否则会锁定账户
	if bindCount < 2 && u.RejectPassword {
		return nil, singleton.Localizer.ErrorT("operation not permitted")
	}

	if err := query.Delete(&model.Oauth2Bind{}).Error; err != nil {
		return nil, newGormError("%v", err)
	}

	return nil, nil
}

// oauth2callback 处理 OAuth2 回调
func oauth2callback(jwtConfig *jwt.GinJWTMiddleware) func(c *gin.Context) (any, error) {
	return func(c *gin.Context) (any, error) {
		callbackData := &model.Oauth2Callback{
			State: c.Query("state"),
			Code:  c.Query("code"),
		}

		state, err := verifyState(c, callbackData.State)
		if err != nil {
			return nil, err
		}

		o2confRaw, has := singleton.Conf.Oauth2[state.Provider]
		if !has {
			return nil, singleton.Localizer.ErrorT("provider not found")
		}

		realip := c.GetString(model.CtxKeyRealIPStr)
		if callbackData.Code == "" {
			model.BlockIP(singleton.DB, realip, model.WAFBlockReasonTypeBruteForceOauth2, model.BlockIDToken)
			return nil, singleton.Localizer.ErrorT("code is required")
		}

		openId, err := exchangeOpenId(c, o2confRaw, callbackData, state.RedirectURL)
		if err != nil {
			model.BlockIP(singleton.DB, realip, model.WAFBlockReasonTypeBruteForceOauth2, model.BlockIDToken)
			return nil, err
		}

		var bind model.Oauth2Bind
		state.Provider = strings.ToLower(state.Provider)
		switch state.Action {
		case model.RTypeBind:
			u, authorized := c.Get(model.CtxKeyAuthorizedUser)
			if !authorized {
				return nil, singleton.Localizer.ErrorT("unauthorized")
			}
			user := u.(*model.User)

			result := singleton.DB.Where("provider = ? AND open_id = ?", state.Provider, openId).Limit(1).Find(&bind)
			if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
				return nil, newGormError("%v", result.Error)
			}
			bind.UserID = user.ID
			bind.Provider = state.Provider
			bind.OpenID = openId

			if result.Error == gorm.ErrRecordNotFound {
				result = singleton.DB.Create(&bind)
			} else {
				result = singleton.DB.Save(&bind)
			}
			if result.Error != nil {
				return nil, newGormError("%v", result.Error)
			}
		default:
			if err := singleton.DB.Where("provider = ? AND open_id = ?", state.Provider, openId).First(&bind).Error; err != nil {
				return nil, singleton.Localizer.ErrorT("oauth2 user not binded yet")
			}
		}

		var bindUser model.User
		if err := singleton.DB.First(&bindUser, bind.UserID).Error; err != nil {
			return nil, newGormError("%v", err)
		}
		claims, err := issueJWTSession(c, &bindUser, singleton.Conf.JWTTimeout)
		if err != nil {
			return nil, err
		}
		tokenString, _, err := jwtConfig.TokenGenerator(claims)
		if err != nil {
			return nil, err
		}

		jwtConfig.SetCookie(c, tokenString)
		setCSRFCookie(c)
		c.Redirect(http.StatusFound, utils.IfOr(state.Action == model.RTypeBind, "/dashboard/profile?oauth2=true", "/dashboard/login?oauth2=true"))

		return nil, errNoop
	}
}

// exchangeOpenId 用 authorization code 换取 access token 并获取用户 OpenID。
//
// 精简版加固：
// 1. SSRF 防护：使用 NewRestrictedHTTPClient 对 UserInfoURL 做内网地址过滤（CIDR 黑名单 + IP 钉死 + 禁止重定向）
// 2. 响应体限制：io.LimitReader 限制 1MB，防止恶意 IdP 返回超大响应导致 OOM
func exchangeOpenId(c *gin.Context, o2confRaw *model.Oauth2Config,
	callbackData *model.Oauth2Callback, redirectURL string) (string, error) {
	o2conf := o2confRaw.Setup(redirectURL)

	otk, err := o2conf.Exchange(c, callbackData.Code)
	if err != nil {
		return "", err
	}

	// 精简版加固 1：对 UserInfoURL 做 SSRF 防护
	// 先用受限客户端校验目标地址（CIDR 黑名单 + IP 钉死），再构造带 OAuth2 token 的请求
	restrictedClient, err := utils.NewRestrictedHTTPClient(o2confRaw.UserInfoURL, false)
	if err != nil {
		return "", fmt.Errorf("userinfo URL not allowed: %w", err)
	}

	// 从受限客户端提取 Transport（已钉死 IP），用于构造带 OAuth2 token 的请求
	restrictedTransport := restrictedClient.Transport

	// 构造带 OAuth2 token 的 HTTP 客户端，复用受限 Transport
	oauth2Client := o2conf.Client(c, otk)
	if transport, ok := oauth2Client.Transport.(*oauth2.Transport); ok {
		// 将受限 Transport 作为底层 Base Transport，OAuth2 token 注入逻辑保持不变
		transport.Base = restrictedTransport
	}

	resp, err := oauth2Client.Get(o2confRaw.UserInfoURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 精简版加固 2：限制 UserInfo 响应体大小为 1MB，防止恶意 IdP 导致 OOM
	body, err := io.ReadAll(io.LimitReader(resp.Body, oauth2UserInfoMaxBytes))
	if err != nil {
		return "", err
	}

	return gjson.GetBytes(body, o2confRaw.UserIDPath).String(), nil
}

// verifyState 验证 OAuth2 state（CSRF 防护三重校验：Cookie + Cache + state 匹配）
func verifyState(c *gin.Context, state string) (*model.Oauth2State, error) {
	stateKey, err := c.Cookie("nz-o2s")
	if err != nil {
		return nil, singleton.Localizer.ErrorT("invalid state key")
	}

	cacheKey := fmt.Sprintf("%s%s", model.CacheKeyOauth2State, stateKey)
	istate, ok := singleton.Cache.Get(cacheKey)
	if !ok {
		return nil, singleton.Localizer.ErrorT("invalid state key")
	}

	oauth2State, ok := istate.(*model.Oauth2State)
	if !ok || oauth2State.State != state {
		return nil, singleton.Localizer.ErrorT("invalid state key")
	}

	return oauth2State, nil
}