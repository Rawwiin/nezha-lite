package tsdb

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/storage"
)

// QueryPeriod 查询时间段
type QueryPeriod string

const (
	Period1Day   QueryPeriod = "1d"
	Period7Days  QueryPeriod = "7d"
	Period30Days QueryPeriod = "30d"
)

// ParseQueryPeriod 解析查询时间段
func ParseQueryPeriod(s string) (QueryPeriod, error) {
	switch s {
	case "1d", "":
		return Period1Day, nil
	case "7d":
		return Period7Days, nil
	case "30d":
		return Period30Days, nil
	default:
		return "", fmt.Errorf("invalid period: %s, expected 1d, 7d, or 30d", s)
	}
}

// Duration 返回时间段的时长
func (p QueryPeriod) Duration() time.Duration {
	switch p {
	case Period7Days:
		return 7 * 24 * time.Hour
	case Period30Days:
		return 30 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// DownsampleInterval 返回降采样间隔
// 1d: 30秒一个点 (2880个点)
// 7d: 30分钟一个点 (336个点)
// 30d: 2小时一个点 (360个点)
func (p QueryPeriod) DownsampleInterval() time.Duration {
	switch p {
	case Period7Days:
		return 30 * time.Minute
	case Period30Days:
		return 2 * time.Hour
	default:
		return 30 * time.Second
	}
}

// DataPoint 服务历史数据点（保留供内部计算使用）
type DataPoint struct {
	Timestamp int64
	Delay     float64
	Status    uint8
}

// MetricDataPoint 服务器监控指标数据点
type MetricDataPoint struct {
	Timestamp int64
	Value     float64
}

// ServiceHistorySummary 服务历史统计摘要（保留供内部计算使用）
type ServiceHistorySummary struct {
	TotalUp   uint64
	TotalDown uint64
	UpPercent float32
	AvgDelay  float64
	DataPoints []DataPoint
}

type rawDataPoint struct {
	timestamp int64
	value     float64
	status    float64
	hasDelay  bool
	hasStatus bool
}


func calculateStats(points []rawDataPoint, downsampleInterval time.Duration) ServiceHistorySummary {
	if len(points) == 0 {
		return ServiceHistorySummary{}
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].timestamp < points[j].timestamp
	})

	var totalDelay float64
	var delayCount int
	var totalUp, totalDown uint64

	for _, p := range points {
		if p.hasDelay {
			totalDelay += p.value
			delayCount++
		}
		if p.hasStatus {
			if p.status >= 0.5 {
				totalUp++
			} else {
				totalDown++
			}
		}
	}

	summary := ServiceHistorySummary{
		TotalUp:   totalUp,
		TotalDown: totalDown,
	}

	if delayCount > 0 {
		summary.AvgDelay = totalDelay / float64(delayCount)
	}

	if totalUp+totalDown > 0 {
		summary.UpPercent = float32(totalUp) / float32(totalUp+totalDown) * 100
	}

	summary.DataPoints = downsample(points, downsampleInterval)

	return summary
}

func downsample(points []rawDataPoint, interval time.Duration) []DataPoint {
	if len(points) == 0 {
		return nil
	}

	intervalMs := interval.Milliseconds()
	result := make([]DataPoint, 0)

	// points 已排序，线性扫描分桶
	bucketStart := (points[0].timestamp / intervalMs) * intervalMs
	var totalDelay float64
	var delayCount, upCount, statusCount int

	flushBucket := func() {
		var avgDelay float64
		if delayCount > 0 {
			avgDelay = totalDelay / float64(delayCount)
		}
		var status uint8
		if statusCount > 0 && upCount > statusCount/2 {
			status = 1
		}
		result = append(result, DataPoint{
			Timestamp: bucketStart,
			Delay:     avgDelay,
			Status:    status,
		})
	}

	for _, p := range points {
		key := (p.timestamp / intervalMs) * intervalMs
		if key != bucketStart {
			flushBucket()
			bucketStart = key
			totalDelay = 0
			delayCount = 0
			upCount = 0
			statusCount = 0
		}
		if p.hasDelay {
			totalDelay += p.value
			delayCount++
		}
		if p.hasStatus {
			statusCount++
			if p.status >= 0.5 {
				upCount++
			}
		}
	}
	flushBucket()

	return result
}

func downsampleMetrics(points []rawDataPoint, interval time.Duration, useLastValue bool) []MetricDataPoint {
	if len(points) == 0 {
		return nil
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].timestamp < points[j].timestamp
	})

	intervalMs := interval.Milliseconds()
	result := make([]MetricDataPoint, 0)

	bucketStart := (points[0].timestamp / intervalMs) * intervalMs
	var total float64
	var count int
	var last rawDataPoint

	flushBucket := func() {
		var value float64
		if useLastValue {
			value = last.value
		} else if count > 0 {
			value = total / float64(count)
		}
		result = append(result, MetricDataPoint{
			Timestamp: bucketStart,
			Value:     value,
		})
	}

	for _, p := range points {
		key := (p.timestamp / intervalMs) * intervalMs
		if key != bucketStart {
			flushBucket()
			bucketStart = key
			total = 0
			count = 0
		}
		total += p.value
		count++
		last = p
	}
	flushBucket()

	return result
}

// isCumulativeMetric 判断指标是否为累积型（单调递增）
func isCumulativeMetric(metric MetricType) bool {
	switch metric {
	case MetricServerNetInTransfer, MetricServerNetOutTransfer, MetricServerUptime:
		return true
	default:
		return false
	}
}

func (db *TSDB) QueryServerMetrics(serverID uint64, metric MetricType, period QueryPeriod) ([]MetricDataPoint, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return nil, fmt.Errorf("TSDB is closed")
	}

	now := time.Now()
	tr := storage.TimeRange{
		MinTimestamp: now.Add(-period.Duration()).UnixMilli(),
		MaxTimestamp: now.UnixMilli(),
	}

	serverIDStr := strconv.FormatUint(serverID, 10)

	tfs := storage.NewTagFilters()
	if err := tfs.Add(nil, []byte(metric), false, false); err != nil {
		return nil, err
	}
	if err := tfs.Add([]byte("server_id"), []byte(serverIDStr), false, false); err != nil {
		return nil, err
	}

	deadline := uint64(time.Now().Add(30 * time.Second).Unix())

	var search storage.Search
	search.Init(nil, db.storage, []*storage.TagFilters{tfs}, tr, 100000, deadline)
	defer search.MustClose()

	var points []rawDataPoint
	var timestamps []int64
	var values []float64

	for search.NextMetricBlock() {
		mbr := search.MetricBlockRef
		var block storage.Block
		mbr.BlockRef.MustReadBlock(&block)

		if err := block.UnmarshalData(); err != nil {
			log.Printf("NEZHA>> TSDB: failed to unmarshal block data: %v", err)
			continue
		}

		timestamps = timestamps[:0]
		values = values[:0]
		timestamps, values = block.AppendRowsWithTimeRangeFilter(timestamps, values, tr)

		for i := range timestamps {
			points = append(points, rawDataPoint{
				timestamp: timestamps[i],
				value:     values[i],
			})
		}
	}

	if err := search.Error(); err != nil {
		return nil, err
	}

	return downsampleMetrics(points, period.DownsampleInterval(), isCumulativeMetric(metric)), nil
}

