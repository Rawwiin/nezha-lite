//go:build darwin

// Modified by Nezha Lite for simplified agent functionality.
// Original source: https://github.com/nezhahq/nezha
// Licensed under the Apache License, Version 2.0

package logger

import (
	"github.com/nezhahq/service"
)

// serviceLogger 包装 service.Logger，macOS 下将 Info 级别降级为 Warning
// （macOS 的 launchd 默认忽略 Info 级别日志）
type serviceLogger struct {
	service.Logger
}

// Info macOS 下降级为 Warning，确保日志可见
func (s *serviceLogger) Info(v ...any) error {
	return s.Warning(v...)
}

// Infof macOS 下降级为 Warningf，确保日志可见
func (s *serviceLogger) Infof(format string, v ...any) error {
	return s.Warningf(format, v...)
}

// NewNezhaServiceLogger 创建系统服务日志记录器（macOS 平台，Info 降级为 Warning）
func NewNezhaServiceLogger(s service.Service, errs chan<- error) (service.Logger, error) {
	logger, err := s.Logger(errs)
	if err != nil {
		return nil, err
	}

	return &serviceLogger{logger}, nil
}
