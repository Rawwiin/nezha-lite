package commands

import (
	"os"

	"github.com/nezhahq/service"
)

// Program 封装 service.Service 的生命周期，由 runService 创建并注入
type Program struct {
	Exit    chan struct{}   // 退出信号通道
	Service service.Service // 底层系统服务实例
	Run     func()          // Agent 主循环入口
}

// Start 服务启动时回调，在独立 goroutine 中执行 Run
func (p *Program) Start(s service.Service) error {
	go p.run()
	return nil
}

// Stop 服务停止时回调，关闭 Exit 通道通知主循环退出
func (p *Program) Stop(s service.Service) error {
	close(p.Exit)
	if service.Interactive() {
		os.Exit(0)
	}
	return nil
}

// run 执行 Agent 主循环，退出时根据运行模式选择停止方式
func (p *Program) run() {
	defer func() {
		if service.Interactive() {
			p.Stop(p.Service)
		} else {
			p.Service.Stop()
		}
	}()
	p.Run()
}
