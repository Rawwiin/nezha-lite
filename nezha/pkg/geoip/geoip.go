package geoip

import (
	_ "embed"
	"net"
	"strings"
	"sync"

	maxminddb "github.com/oschwald/maxminddb-golang"
)

//go:embed geoip.db
var db []byte

var (
	dbOnce = sync.OnceValues(func() (*maxminddb.Reader, error) {
		db, err := maxminddb.FromBytes(db)
		if err != nil {
			return nil, err
		}
		return db, nil
	})
)

type IPInfo struct {
	Country       string `maxminddb:"country"`
	CountryName   string `maxminddb:"country_name"`
	Continent     string `maxminddb:"continent"`
	ContinentName string `maxminddb:"continent_name"`
}

// Lookup 查询 IP 的 GeoIP 信息，数据库无效时返回空字符串但不报错
func Lookup(ip net.IP) (string, error) {
	db, err := dbOnce()
	if err != nil {
		// GeoIP 数据库无效时 graceful 降级，不阻止程序运行
		return "", nil
	}

	var record IPInfo
	err = db.Lookup(ip, &record)
	if err != nil {
		return "", nil
	}

	if record.Country != "" {
		return strings.ToLower(record.Country), nil
	} else if record.Continent != "" {
		return strings.ToLower(record.Continent), nil
	}

	return "", nil
}
