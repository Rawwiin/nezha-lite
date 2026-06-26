// Modified by Nezha Lite for simplified agent functionality.
// Original source: https://github.com/nezhahq/nezha
// Licensed under the Apache License, Version 2.0

package model

// 任务类型常量（与 dashboard model 及原版保持一致）
// 精简版仅处理延迟检测和心跳保活，但常量值必须与原版完全一致，
// 否则跨版本通信时会导致任务类型错位（如 Keepalive 被解释为 Command）。
// 已移除的任务类型保留占位，确保 iota 序列不变。
const (
	_                                 = iota // 0
	TaskTypeHTTPGet                          // 1: HTTP 可用性探测
	TaskTypeICMPPing                         // 2: ICMP 连通性探测
	TaskTypeTCPPing                          // 3: TCP 端口探测
	TaskTypeCommand_                         // 4: 命令执行（已移除，保留占位）
	TaskTypeTerminal_                        // 5: 终端（已移除，保留占位）
	TaskTypeUpgrade_                         // 6: 升级（已移除，保留占位）
	TaskTypeKeepalive                        // 7: 心跳保活
	TaskTypeTerminalGRPC_                    // 8: gRPC 终端（已移除，保留占位）
	TaskTypeNAT_                             // 9: NAT 穿透（已移除，保留占位）
	TaskTypeReportHostInfoDeprecated_        // 10: 旧版主机信息上报（已移除，保留占位）
	TaskTypeFM_                              // 11: 文件管理（已移除，保留占位）
	TaskTypeReportConfig_                    // 12: 配置上报（已移除，保留占位）
	TaskTypeApplyConfig_                     // 13: 配置下发（已移除，保留占位）
	TaskTypeServerTransferApply_             // 14: 服务器转移（已移除，保留占位）
	TaskTypeExec_                            // 15: MCP 命令执行（已移除，保留占位）
	TaskTypeFsList_                          // 16: MCP 文件列表（已移除，保留占位）
	TaskTypeFsRead_                          // 17: MCP 文件读取（已移除，保留占位）
	TaskTypeFsWrite_                         // 18: MCP 文件写入（已移除，保留占位）
	TaskTypeFsDelete_                        // 19: MCP 文件删除（已移除，保留占位）
	TaskTypeFsTransfer_                      // 20: MCP 文件传输（已移除，保留占位）
)
