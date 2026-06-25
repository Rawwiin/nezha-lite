package singleton

import (
	_ "embed"
	"iter"
	"log"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"gorm.io/gorm"
	"sigs.k8s.io/yaml"

	"github.com/nezhahq/nezha/model"
	"github.com/nezhahq/nezha/pkg/utils"
	"github.com/nezhahq/nezha/service/singleton/sqlitedrv"
)

var Version = "debug"

var (
	Cache             *cache.Cache
	DB                *gorm.DB
	Loc               *time.Location
	FrontendTemplates []model.FrontendTemplate
	DashboardBootTime = uint64(time.Now().Unix())

	ServerShared          *ServerClass
	ServiceSentinelShared *ServiceSentinel
	DDNSShared            *DDNSClass
	NotificationShared    *NotificationClass
)

//go:embed frontend-templates.yaml
var frontendTemplatesYAML []byte

func InitTimezoneAndCache() error {
	var err error
	Loc, err = time.LoadLocation(Conf.Location)
	if err != nil {
		return err
	}

	Cache = cache.New(5*time.Minute, 10*time.Minute)
	return nil
}

// LoadSingleton 加载子服务并执行
func LoadSingleton() (err error) {
	initI18n() // 加载本地化服务
	initUser() // 加载用户ID绑定表
	DDNSShared = NewDDNSClass()
	ServerShared = NewServerClass()
	ServiceSentinelShared, err = NewServiceSentinel()
	if err != nil {
		return err
	}
	NotificationShared = NewNotificationClass()
	// 启动告警检查
	go AlertSentinelStart()
	return
}

// InitFrontendTemplates 从内置文件中加载FrontendTemplates
func InitFrontendTemplates() error {
	err := yaml.Unmarshal(frontendTemplatesYAML, &FrontendTemplates)
	if err != nil {
		return err
	}
	return nil
}

// InitDBFromPath 从给出的文件路径中加载数据库
func InitDBFromPath(path string) error {
	var err error
	DB, err = gorm.Open(sqlitedrv.Open(path), &gorm.Config{
		CreateBatchSize: 200,
	})
	if err != nil {
		return err
	}
	// 设置 SQLite PRAGMA：modernc 驱动不支持通过 DSN 传参，需手动执行
	// busy_timeout: 并发写入时等待锁最多 5 秒，避免立即返回 SQLITE_BUSY
	// journal_mode=WAL: 允许读写并发，大幅减少锁冲突
	// synchronous=NORMAL: WAL 模式下的推荐设置，兼顾性能与安全
	if err := DB.Exec("PRAGMA busy_timeout = 5000").Error; err != nil {
		return err
	}
	if err := DB.Exec("PRAGMA journal_mode = WAL").Error; err != nil {
		return err
	}
	if err := DB.Exec("PRAGMA synchronous = NORMAL").Error; err != nil {
		return err
	}
	if Conf.Debug {
		DB = DB.Debug()
	}
	err = DB.AutoMigrate(model.Server{}, model.User{}, model.ServerGroup{}, model.ServerGroupServer{},
		model.WAF{}, model.JWTSession{}, model.APIToken{}, model.Service{}, model.ServiceHistory{},
		model.Transfer{}, model.Notification{}, model.NotificationGroup{}, model.NotificationGroupNotification{},
		model.AlertRule{})
	if err != nil {
		return err
	}

	return nil
}

// RecordTransferHourlyUsage 对流量记录进行打点
func RecordTransferHourlyUsage(servers ...*model.Server) {
	now := time.Now()
	nowTrimSeconds := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())

	var txs []model.Transfer
	var slist iter.Seq[*model.Server]
	if len(servers) > 0 {
		slist = slices.Values(servers)
	} else {
		slist = utils.Seq2To1(ServerShared.Range)
	}

	for server := range slist {
		tx := model.Transfer{
			ServerID: server.ID,
			In:       utils.SubUintChecked(server.State.NetInTransfer, server.PrevTransferInSnapshot),
			Out:      utils.SubUintChecked(server.State.NetOutTransfer, server.PrevTransferOutSnapshot),
		}
		if tx.In == 0 && tx.Out == 0 {
			continue
		}
		server.PrevTransferInSnapshot = server.State.NetInTransfer
		server.PrevTransferOutSnapshot = server.State.NetOutTransfer
		tx.CreatedAt = nowTrimSeconds
		txs = append(txs, tx)
	}

	if len(txs) == 0 {
		return
	}
	log.Printf("NEZHA>> Saved traffic metrics to database. Affected %d row(s), Error: %v", len(txs), DB.Create(txs).Error)
}

// CleanMonitorHistory 清理流量记录（TSDB 有自己的保留策略）
func CleanMonitorHistory() {
	// 清理已被删除的服务器的流量记录
	DB.Unscoped().Delete(&model.Transfer{}, "server_id NOT IN (SELECT `id` FROM servers)")
}

// PerformMaintenance 执行系统维护（SQLite VACUUM 和 TSDB 维护）
func PerformMaintenance() {
	log.Println("NEZHA>> Starting system maintenance...")

	// 1. SQLite 维护
	if DB != nil {
		log.Println("NEZHA>> SQLite: Starting VACUUM...")
		if err := DB.Exec("VACUUM").Error; err != nil {
			log.Printf("NEZHA>> SQLite: VACUUM failed: %v", err)
		} else {
			log.Println("NEZHA>> SQLite: VACUUM completed")
		}
	}

	// 2. TSDB 维护
	if TSDBEnabled() {
		TSDBShared.Maintenance()
	}

	log.Println("NEZHA>> System maintenance completed")
}

// IPDesensitize 根据设置选择是否对IP进行打码处理 返回处理后的IP(关闭打码则返回原IP)
func IPDesensitize(ip string) string {
	if Conf.EnablePlainIPInNotification {
		return ip
	}
	return utils.IPDesensitize(ip)
}

type class[K comparable, V model.CommonInterface] struct {
	list   map[K]V
	listMu sync.RWMutex

	sortedList   []V
	sortedListMu sync.RWMutex
}

func (c *class[K, V]) Get(id K) (s V, ok bool) {
	c.listMu.RLock()
	defer c.listMu.RUnlock()

	s, ok = c.list[id]
	return
}

func (c *class[K, V]) GetList() map[K]V {
	c.listMu.RLock()
	defer c.listMu.RUnlock()

	return maps.Clone(c.list)
}

func (c *class[K, V]) GetSortedList() []V {
	c.sortedListMu.RLock()
	defer c.sortedListMu.RUnlock()

	return slices.Clone(c.sortedList)
}

func (c *class[K, V]) Range(fn func(k K, v V) bool) {
	c.listMu.RLock()
	defer c.listMu.RUnlock()

	for k, v := range c.list {
		if !fn(k, v) {
			break
		}
	}
}

func (c *class[K, V]) CheckPermission(ctx *gin.Context, idList iter.Seq[K]) bool {
	c.listMu.RLock()
	defer c.listMu.RUnlock()

	for id := range idList {
		if s, ok := c.list[id]; ok {
			if !s.HasPermission(ctx) {
				return false
			}
		}
	}
	return true
}
