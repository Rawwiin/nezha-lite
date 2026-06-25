package model

// 任务类型常量（与 dashboard model 保持一致）
// 精简版仅保留延迟检测和心跳保活四种任务类型
const (
	_                 = iota
	TaskTypeHTTPGet   // HTTP 可用性探测
	TaskTypeICMPPing  // ICMP 连通性探测
	TaskTypeTCPPing   // TCP 端口探测
	TaskTypeKeepalive // 心跳保活
)
