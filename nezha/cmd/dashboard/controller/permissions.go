// Modified by Nezha Lite for simplified dashboard functionality.
// Original source: https://github.com/nezhahq/nezha
// Licensed under the Apache License, Version 2.0

package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/nezhahq/nezha/model"
	"github.com/nezhahq/nezha/service/singleton"
)

func callerIsAdmin(c *gin.Context) bool {
	auth, ok := c.Get(model.CtxKeyAuthorizedUser)
	if !ok {
		return false
	}
	user, ok := auth.(*model.User)
	if !ok || user == nil {
		return false
	}
	return user.Role.IsAdmin()
}

// patAllowsServer reports whether the caller's PAT (if any) is allowed to
// touch serverID. JWT callers (no PAT in context) always pass. Used as an
// extra guard before the admin / owner short-circuits so a PAT scoped to
// a server_ids whitelist cannot widen reach via the caller's admin role.
func patAllowsServer(c *gin.Context, serverID uint64) bool {
	v, ok := c.Get(model.CtxKeyAPIToken)
	if !ok {
		return true
	}
	tok, _ := v.(model.APITokenAccessor)
	if tok == nil {
		return true
	}
	return tok.CanAccessServer(serverID)
}

// patHasServerWhitelist reports whether the caller is authenticated by a PAT
// that carries a non-empty server_ids whitelist. JWT callers and unscoped PATs
// have no whitelist to escape and pass through.
func patHasServerWhitelist(c *gin.Context) bool {
	v, ok := c.Get(model.CtxKeyAPIToken)
	if !ok {
		return false
	}
	wl, ok := v.(model.APITokenWhitelistView)
	if !ok || wl == nil {
		return false
	}
	return len(wl.ServerIDs()) > 0
}

// patAccessorFromContext returns the request's PAT viewed as an
// APITokenAccessor, or nil for JWT requests.
func patAccessorFromContext(c *gin.Context) model.APITokenAccessor {
	v, ok := c.Get(model.CtxKeyAPIToken)
	if !ok {
		return nil
	}
	tok, _ := v.(model.APITokenAccessor)
	if tok == nil {
		return nil
	}
	return tok
}

// denyListOwnedByCaller verifies every id in denyList refers to a server
// owned by ownerUID. Under *CoverAll the deny-list expresses exclusion, not
// access, so it must not point at someone else's servers.
func denyListOwnedByCaller(ownerUID uint64, denyList map[uint64]bool) bool {
	ownerIsAdmin := model.OwnerIsAdminLookup != nil && model.OwnerIsAdminLookup(ownerUID)
	for id := range denyList {
		s, found := singleton.ServerShared.Get(id)
		if !found || s == nil {
			return false
		}
		if ownerIsAdmin {
			continue
		}
		if s.GetUserID() != ownerUID {
			return false
		}
	}
	return true
}

// denyListCoversAllOwnerServersOutsidePATWhitelist reports whether every
// server visible to the owner that is NOT in the caller PAT's server_ids
// whitelist also appears in denyList.
func denyListCoversAllOwnerServersOutsidePATWhitelist(c *gin.Context, ownerUID uint64, denyList map[uint64]bool) bool {
	tok := patAccessorFromContext(c)
	if tok == nil {
		return true
	}
	denyIDs := make([]uint64, 0, len(denyList))
	for id, mark := range denyList {
		if mark {
			denyIDs = append(denyIDs, id)
		}
	}
	return model.DenyListSafeForLimitedPAT(tok, ownerUID, denyIDs)
}

// coverMode 抽象「cover 字段在 dispatch 时如何解读 servers 字段」。
type coverMode uint8

const (
	coverModePinnedByCaller coverMode = iota
	coverModeAllMinusDeny
	coverModeAllowList
)

// assertPATCoverFanoutWithinWhitelist 是 cover-all / cover-ignore-all 两类
// 「按 owner 全量 fan-out」资源的 PAT 收口。
func assertPATCoverFanoutWithinWhitelist(c *gin.Context, ownerUID uint64, mode coverMode, servers []uint64) error {
	if !patHasServerWhitelist(c) {
		return nil
	}
	switch mode {
	case coverModePinnedByCaller:
		return nil
	case coverModeAllMinusDeny:
		denySet := make(map[uint64]bool, len(servers))
		for _, id := range servers {
			denySet[id] = true
		}
		if !denyListCoversAllOwnerServersOutsidePATWhitelist(c, ownerUID, denySet) {
			return singleton.Localizer.ErrorT("permission denied")
		}
		return nil
	case coverModeAllowList:
		tok := patAccessorFromContext(c)
		if tok == nil {
			return nil
		}
		for _, id := range servers {
			if !tok.CanAccessServer(id) {
				return singleton.Localizer.ErrorT("permission denied")
			}
		}
		return nil
	default:
		return singleton.Localizer.ErrorT("permission denied")
	}
}

const coverModeUnknown coverMode = 255

// patGroupMembershipAccessAllowed returns false when the caller's PAT
// carries a server_ids whitelist that does not cover every current member
// of groupID.
func patGroupMembershipAccessAllowed(c *gin.Context, groupID uint64) bool {
	tok := patAccessorFromContext(c)
	if tok == nil || !patHasServerWhitelist(c) {
		return true
	}
	var members []model.ServerGroupServer
	if err := singleton.DB.Where("server_group_id = ?", groupID).Find(&members).Error; err != nil {
		return false
	}
	for _, m := range members {
		if !tok.CanAccessServer(m.ServerId) {
			return false
		}
	}
	return true
}

func userCanViewServer(c *gin.Context, server *model.Server) bool {
	if server == nil {
		return false
	}
	if !patAllowsServer(c, server.GetID()) {
		return false
	}
	if callerIsAdmin(c) {
		return true
	}
	if _, isMember := c.Get(model.CtxKeyAuthorizedUser); isMember {
		if server.HasPermission(c) {
			return true
		}
		return !server.HideForGuest
	}
	return !server.HideForGuest
}

func userCanViewService(c *gin.Context, service *model.Service) bool {
	if service == nil {
		return false
	}
	if !patAllowsServer(c, service.GetID()) {
		return false
	}
	if callerIsAdmin(c) {
		return true
	}
	if _, isMember := c.Get(model.CtxKeyAuthorizedUser); isMember {
		if service.HasPermission(c) {
			return true
		}
		return !service.HideForGuest
	}
	return !service.HideForGuest
}

// assertOwnsNotificationGroup 校验通知组归属权限
func assertOwnsNotificationGroup(c *gin.Context, groupID uint64) error {
	if groupID == 0 {
		return nil
	}

	var ng model.NotificationGroup
	if err := singleton.DB.First(&ng, groupID).Error; err != nil {
		return singleton.Localizer.ErrorT("notification group id %d does not exist", groupID)
	}
	if !ng.HasPermission(c) {
		return singleton.Localizer.ErrorT("permission denied")
	}
	return nil
}
