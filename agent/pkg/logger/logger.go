package logger

import (
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	defaultLogger = &ServiceLogger{enabled: true}

	loggerOnce sync.Once
)

type ServiceLogger struct {
	enabled bool
}

// InitDefaultLogger 初始化默认日志器
func InitDefaultLogger(enabled bool, _ interface{}) {
	loggerOnce.Do(func() {
		defaultLogger.enabled = enabled
	})
}

// SetEnable 设置日志开关
func SetEnable(enable bool) {
	defaultLogger.SetEnable(enable)
}

// Println 打印日志
func Println(v ...interface{}) {
	defaultLogger.Println(v...)
}

// Printf 格式化打印日志
func Printf(format string, v ...interface{}) {
	defaultLogger.Printf(format, v...)
}

// Error 打印错误日志
func Error(v ...interface{}) error {
	return defaultLogger.Error(v...)
}

// Errorf 格式化打印错误日志
func Errorf(format string, v ...interface{}) error {
	return defaultLogger.Errorf(format, v...)
}

// NewServiceLogger 创建新的日志器
func NewServiceLogger(enable bool, _ interface{}) *ServiceLogger {
	return &ServiceLogger{
		enabled: enable,
	}
}

func (s *ServiceLogger) SetEnable(enable bool) {
	s.enabled = enable
}

func (s *ServiceLogger) Println(v ...interface{}) {
	if s.enabled {
		log.Printf("NEZHA@%s>> %v", time.Now().Format(time.DateTime), fmt.Sprint(v...))
	}
}

func (s *ServiceLogger) Printf(format string, v ...interface{}) {
	if s.enabled {
		log.Printf("NEZHA@%s>> "+format, append([]interface{}{time.Now().Format(time.DateTime)}, v...)...)
	}
}

func (s *ServiceLogger) Error(v ...interface{}) error {
	if s.enabled {
		return fmt.Errorf("NEZHA@%s>> %v", time.Now().Format(time.DateTime), fmt.Sprint(v...))
	}
	return nil
}

func (s *ServiceLogger) Errorf(format string, v ...interface{}) error {
	if s.enabled {
		return fmt.Errorf("NEZHA@%s>> "+format, append([]interface{}{time.Now().Format(time.DateTime)}, v...)...)
	}
	return nil
}
