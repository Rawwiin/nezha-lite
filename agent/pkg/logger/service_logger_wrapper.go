//go:build !darwin

package logger

import (
	"github.com/nezhahq/service"
)

// NewNezhaServiceLogger 创建系统服务日志记录器（非 macOS 平台）
func NewNezhaServiceLogger(s service.Service, errs chan<- error) (service.Logger, error) {
	return s.Logger(errs)
}
