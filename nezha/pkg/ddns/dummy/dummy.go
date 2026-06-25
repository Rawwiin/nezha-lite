package dummy

import (
	"context"

	"github.com/libdns/libdns"
)

// Provider 内部测试用的 dummy DDNS provider
type Provider struct{}

// SetRecords 直接返回记录，不做任何操作
func (provider *Provider) SetRecords(ctx context.Context, zone string,
	recs []libdns.Record) ([]libdns.Record, error) {
	return recs, nil
}
