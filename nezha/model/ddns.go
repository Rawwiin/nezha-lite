package model

import (
	"github.com/goccy/go-json"
	"gorm.io/gorm"
)

// DDNS 服务商常量
const (
	ProviderDummy        = "dummy"
	ProviderWebHook      = "webhook"
	ProviderCloudflare   = "cloudflare"
	ProviderTencentCloud = "tencentcloud"
	ProviderHE           = "he"
)

// ProviderList 支持的 DDNS 服务商列表
var ProviderList = [...]string{
	ProviderDummy, ProviderWebHook, ProviderCloudflare, ProviderTencentCloud, ProviderHE,
}

// DDNSProfile DDNS 配置 profile
type DDNSProfile struct {
	Common
	EnableIPv4         *bool    `json:"enable_ipv4,omitempty"`
	EnableIPv6         *bool    `json:"enable_ipv6,omitempty"`
	MaxRetries         uint64   `json:"max_retries"`
	Name               string   `json:"name"`
	Provider           string   `json:"provider"`
	AccessID           string   `json:"access_id,omitempty"`
	AccessSecret       string   `json:"access_secret,omitempty"`
	WebhookURL         string   `json:"webhook_url,omitempty"`
	WebhookMethod      uint8    `json:"webhook_method,omitempty"`
	WebhookRequestType uint8    `json:"webhook_request_type,omitempty"`
	WebhookRequestBody string   `json:"webhook_request_body,omitempty"`
	WebhookHeaders     string   `json:"webhook_headers,omitempty"`
	Domains            []string `json:"domains" gorm:"-"`
	DomainsRaw         string   `json:"-"`
}

func (d *DDNSProfile) TableName() string {
	return "ddns"
}

// BeforeSave 保存前将 Domains 序列化为 JSON
func (d *DDNSProfile) BeforeSave(tx *gorm.DB) error {
	if data, err := json.Marshal(d.Domains); err != nil {
		return err
	} else {
		d.DomainsRaw = string(data)
	}
	return nil
}

// AfterFind 查询后将 JSON 反序列化为 Domains
func (d *DDNSProfile) AfterFind(tx *gorm.DB) error {
	return json.Unmarshal([]byte(d.DomainsRaw), &d.Domains)
}
