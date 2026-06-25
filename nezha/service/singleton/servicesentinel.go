package singleton

import (
	"cmp"
	"iter"
	"log"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"
	"golang.org/x/exp/constraints"

	"github.com/nezhahq/nezha/model"
	"github.com/nezhahq/nezha/pkg/tsdb"
	"github.com/nezhahq/nezha/pkg/utils"
	pb "github.com/nezhahq/nezha/proto"
)

const (
	_CurrentStatusSize = 30 // 统计 15 分钟内的数据为当前状态
)

type serviceResponseItem struct {
	model.ServiceResponseItem
	service *model.Service
}

type ReportData struct {
	Data     *pb.TaskResult
	Reporter uint64
}

type _TodayStatsOfService struct {
	Up    uint64
	Down  uint64
	Delay float64
}

type serviceResponseData = _TodayStatsOfService

type serviceTaskStatus struct {
	lastStatus uint8
	t          time.Time
	result     []*pb.TaskResult
}

type pingStore struct {
	count        int
	ping         float64
	successCount int
}

type ServiceSentinel struct {
	serviceReportChannel chan ReportData

	serviceResponseDataStoreLock sync.RWMutex
	serviceStatusToday           map[uint64]*_TodayStatsOfService
	serviceCurrentStatusData     map[uint64]*serviceTaskStatus
	serviceResponseDataStore     map[uint64]serviceResponseData

	serviceResponsePing map[uint64]map[uint64]*pingStore
	tlsCertCache        map[uint64]string

	servicesLock    sync.RWMutex
	serviceListLock sync.RWMutex
	services        map[uint64]*model.Service
	serviceList     []*model.Service

	monthlyStatusLock sync.Mutex
	monthlyStatus     map[uint64]*serviceResponseItem

	tickers     map[uint64]*time.Ticker
	tickersLock sync.Mutex

	closeOnce sync.Once
	workerWG  sync.WaitGroup
}

func NewServiceSentinel() (*ServiceSentinel, error) {
	ss := &ServiceSentinel{
		serviceReportChannel:     make(chan ReportData, 200),
		serviceStatusToday:       make(map[uint64]*_TodayStatsOfService),
		serviceCurrentStatusData: make(map[uint64]*serviceTaskStatus),
		serviceResponseDataStore: make(map[uint64]serviceResponseData),
		serviceResponsePing:      make(map[uint64]map[uint64]*pingStore),
		services:                 make(map[uint64]*model.Service),
		tlsCertCache:             make(map[uint64]string),
		monthlyStatus:            make(map[uint64]*serviceResponseItem),
		tickers:                  make(map[uint64]*time.Ticker),
	}

	if err := ss.loadServiceHistory(); err != nil {
		return nil, err
	}

	ss.workerWG.Add(1)
	go func() {
		defer ss.workerWG.Done()
		ss.worker()
	}()

	go ss.dailyRefreshWorker()

	return ss, nil
}

// dailyRefreshWorker 每日刷新月度状态
func (ss *ServiceSentinel) dailyRefreshWorker() {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, Loc)
	time.Sleep(next.Sub(now))

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		ss.refreshMonthlyServiceStatus()
		<-ticker.C
	}
}

func (ss *ServiceSentinel) refreshMonthlyServiceStatus() {
	ss.LoadStats()
	ss.serviceResponseDataStoreLock.Lock()
	defer ss.serviceResponseDataStoreLock.Unlock()
	ss.monthlyStatusLock.Lock()
	defer ss.monthlyStatusLock.Unlock()
	for k, v := range ss.monthlyStatus {
		for i := range len(v.Up) - 1 {
			if i == 0 {
				v.TotalDown -= v.Down[i]
				v.TotalUp -= v.Up[i]
			}
			v.Up[i], v.Down[i], v.Delay[i] = v.Up[i+1], v.Down[i+1], v.Delay[i+1]
		}
		v.Up[29] = 0
		v.Down[29] = 0
		v.Delay[29] = 0
		ss.serviceResponseDataStore[k] = serviceResponseData{}
		ss.serviceStatusToday[k].Delay = 0
		ss.serviceStatusToday[k].Up = 0
		ss.serviceStatusToday[k].Down = 0
	}
}

func (ss *ServiceSentinel) Dispatch(r ReportData) {
	ss.serviceReportChannel <- r
}

func sortServices(services []*model.Service) {
	slices.SortFunc(services, func(a, b *model.Service) int {
		if a.DisplayIndex != b.DisplayIndex {
			return cmp.Compare(b.DisplayIndex, a.DisplayIndex)
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

func (ss *ServiceSentinel) UpdateServiceList() {
	ss.servicesLock.RLock()
	defer ss.servicesLock.RUnlock()

	ss.serviceListLock.Lock()
	defer ss.serviceListLock.Unlock()

	ss.serviceList = utils.MapValuesToSlice(ss.services)
	sortServices(ss.serviceList)
}

func (ss *ServiceSentinel) loadServiceHistory() error {
	var services []*model.Service
	if err := DB.Find(&services).Error; err != nil {
		return err
	}

	for _, service := range services {
		ss.services[service.ID] = service
		ss.serviceCurrentStatusData[service.ID] = new(serviceTaskStatus)
		ss.serviceCurrentStatusData[service.ID].result = make([]*pb.TaskResult, 0, _CurrentStatusSize)
		ss.serviceStatusToday[service.ID] = &_TodayStatsOfService{}
		ss.startServiceTicker(service)
	}
	ss.serviceList = services
	sortServices(ss.serviceList)

	year, month, day := time.Now().Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, Loc)

	for _, service := range services {
		ss.monthlyStatus[service.ID] = &serviceResponseItem{
			service: service,
			ServiceResponseItem: model.ServiceResponseItem{
				Delay: &[30]float64{},
				Up:    &[30]uint64{},
				Down:  &[30]uint64{},
			},
		}
	}

	ss.loadMonthlyStatusFromDB(today)
	return nil
}

func (ss *ServiceSentinel) loadMonthlyStatusFromDB(today time.Time) {
	var mhs []model.ServiceHistory
	DB.Where("created_at > ? AND created_at < ? AND server_id = 0", today.AddDate(0, 0, -29), today).Find(&mhs)
	delayCount := make(map[uint64]map[int]int)
	for _, mh := range mhs {
		dayIndex := 28 - int(today.Sub(mh.CreatedAt).Hours())/24
		if dayIndex < 0 {
			continue
		}
		ms := ss.monthlyStatus[mh.ServiceID]
		if ms == nil {
			continue
		}
		if delayCount[mh.ServiceID] == nil {
			delayCount[mh.ServiceID] = make(map[int]int)
		}
		ms.Delay[dayIndex] = (ms.Delay[dayIndex]*float64(delayCount[mh.ServiceID][dayIndex]) + mh.AvgDelay) / float64(delayCount[mh.ServiceID][dayIndex]+1)
		delayCount[mh.ServiceID][dayIndex]++
		ms.Up[dayIndex] += mh.Up
		ms.TotalUp += mh.Up
		ms.Down[dayIndex] += mh.Down
		ms.TotalDown += mh.Down
	}
}

func (ss *ServiceSentinel) loadTodayStats(today time.Time) {
	var mhs []model.ServiceHistory
	DB.Where("created_at >= ? AND server_id = 0", today).Find(&mhs)
	totalDelay := make(map[uint64]float64)
	totalDelayCount := make(map[uint64]int)
	for _, mh := range mhs {
		ss.serviceStatusToday[mh.ServiceID].Up += mh.Up
		ss.monthlyStatus[mh.ServiceID].TotalUp += mh.Up
		ss.serviceStatusToday[mh.ServiceID].Down += mh.Down
		ss.monthlyStatus[mh.ServiceID].TotalDown += mh.Down
		totalDelay[mh.ServiceID] += mh.AvgDelay
		totalDelayCount[mh.ServiceID]++
	}
	for id, delay := range totalDelay {
		ss.serviceStatusToday[id].Delay = delay / float64(totalDelayCount[id])
	}
}

func (ss *ServiceSentinel) startServiceTicker(s *model.Service) {
	ss.tickersLock.Lock()
	defer ss.tickersLock.Unlock()

	if oldTicker, ok := ss.tickers[s.ID]; ok {
		oldTicker.Stop()
		delete(ss.tickers, s.ID)
	}

	duration := time.Duration(s.Duration) * time.Second
	if duration <= 0 {
		duration = 30 * time.Second
	}

	ticker := time.NewTicker(duration)
	ss.tickers[s.ID] = ticker

	go func(service *model.Service) {
		for range ticker.C {
			ss.dispatchServiceTask(service)
		}
	}(s)
}

func (ss *ServiceSentinel) stopServiceTicker(id uint64) {
	ss.tickersLock.Lock()
	defer ss.tickersLock.Unlock()
	if ticker, ok := ss.tickers[id]; ok {
		ticker.Stop()
		delete(ss.tickers, id)
	}
}

func (ss *ServiceSentinel) dispatchServiceTask(s *model.Service) {
	servers := ServerShared.GetList()
	for _, server := range servers {
		if canDispatchServiceTask(s, server) {
			task := s.PB()
			if err := server.SendTask(task); err != nil {
				// Agent 离线，正常情况，忽略
			}
		}
	}
}

func canDispatchServiceTask(service *model.Service, server *model.Server) bool {
	if service == nil || server == nil {
		return false
	}
	if service.UserID != server.GetUserID() && !userIsAdmin(service.UserID) {
		return false
	}
	switch service.Cover {
	case model.ServiceCoverAll:
		if service.SkipServers[server.ID] {
			return false
		}
	case model.ServiceCoverIgnoreAll:
		if !service.SkipServers[server.ID] {
			return false
		}
	default:
		return false
	}
	return true
}

func (ss *ServiceSentinel) Update(m *model.Service) error {
	ss.serviceResponseDataStoreLock.Lock()
	defer ss.serviceResponseDataStoreLock.Unlock()
	ss.monthlyStatusLock.Lock()
	defer ss.monthlyStatusLock.Unlock()
	ss.servicesLock.Lock()
	defer ss.servicesLock.Unlock()

	ss.startServiceTicker(m)

	if ss.services[m.ID] != nil {
		ss.stopServiceTicker(m.ID)
		ss.startServiceTicker(m)
	} else {
		ss.monthlyStatus[m.ID] = &serviceResponseItem{
			service: m,
			ServiceResponseItem: model.ServiceResponseItem{
				Delay: &[30]float64{},
				Up:    &[30]uint64{},
				Down:  &[30]uint64{},
			},
		}
		if ss.serviceCurrentStatusData[m.ID] == nil {
			ss.serviceCurrentStatusData[m.ID] = new(serviceTaskStatus)
		}
		ss.serviceCurrentStatusData[m.ID].result = make([]*pb.TaskResult, 0, _CurrentStatusSize)
		ss.serviceStatusToday[m.ID] = &_TodayStatsOfService{}
	}
	ss.services[m.ID] = m
	return nil
}

func (ss *ServiceSentinel) Delete(ids []uint64) {
	ss.serviceResponseDataStoreLock.Lock()
	defer ss.serviceResponseDataStoreLock.Unlock()
	ss.monthlyStatusLock.Lock()
	defer ss.monthlyStatusLock.Unlock()
	ss.servicesLock.Lock()
	defer ss.servicesLock.Unlock()

	for _, id := range ids {
		ss.stopServiceTicker(id)
		delete(ss.serviceCurrentStatusData, id)
		delete(ss.serviceResponseDataStore, id)
		delete(ss.tlsCertCache, id)
		delete(ss.serviceStatusToday, id)
		delete(ss.services, id)
		delete(ss.monthlyStatus, id)
	}
}

func (ss *ServiceSentinel) LoadStats() map[uint64]*serviceResponseItem {
	ss.servicesLock.RLock()
	defer ss.servicesLock.RUnlock()
	ss.serviceResponseDataStoreLock.RLock()
	defer ss.serviceResponseDataStoreLock.RUnlock()
	ss.monthlyStatusLock.Lock()
	defer ss.monthlyStatusLock.Unlock()

	for k := range ss.services {
		ss.monthlyStatus[k].service = ss.services[k]
		v := ss.serviceStatusToday[k]

		ss.monthlyStatus[k].TotalUp -= ss.monthlyStatus[k].Up[29]
		ss.monthlyStatus[k].TotalDown -= ss.monthlyStatus[k].Down[29]
		ss.monthlyStatus[k].TotalUp += v.Up
		ss.monthlyStatus[k].TotalDown += v.Down

		ss.monthlyStatus[k].Up[29] = v.Up
		ss.monthlyStatus[k].Down[29] = v.Down
		ss.monthlyStatus[k].Delay[29] = v.Delay
	}

	for k, v := range ss.serviceResponseDataStore {
		ss.monthlyStatus[k].CurrentDown = v.Down
		ss.monthlyStatus[k].CurrentUp = v.Up
	}

	return ss.monthlyStatus
}

func (ss *ServiceSentinel) CopyStats() map[uint64]model.ServiceResponseItem {
	var stats map[uint64]*serviceResponseItem
	copier.Copy(&stats, ss.LoadStats())

	sri := make(map[uint64]model.ServiceResponseItem)
	for k, service := range stats {
		service.ServiceName = service.service.Name
		sri[k] = service.ServiceResponseItem
	}

	return sri
}

func (ss *ServiceSentinel) Get(id uint64) (s *model.Service, ok bool) {
	ss.servicesLock.RLock()
	defer ss.servicesLock.RUnlock()

	s, ok = ss.services[id]
	return
}

func (ss *ServiceSentinel) GetList() map[uint64]*model.Service {
	ss.servicesLock.RLock()
	defer ss.servicesLock.RUnlock()

	return maps.Clone(ss.services)
}

func (ss *ServiceSentinel) GetSortedList() []*model.Service {
	ss.serviceListLock.RLock()
	defer ss.serviceListLock.RUnlock()

	return slices.Clone(ss.serviceList)
}

func (ss *ServiceSentinel) CheckPermission(c *gin.Context, idList iter.Seq[uint64]) bool {
	ss.servicesLock.RLock()
	defer ss.servicesLock.RUnlock()

	for id := range idList {
		if s, ok := ss.services[id]; ok {
			if !s.HasPermission(c) {
				return false
			}
		}
	}
	return true
}

func canReportServiceResult(service *model.Service, reporter *model.Server, taskType uint64) bool {
	if service == nil || reporter == nil || uint64(service.Type) != taskType {
		return false
	}
	switch service.Cover {
	case model.ServiceCoverAll:
		if service.SkipServers[reporter.ID] {
			return false
		}
	case model.ServiceCoverIgnoreAll:
		if !service.SkipServers[reporter.ID] {
			return false
		}
	default:
		return false
	}

	return service.UserID == reporter.GetUserID() || userIsAdmin(service.UserID)
}

func (ss *ServiceSentinel) Close() {
	ss.closeOnce.Do(func() {
		ss.tickersLock.Lock()
		for _, t := range ss.tickers {
			t.Stop()
		}
		ss.tickersLock.Unlock()
		close(ss.serviceReportChannel)
		ss.workerWG.Wait()
	})
}

func (ss *ServiceSentinel) worker() {
	for r := range ss.serviceReportChannel {
		cs, _ := ss.Get(r.Data.GetId())
		reporter, _ := ServerShared.Get(r.Reporter)
		if !canReportServiceResult(cs, reporter, r.Data.GetType()) {
			log.Printf("NEZHA>> Incorrect service monitor report %+v", r)
			continue
		}

		mh := r.Data
		if mh.Type == model.TaskTypeTCPPing || mh.Type == model.TaskTypeICMPPing {
			serviceTcpMap, ok := ss.serviceResponsePing[mh.GetId()]
			if !ok {
				serviceTcpMap = make(map[uint64]*pingStore)
				ss.serviceResponsePing[mh.GetId()] = serviceTcpMap
			}
			ts, ok := serviceTcpMap[r.Reporter]
			if !ok {
				ts = &pingStore{}
			}
			ts.count++
			ts.ping = (ts.ping*float64(ts.count-1) + float64(mh.Delay)) / float64(ts.count)
			if mh.Successful {
				ts.successCount++
			}
			if ts.count == Conf.AvgPingCount {
				if TSDBEnabled() {
					if err := TSDBShared.WriteServiceMetrics(&tsdb.ServiceMetrics{
						ServiceID:  mh.GetId(),
						ServerID:   r.Reporter,
						Timestamp:  time.Now(),
						Delay:      ts.ping,
						Successful: ts.successCount*2 >= ts.count,
					}); err != nil {
						log.Printf("NEZHA>> Failed to save service monitor metrics to TSDB: %v", err)
					}
				} else {
					if err := DB.Create(&model.ServiceHistory{
						ServiceID: mh.GetId(),
						AvgDelay:  ts.ping,
						Data:      mh.Data,
						ServerID:  r.Reporter,
					}).Error; err != nil {
						log.Printf("NEZHA>> Failed to save service monitor metrics: %v", err)
					}
				}
				ts.count = 0
				ts.ping = 0
				ts.successCount = 0
			}
			serviceTcpMap[r.Reporter] = ts
		} else {
			if TSDBEnabled() {
				if err := TSDBShared.WriteServiceMetrics(&tsdb.ServiceMetrics{
					ServiceID:  mh.GetId(),
					ServerID:   r.Reporter,
					Timestamp:  time.Now(),
					Delay:      float64(mh.Delay),
					Successful: mh.Successful,
				}); err != nil {
					log.Printf("NEZHA>> Failed to save service monitor metrics to TSDB: %v", err)
				}
			}
		}

		ss.serviceResponseDataStoreLock.Lock()
		if mh.Successful {
			ss.serviceStatusToday[mh.GetId()].Delay = (ss.serviceStatusToday[mh.GetId()].Delay*float64(ss.serviceStatusToday[mh.GetId()].Up) + float64(mh.Delay)) / float64(ss.serviceStatusToday[mh.GetId()].Up+1)
			ss.serviceStatusToday[mh.GetId()].Up++
		} else {
			ss.serviceStatusToday[mh.GetId()].Down++
		}

		currentTime := time.Now()
		if ss.serviceCurrentStatusData[mh.GetId()].t.IsZero() {
			ss.serviceCurrentStatusData[mh.GetId()].t = currentTime
		}

		if ss.serviceCurrentStatusData[mh.GetId()].t.Before(currentTime) {
			ss.serviceCurrentStatusData[mh.GetId()].t = currentTime.Add(30 * time.Second)
			ss.serviceCurrentStatusData[mh.GetId()].result = append(ss.serviceCurrentStatusData[mh.GetId()].result, mh)
		}

		ss.serviceResponseDataStore[mh.GetId()] = serviceResponseData{}

		for _, cs := range ss.serviceCurrentStatusData[mh.GetId()].result {
			if cs.GetId() > 0 {
				rd := ss.serviceResponseDataStore[mh.GetId()]
				if cs.Successful {
					rd.Up++
					rd.Delay = (rd.Delay*float64(rd.Up-1) + float64(cs.Delay)) / float64(rd.Up)
				} else {
					rd.Down++
				}
				ss.serviceResponseDataStore[mh.GetId()] = rd
			}
		}

		var stateCode uint8
		{
			upPercent := uint64(0)
			rd := ss.serviceResponseDataStore[mh.GetId()]
			if rd.Down+rd.Up > 0 {
				upPercent = rd.Up * 100 / (rd.Down + rd.Up)
			}
			stateCode = GetStatusCode(upPercent)
		}

		if len(ss.serviceCurrentStatusData[mh.GetId()].result) == _CurrentStatusSize {
			ss.serviceCurrentStatusData[mh.GetId()].t = currentTime
			if !TSDBEnabled() {
				rd := ss.serviceResponseDataStore[mh.GetId()]
				if err := DB.Create(&model.ServiceHistory{
					ServiceID: mh.GetId(),
					AvgDelay:  rd.Delay,
					Data:      mh.Data,
					Up:        rd.Up,
					Down:      rd.Down,
				}).Error; err != nil {
					log.Printf("NEZHA>> Failed to save service monitor metrics: %v", err)
				}
			}
			ss.serviceCurrentStatusData[mh.GetId()].result = ss.serviceCurrentStatusData[mh.GetId()].result[:0]
		}

		cs, _ = ss.Get(mh.GetId())
		m := ServerShared.GetList()
		if mh.Delay > 0 {
			delayCheck(&r, m, cs, mh)
		}

		if stateCode == StatusDown || stateCode != ss.serviceCurrentStatusData[mh.GetId()].lastStatus {
			lastStatus := ss.serviceCurrentStatusData[mh.GetId()].lastStatus
			ss.serviceCurrentStatusData[mh.GetId()].lastStatus = stateCode
			notifyCheck(&r, m, cs, mh, lastStatus, stateCode)
		}
		ss.serviceResponseDataStoreLock.Unlock()

		// TLS 证书检查（仅记录日志，不发通知）
		if strings.HasPrefix(mh.Data, "SSL证书错误：") {
			if !strings.HasSuffix(mh.Data, "timeout") &&
				!strings.HasSuffix(mh.Data, "EOF") &&
				!strings.HasSuffix(mh.Data, "timed out") {
				log.Printf("NEZHA>> [TLS] Fetch cert info failed, Service: %s, Error: %s", cs.Name, mh.Data)
			}
		} else {
			var newCert = strings.Split(mh.Data, "|")
			if len(newCert) > 1 {
				if ss.tlsCertCache[mh.GetId()] == "" {
					ss.tlsCertCache[mh.GetId()] = mh.Data
				}
				oldCert := strings.Split(ss.tlsCertCache[mh.GetId()], "|")
				expiresOld, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", oldCert[1])
				expiresNew, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", newCert[1])
				if oldCert[0] != newCert[0] && !expiresNew.Equal(expiresOld) {
					ss.tlsCertCache[mh.GetId()] = mh.Data
					log.Printf("NEZHA>> [TLS] Certificate changed for service %s", cs.Name)
				}
				if expiresNew.Before(time.Now().AddDate(0, 0, 7)) {
					log.Printf("NEZHA>> [TLS] Certificate will expire within 7 days for service %s, expires: %s", cs.Name, expiresNew.Format("2006-01-02 15:04:05"))
				}
			}
		}
	}
}

func delayCheck(r *ReportData, m map[uint64]*model.Server, ss *model.Service, mh *pb.TaskResult) {
	if !ss.LatencyNotify {
		return
	}
	if mh.Delay > ss.MaxLatency {
		reporterServer := m[r.Reporter]
		log.Printf("NEZHA>> [Latency] %s %2f > %2f, Reporter: %s", ss.Name, mh.Delay, ss.MaxLatency, reporterServer.Name)
	} else if mh.Delay < ss.MinLatency {
		reporterServer := m[r.Reporter]
		log.Printf("NEZHA>> [Latency] %s %2f < %2f, Reporter: %s", ss.Name, mh.Delay, ss.MinLatency, reporterServer.Name)
	}
}

func notifyCheck(r *ReportData, m map[uint64]*model.Server,
	ss *model.Service, mh *pb.TaskResult, lastStatus, stateCode uint8) {
	isNeedSendNotification := ss.Notify && (lastStatus != 0 || stateCode == StatusDown)
	if isNeedSendNotification {
		reporterServer := m[r.Reporter]
		log.Printf("NEZHA>> [%s] %s Reporter: %s, Error: %s", StatusCodeToString(stateCode), ss.Name, reporterServer.Name, mh.Data)
	}

	// 精简版：不触发 Cron 恢复/失败任务（Cron 系统已删除）
}

const (
	_ = iota
	StatusNoData
	StatusGood
	StatusLowAvailability
	StatusDown
)

func GetStatusCode[T constraints.Float | constraints.Integer](percent T) uint8 {
	if percent == 0 {
		return StatusNoData
	}
	if percent > 95 {
		return StatusGood
	}
	if percent > 80 {
		return StatusLowAvailability
	}
	return StatusDown
}

func StatusCodeToString(statusCode uint8) string {
	switch statusCode {
	case StatusNoData:
		return Localizer.T("No Data")
	case StatusGood:
		return Localizer.T("Good")
	case StatusLowAvailability:
		return Localizer.T("Low Availability")
	case StatusDown:
		return Localizer.T("Down")
	default:
		return ""
	}
}
