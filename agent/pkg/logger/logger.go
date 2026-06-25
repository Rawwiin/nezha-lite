package logger

import (
	"fmt"
	"sync"
	"time"

	"github.com/nezhahq/service"
)

var (
	// defaultLogger 默认使用 ConsoleLogger，runService 时通过 InitDefaultLogger 替换为系统服务日志
	defaultLogger = &ServiceLogger{enabled: true, logger: service.ConsoleLogger}

	loggerOnce sync.Once
)

// ServiceLogger 封装 service.Logger，支持开关控制日志输出
type ServiceLogger struct {
	enabled bool
	logger  service.Logger
}

// InitDefaultLogger 初始化默认日志记录器的后端（仅首次调用生效）
// runService 时用此函数将 ConsoleLogger 替换为系统服务日志
func InitDefaultLogger(enabled bool, logger service.Logger) {
	loggerOnce.Do(func() {
		defaultLogger.enabled = enabled
		defaultLogger.logger = logger
	})
}

// SetEnable 设置日志开关，可多次调用
func SetEnable(enable bool) {
	defaultLogger.enabled = enable
}

// Println 打印日志
func Println(v ...interface{}) {
	defaultLogger.Println(v...)
}

// Printf 格式化打印日志
func Printf(format string, v ...interface{}) {
	defaultLogger.Printf(format, v...)
}

func (s *ServiceLogger) Println(v ...interface{}) {
	if s.enabled {
		s.logger.Infof("NEZHA@%s>> %v", time.Now().Format(time.DateTime), fmt.Sprint(v...))
	}
}

func (s *ServiceLogger) Printf(format string, v ...interface{}) {
	if s.enabled {
		s.logger.Infof("NEZHA@%s>> "+format, append([]interface{}{time.Now().Format(time.DateTime)}, v...)...)
	}
}
