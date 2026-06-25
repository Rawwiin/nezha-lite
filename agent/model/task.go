package model

const (
	_ = iota
	TaskTypeHTTPGet
	TaskTypeICMPPing
	TaskTypeTCPPing
	TaskTypeUpgrade
	TaskTypeKeepalive
	TaskTypeReportConfig
	TaskTypeApplyConfig
	TaskTypeServerTransferApply
)
