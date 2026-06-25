package model

// ServerMetricsDataPoint 服务器监控指标数据点
type ServerMetricsDataPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// ServerMetricsResponse 服务器监控指标响应
type ServerMetricsResponse struct {
	ServerID   uint64                   `json:"server_id"`
	ServerName string                   `json:"server_name"`
	Metric     string                   `json:"metric"`
	DataPoints []ServerMetricsDataPoint `json:"data_points"`
}
