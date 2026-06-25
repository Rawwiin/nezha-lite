package model

// 任务类型常量（与 agent model 保持一致）
const (
	_ = iota
	TaskTypeHTTPGet
	TaskTypeICMPPing
	TaskTypeTCPPing
	TaskTypeCommand
	TaskTypeUpgrade
	TaskTypeKeepalive
	TaskTypeReportConfig
	TaskTypeApplyConfig
	TaskTypeServerTransferApply
)
