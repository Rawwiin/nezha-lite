package controller

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"
	"gorm.io/gorm"

	"github.com/nezhahq/nezha/model"
	"github.com/nezhahq/nezha/pkg/tsdb"
	"github.com/nezhahq/nezha/service/singleton"
)

// Show service
// @Summary Show service
// @Security BearerAuth
// @Schemes
// @Description Show service
// @Tags common
// @Produce json
// @Success 200 {object} model.CommonResponse[model.ServiceResponse]
// @Router /service [get]
func showService(c *gin.Context) (*model.ServiceResponse, error) {
	stats := filterServiceStatsForViewer(c, singleton.ServiceSentinelShared.CopyStats())
	return &model.ServiceResponse{
		Services: stats,
	}, nil
}

func filterServiceStatsForViewer(c *gin.Context, stats map[uint64]model.ServiceResponseItem) map[uint64]model.ServiceResponseItem {
	if len(stats) == 0 {
		return stats
	}
	services := singleton.ServiceSentinelShared.GetList()
	filteredStats := make(map[uint64]model.ServiceResponseItem, len(stats))
	for serviceID, stat := range stats {
		service, ok := services[serviceID]
		if !ok || !userCanViewService(c, service) {
			continue
		}
		filteredStats[serviceID] = stat
	}
	return filteredStats
}

// List service
// @Summary List service
// @Security BearerAuth
// @Schemes
// @Description List service
// @Tags auth required
// @Param id query uint false "Resource ID"
// @Produce json
// @Success 200 {object} model.CommonResponse[[]model.Service]
// @Router /service/list [get]
func listService(c *gin.Context) ([]*model.Service, error) {
	var ss []*model.Service
	ssl := singleton.ServiceSentinelShared.GetSortedList()
	if err := copier.Copy(&ss, &ssl); err != nil {
		return nil, err
	}
	return ss, nil
}

// Get service history
// @Summary Get service history by service ID
// @Security BearerAuth
// @Schemes
// @Description Get service monitoring history for a specific service
// @Tags common
// @param id path uint true "Service ID"
// @param period query string false "Time period: 1d, 7d, 30d (default: 1d)"
// @Produce json
// @Success 200 {object} model.CommonResponse[model.ServiceHistoryResponse]
// @Router /service/{id}/history [get]
func getServiceHistory(c *gin.Context) (*model.ServiceHistoryResponse, error) {
	idStr := c.Param("id")
	serviceID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return nil, err
	}

	service, ok := singleton.ServiceSentinelShared.Get(serviceID)
	if !ok || service == nil || !userCanViewService(c, service) {
		return nil, singleton.Localizer.ErrorT("service not found")
	}

	periodStr := c.DefaultQuery("period", "1d")
	period, err := tsdb.ParseQueryPeriod(periodStr)
	if err != nil {
		return nil, err
	}

	_, isMember := c.Get(model.CtxKeyAuthorizedUser)
	if !isMember && period != tsdb.Period1Day {
		return nil, singleton.Localizer.ErrorT("unauthorized: only 1d data available for guests")
	}

	// TSDB 启用时从 TSDB 查询，否则回退到 SQLite
	if singleton.TSDBEnabled() {
		return queryServiceHistoryFromTSDB(c, serviceID, period, service.Name)
	}
	return queryServiceHistoryFromDB(c, serviceID, period, service.Name)
}

// queryServiceHistoryFromTSDB 从 TSDB 查询服务监控历史
func queryServiceHistoryFromTSDB(c *gin.Context, serviceID uint64, period tsdb.QueryPeriod, serviceName string) (*model.ServiceHistoryResponse, error) {
	result, err := singleton.TSDBShared.QueryServiceHistory(serviceID, period)
	if err != nil {
		return nil, err
	}

	serverMap := singleton.ServerShared.GetList()
	filtered := result.Servers[:0]
	for i := range result.Servers {
		server, ok := serverMap[result.Servers[i].ServerID]
		if !ok || !userCanViewServer(c, server) {
			continue
		}
		result.Servers[i].ServerName = server.Name
		filtered = append(filtered, result.Servers[i])
	}
	result.Servers = filtered
	result.ServiceName = serviceName
	return result, nil
}

func queryServiceHistoryFromDB(c *gin.Context, serviceID uint64, period tsdb.QueryPeriod, serviceName string) (*model.ServiceHistoryResponse, error) {
	since := time.Now().Add(-period.Duration())

	var histories []model.ServiceHistory
	if err := singleton.DB.Where("service_id = ? AND server_id != 0 AND created_at >= ?", serviceID, since).
		Order("server_id, created_at").Find(&histories).Error; err != nil {
		return nil, err
	}

	serverMap := singleton.ServerShared.GetList()
	grouped := make(map[uint64][]model.ServiceHistory)
	for _, h := range histories {
		grouped[h.ServerID] = append(grouped[h.ServerID], h)
	}

	response := &model.ServiceHistoryResponse{
		ServiceID:   serviceID,
		ServiceName: serviceName,
		Servers:     make([]model.ServerServiceStats, 0),
	}

	for serverID, records := range grouped {
		server, ok := serverMap[serverID]
		if !ok || !userCanViewServer(c, server) {
			continue
		}
		stats := model.ServerServiceStats{
			ServerID:   serverID,
			ServerName: server.Name,
		}

		var totalDelay float64
		var totalUp, totalDown uint64
		dps := make([]model.DataPoint, 0, len(records))
		for _, r := range records {
			status := uint8(1)
			if r.Down > 0 && r.Up == 0 {
				status = 0
			}
			dps = append(dps, model.DataPoint{
				Timestamp: r.CreatedAt.Unix() * 1000,
				Delay:     r.AvgDelay,
				Status:    status,
			})
			totalDelay += r.AvgDelay
			totalUp += r.Up
			totalDown += r.Down
		}

		var avgDelay float64
		if len(records) > 0 {
			avgDelay = totalDelay / float64(len(records))
		}
		var upPercent float32
		if totalUp+totalDown > 0 {
			upPercent = float32(totalUp) / float32(totalUp+totalDown) * 100
		}
		stats.Stats = model.ServiceHistorySummary{
			AvgDelay:   avgDelay,
			UpPercent:  upPercent,
			TotalUp:    totalUp,
			TotalDown:  totalDown,
			DataPoints: dps,
		}
		response.Servers = append(response.Servers, stats)
	}

	return response, nil
}

// List server services
// @Summary List service histories by server id
// @Security BearerAuth
// @Schemes
// @Description List service histories for a specific server
// @Tags common
// @param id path uint true "Server ID"
// @param period query string false "Time period: 1d, 7d, 30d (default: 1d)"
// @Produce json
// @Success 200 {object} model.CommonResponse[[]model.ServiceInfos]
// @Router /server/{id}/service [get]
func listServerServices(c *gin.Context) ([]*model.ServiceInfos, error) {
	idStr := c.Param("id")
	serverID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return nil, err
	}

	m := singleton.ServerShared.GetList()
	server, ok := m[serverID]
	if !ok || server == nil {
		return nil, singleton.Localizer.ErrorT("server not found")
	}

	if !userCanViewServer(c, server) {
		return nil, singleton.Localizer.ErrorT("unauthorized")
	}

	periodStr := c.DefaultQuery("period", "1d")
	period, err := tsdb.ParseQueryPeriod(periodStr)
	if err != nil {
		return nil, err
	}

	_, isMember := c.Get(model.CtxKeyAuthorizedUser)
	if !isMember && period != tsdb.Period1Day {
		return nil, singleton.Localizer.ErrorT("unauthorized: only 1d data available for guests")
	}

	allServices := singleton.ServiceSentinelShared.GetSortedList()
	services := make([]*model.Service, 0, len(allServices))
	for _, s := range allServices {
		if userCanViewService(c, s) {
			services = append(services, s)
		}
	}

	// TSDB 启用时从 TSDB 查询，否则回退到 SQLite
	if singleton.TSDBEnabled() {
		return queryServerServicesFromTSDB(serverID, server.Name, period, services)
	}
	return queryServerServicesFromDB(serverID, server.Name, period, services)
}

// queryServerServicesFromTSDB 从 TSDB 查询指定服务器的服务监控历史
func queryServerServicesFromTSDB(serverID uint64, serverName string, period tsdb.QueryPeriod, services []*model.Service) ([]*model.ServiceInfos, error) {
	historyResults, err := singleton.TSDBShared.QueryServiceHistoryByServerID(serverID, period)
	if err != nil {
		return nil, err
	}

	var result []*model.ServiceInfos
	for _, service := range services {
		// 检查 server 是否在 service 的覆盖范围内
		if service.Cover == model.ServiceCoverAll {
			if service.SkipServers[serverID] {
				continue
			}
		} else {
			if !service.SkipServers[serverID] {
				continue
			}
		}

		historyResult, ok := historyResults[service.ID]
		if !ok || len(historyResult.Servers) == 0 {
			continue
		}

		serverStats := historyResult.Servers[0]
		infos := &model.ServiceInfos{
			ServiceID:    service.ID,
			ServerID:     serverID,
			ServiceName:  service.Name,
			ServerName:   serverName,
			DisplayIndex: service.DisplayIndex,
			CreatedAt:    make([]int64, len(serverStats.Stats.DataPoints)),
			AvgDelay:     make([]float64, len(serverStats.Stats.DataPoints)),
		}
		for i, dp := range serverStats.Stats.DataPoints {
			infos.CreatedAt[i] = dp.Timestamp
			infos.AvgDelay[i] = dp.Delay
		}
		result = append(result, infos)
	}

	return result, nil
}

func queryServerServicesFromDB(serverID uint64, serverName string, period tsdb.QueryPeriod, services []*model.Service) ([]*model.ServiceInfos, error) {
	since := time.Now().Add(-period.Duration())

	var histories []model.ServiceHistory
	if err := singleton.DB.Where("server_id = ? AND created_at >= ?", serverID, since).
		Order("service_id, created_at").Find(&histories).Error; err != nil {
		return nil, err
	}

	grouped := make(map[uint64][]model.ServiceHistory)
	for _, h := range histories {
		grouped[h.ServiceID] = append(grouped[h.ServiceID], h)
	}

	var result []*model.ServiceInfos
	for _, service := range services {
		if service.Cover == model.ServiceCoverAll {
			if service.SkipServers[serverID] {
				continue
			}
		} else {
			if !service.SkipServers[serverID] {
				continue
			}
		}

		records, ok := grouped[service.ID]
		if !ok {
			continue
		}

		infos := &model.ServiceInfos{
			ServiceID:    service.ID,
			ServerID:     serverID,
			ServiceName:  service.Name,
			ServerName:   serverName,
			DisplayIndex: service.DisplayIndex,
			CreatedAt:    make([]int64, 0, len(records)),
			AvgDelay:     make([]float64, 0, len(records)),
		}

		for _, r := range records {
			infos.CreatedAt = append(infos.CreatedAt, r.CreatedAt.Truncate(time.Minute).Unix()*1000)
			infos.AvgDelay = append(infos.AvgDelay, r.AvgDelay)
		}

		result = append(result, infos)
	}

	return result, nil
}

// List server with service
// @Summary List server with service
// @Security BearerAuth
// @Schemes
// @Description List servers that have service monitoring data
// @Tags common
// @Produce json
// @Success 200 {object} model.CommonResponse[[]uint64]
// @Router /service/server [get]
func listServerWithServices(c *gin.Context) ([]uint64, error) {
	services := singleton.ServiceSentinelShared.GetList()
	serverMap := singleton.ServerShared.GetList()

	serverIDSet := make(map[uint64]bool)

	for _, service := range services {
		if service.Cover == model.ServiceCoverAll {
			for serverID := range serverMap {
				if !service.SkipServers[serverID] {
					serverIDSet[serverID] = true
				}
			}
		} else {
			for serverID, enabled := range service.SkipServers {
				if enabled {
					serverIDSet[serverID] = true
				}
			}
		}
	}

	var ret []uint64
	for id := range serverIDSet {
		server, ok := serverMap[id]
		if !ok || server == nil {
			continue
		}
		if userCanViewServer(c, server) {
			ret = append(ret, id)
		}
	}

	return ret, nil
}

// Create service
// @Summary Create service
// @Security BearerAuth
// @Schemes
// @Description Create service
// @Tags auth required
// @Accept json
// @param request body model.ServiceForm true "Service Request"
// @Produce json
// @Success 200 {object} model.CommonResponse[uint64]
// @Router /service [post]
func createService(c *gin.Context) (uint64, error) {
	var mf model.ServiceForm
	if err := c.ShouldBindJSON(&mf); err != nil {
		return 0, err
	}

	if !isValidServiceCover(mf.Cover) {
		return 0, singleton.Localizer.ErrorT("permission denied")
	}

	uid := getUid(c)

	var m model.Service
	m.UserID = uid
	m.Name = mf.Name
	m.Target = strings.TrimSpace(mf.Target)
	m.Type = mf.Type
	m.SkipServers = mf.SkipServers
	m.Cover = mf.Cover
	m.DisplayIndex = mf.DisplayIndex
	m.Notify = mf.Notify
	m.NotificationGroupID = mf.NotificationGroupID
	m.Duration = mf.Duration
	m.LatencyNotify = mf.LatencyNotify
	m.MinLatency = mf.MinLatency
	m.MaxLatency = mf.MaxLatency
	m.HideForGuest = mf.HideForGuest

	if err := singleton.DB.Create(&m).Error; err != nil {
		return 0, newGormError("%v", err)
	}

	if err := singleton.ServiceSentinelShared.Update(&m); err != nil {
		return 0, err
	}

	singleton.ServiceSentinelShared.UpdateServiceList()
	return m.ID, nil
}

// Update service
// @Summary Update service
// @Security BearerAuth
// @Schemes
// @Description Update service
// @Tags auth required
// @Accept json
// @param id path uint true "Service ID"
// @param request body model.ServiceForm true "Service Request"
// @Produce json
// @Success 200 {object} model.CommonResponse[any]
// @Router /service/{id} [patch]
func updateService(c *gin.Context) (any, error) {
	strID := c.Param("id")
	id, err := strconv.ParseUint(strID, 10, 64)
	if err != nil {
		return nil, err
	}
	var mf model.ServiceForm
	if err := c.ShouldBindJSON(&mf); err != nil {
		return nil, err
	}

	if !isValidServiceCover(mf.Cover) {
		return nil, singleton.Localizer.ErrorT("permission denied")
	}

	var m model.Service
	if err := singleton.DB.First(&m, id).Error; err != nil {
		return nil, singleton.Localizer.ErrorT("service id %d does not exist", id)
	}

	if !m.HasPermission(c) {
		return nil, singleton.Localizer.ErrorT("permission denied")
	}

	m.Name = mf.Name
	m.Target = strings.TrimSpace(mf.Target)
	m.Type = mf.Type
	m.SkipServers = mf.SkipServers
	m.Cover = mf.Cover
	m.DisplayIndex = mf.DisplayIndex
	m.Notify = mf.Notify
	m.NotificationGroupID = mf.NotificationGroupID
	m.Duration = mf.Duration
	m.LatencyNotify = mf.LatencyNotify
	m.MinLatency = mf.MinLatency
	m.MaxLatency = mf.MaxLatency
	m.HideForGuest = mf.HideForGuest

	if err := singleton.DB.Save(&m).Error; err != nil {
		return nil, newGormError("%v", err)
	}

	if err := singleton.ServiceSentinelShared.Update(&m); err != nil {
		return nil, err
	}

	singleton.ServiceSentinelShared.UpdateServiceList()
	return nil, nil
}

// Batch delete service
// @Summary Batch delete service
// @Security BearerAuth
// @Schemes
// @Description Batch delete service
// @Tags auth required
// @Accept json
// @param request body []uint true "id list"
// @Produce json
// @Success 200 {object} model.CommonResponse[any]
// @Router /batch-delete/service [post]
func batchDeleteService(c *gin.Context) (any, error) {
	var ids []uint64
	if err := c.ShouldBindJSON(&ids); err != nil {
		return nil, err
	}

	if !singleton.ServiceSentinelShared.CheckPermission(c, slices.Values(ids)) {
		return nil, singleton.Localizer.ErrorT("permission denied")
	}

	err := singleton.DB.Transaction(func(tx *gorm.DB) error {
		return tx.Unscoped().Delete(&model.Service{}, "id in (?)", ids).Error
	})
	if err != nil {
		return nil, err
	}
	singleton.ServiceSentinelShared.Delete(ids)
	singleton.ServiceSentinelShared.UpdateServiceList()
	return nil, nil
}

func isValidServiceCover(cover uint8) bool {
	return cover == model.ServiceCoverAll || cover == model.ServiceCoverIgnoreAll
}
