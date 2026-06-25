package singleton

import (
	"cmp"
	"fmt"
	"log"
	"slices"

	"github.com/libdns/cloudflare"
	"github.com/libdns/he"
	tencentcloud "github.com/nezhahq/libdns-tencentcloud"

	"github.com/nezhahq/nezha/model"
	ddns2 "github.com/nezhahq/nezha/pkg/ddns"
	"github.com/nezhahq/nezha/pkg/ddns/dummy"
	"github.com/nezhahq/nezha/pkg/ddns/webhook"
	"github.com/nezhahq/nezha/pkg/utils"
)

// DDNSClass 管理 DDNS profile 的内存缓存
type DDNSClass struct {
	class[uint64, *model.DDNSProfile]
}

// NewDDNSClass 从数据库加载所有 DDNS profile 到内存
func NewDDNSClass() *DDNSClass {
	var sortedList []*model.DDNSProfile

	DB.Find(&sortedList)
	list := make(map[uint64]*model.DDNSProfile, len(sortedList))
	for _, profile := range sortedList {
		list[profile.ID] = profile
	}

	dc := &DDNSClass{
		class: class[uint64, *model.DDNSProfile]{
			list:       list,
			sortedList: sortedList,
		},
	}
	return dc
}

// NewEmptyDDNSClassForTest 创建空的 DDNSClass 用于测试
func NewEmptyDDNSClassForTest() *DDNSClass {
	return &DDNSClass{
		class: class[uint64, *model.DDNSProfile]{
			list: map[uint64]*model.DDNSProfile{},
		},
	}
}

// InsertForTest 向测试用的 DDNSClass 中插入 profile
func (c *DDNSClass) InsertForTest(p *model.DDNSProfile) {
	c.listMu.Lock()
	c.list[p.ID] = p
	c.listMu.Unlock()
	c.sortList()
}

// Update 更新内存中的 DDNS profile
func (c *DDNSClass) Update(p *model.DDNSProfile) {
	c.listMu.Lock()
	c.list[p.ID] = p
	c.listMu.Unlock()

	c.sortList()
}

// Delete 从内存中删除指定的 DDNS profile
func (c *DDNSClass) Delete(idList []uint64) {
	c.listMu.Lock()
	for _, id := range idList {
		delete(c.list, id)
	}
	c.listMu.Unlock()

	c.sortList()
}

// profileOwnedByRealAdmin 判断 uid 是否为真正的管理员用户
func profileOwnedByRealAdmin(uid uint64) bool {
	return uid != 0 && userIsAdmin(uid)
}

// GetDDNSProvidersFromProfiles 根据 profile ID 列表获取 DDNS provider，
// 会校验 profile 归属权（GHSA-39g2-8x68-pmx8）
func (c *DDNSClass) GetDDNSProvidersFromProfiles(profileId []uint64, ip *model.IP, ownerUID uint64) ([]*ddns2.Provider, error) {
	profiles := make([]*model.DDNSProfile, 0, len(profileId))

	c.listMu.RLock()
	for _, id := range profileId {
		if profile, ok := c.list[id]; ok {
			if profile.UserID != ownerUID && !profileOwnedByRealAdmin(profile.UserID) {
				// Fail-closed：管理员可以绑定 member 拥有的 profile，
				// 但 worker 时只运行同 owner 或真正 admin 的 profile
				log.Printf("NEZHA>> Skipping DDNS profile %d (owner %d) for server owner %d: not owned by server owner or a real admin", profile.ID, profile.UserID, ownerUID)
				continue
			}
			profiles = append(profiles, profile)
		} else {
			c.listMu.RUnlock()
			return nil, fmt.Errorf("cannot find DDNS profile %d", id)
		}
	}
	c.listMu.RUnlock()

	providers := make([]*ddns2.Provider, 0, len(profiles))
	for _, profile := range profiles {
		provider := &ddns2.Provider{DDNSProfile: profile, IPAddrs: ip}
		switch profile.Provider {
		case model.ProviderDummy:
			provider.Setter = &dummy.Provider{}
			providers = append(providers, provider)
		case model.ProviderWebHook:
			provider.Setter = &webhook.Provider{DDNSProfile: profile}
			providers = append(providers, provider)
		case model.ProviderCloudflare:
			provider.Setter = &cloudflare.Provider{APIToken: profile.AccessSecret}
			providers = append(providers, provider)
		case model.ProviderTencentCloud:
			provider.Setter = &tencentcloud.Provider{SecretId: profile.AccessID, SecretKey: profile.AccessSecret}
			providers = append(providers, provider)
		case model.ProviderHE:
			provider.Setter = &he.Provider{APIKey: profile.AccessSecret}
			providers = append(providers, provider)
		default:
			return nil, fmt.Errorf("cannot find DDNS provider %s", profile.Provider)
		}
	}
	return providers, nil
}

// sortList 按 ID 排序 DDNS profile 列表
func (c *DDNSClass) sortList() {
	c.listMu.RLock()
	defer c.listMu.RUnlock()

	sortedList := utils.MapValuesToSlice(c.list)
	slices.SortFunc(sortedList, func(a, b *model.DDNSProfile) int {
		return cmp.Compare(a.ID, b.ID)
	})

	c.sortedListMu.Lock()
	defer c.sortedListMu.Unlock()
	c.sortedList = sortedList
}
