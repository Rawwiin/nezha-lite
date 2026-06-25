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

// SetEnable 设置日志开关（仅首次调用生效，由 agent 启动时根据 debug 配置初始化）
func SetEnable(enable bool) {
	loggerOnce.Do(func() {
		defaultLogger.enabled = enable
	})
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
		log.Printf("NEZHA@%s>> %v", time.Now().Format(time.DateTime), fmt.Sprint(v...))
	}
}

func (s *ServiceLogger) Printf(format string, v ...interface{}) {
	if s.enabled {
		log.Printf("NEZHA@%s>> "+format, append([]interface{}{time.Now().Format(time.DateTime)}, v...)...)
	}
}
