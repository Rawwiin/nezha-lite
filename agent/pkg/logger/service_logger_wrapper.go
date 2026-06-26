//go:build !darwin

// Modified by Nezha Lite for simplified agent functionality.
// Original source: https://github.com/nezhahq/nezha
// Licensed under the Apache License, Version 2.0

package logger

import (
	"github.com/nezhahq/service"
)

// NewNezhaServiceLogger 创建系统服务日志记录器（非 macOS 平台）
func NewNezhaServiceLogger(s service.Service, errs chan<- error) (service.Logger, error) {
	return s.Logger(errs)
}
