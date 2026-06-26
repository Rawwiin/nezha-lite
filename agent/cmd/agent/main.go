// Modified by Nezha Lite for simplified agent functionality.
// Original source: https://github.com/nezhahq/nezha
// Licensed under the Apache License, Version 2.0

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	ping "github.com/prometheus-community/pro-bing"
	utls "github.com/refraction-networking/utls"
	"github.com/shirou/gopsutil/v4/host"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"

	"github.com/nezhahq/agent/cmd/agent/commands"
	"github.com/nezhahq/agent/model"
	"github.com/nezhahq/agent/pkg/logger"
	"github.com/nezhahq/agent/pkg/monitor"
	"github.com/nezhahq/agent/pkg/util"
	utlsx "github.com/nezhahq/agent/pkg/utls"
	pb "github.com/nezhahq/agent/proto"
	"github.com/nezhahq/service"
)

var (
	version               = monitor.Version // 构建时注入的版本号
	executablePath        string            // 可执行文件路径，runService 用于生成服务名
	defaultConfigPath     = loadDefaultConfigPath()
	agentConfig           model.AgentConfig
	client                pb.NezhaServiceClient
	initialized           bool
	prevDashboardBootTime uint64
	geoipReported         bool
	lastReportHostInfo    time.Time
	lastReportIPInfo      time.Time

	hostStatus atomic.Bool
	ipStatus   atomic.Bool

	// icmpSem 限制同时执行的 ICMP Ping 任务数，防止高频下发导致 Agent 成为 DDoS 放大器
	icmpSem = make(chan struct{}, 3)

	httpClient = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: time.Second * 30,
	}
)

var (
	println = logger.Println
	printf  = logger.Printf
)

const (
	delayWhenError = time.Second * 10 // Agent 重连间隔
	networkTimeOut = time.Second * 5  // 普通网络超时
)

// 设置环境：DNS 解析器、HTTP 传输层（utls 指纹模拟）
func setEnv() {
	resolver.SetDefaultScheme("passthrough")
	net.DefaultResolver.PreferGo = true // 使用 Go 内置 DNS 解析器
	net.DefaultResolver.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: time.Second * 5}
		dnsServers := util.DNSServersAll
		if len(agentConfig.DNS) > 0 {
			dnsServers = agentConfig.DNS
		}
		var conn net.Conn
		var err error
		for _, server := range util.RangeRnd(dnsServers) {
			conn, err = d.DialContext(ctx, "udp", server)
			if err == nil {
				return conn, nil
			}
		}
		return nil, err
	}
	headers := util.BrowserHeaders()
	http.DefaultClient.Timeout = time.Second * 30
	httpClient.Transport = utlsx.NewUTLSHTTPRoundTripperWithProxy(
		utls.HelloChrome_Auto, new(utls.Config),
		http.DefaultTransport, nil, headers,
	)
}

func loadDefaultConfigPath() string {
	var err error
	executablePath, err = os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(executablePath), "config.yml")
}

// preRun 初始化环境并读取配置文件
func preRun(configPath string) error {
	setEnv()

	if configPath == "" {
		configPath = defaultConfigPath
	}

	// windows 环境检查架构匹配
	if runtime.GOOS == "windows" {
		hostArch, err := host.KernelArch()
		if err != nil {
			return err
		}
		switch hostArch {
		case "i386", "i686":
			hostArch = "386"
		case "x86_64":
			hostArch = "amd64"
		case "aarch64":
			hostArch = "arm64"
		}
		if runtime.GOARCH != hostArch {
			return fmt.Errorf("与当前系统不匹配，当前运行 %s_%s, 需要下载 %s_%s", runtime.GOOS, runtime.GOARCH, runtime.GOOS, hostArch)
		}
	}

	if err := agentConfig.Read(configPath); err != nil {
		return fmt.Errorf("init config failed: %v", err)
	}

	// 根据配置初始化日志开关：debug=false 时静默运行，debug=true（默认）输出全部日志
	logger.SetEnable(agentConfig.Debug)

	// 初始化 monitor 配置
	monitor.InitConfig(&agentConfig)
	monitor.CustomEndpoints = agentConfig.CustomIPApi

	return nil
}

func main() {
	// 子命令模式：nezha-agent service <install/uninstall/start/stop/restart>
	if len(os.Args) >= 2 && os.Args[1] == "service" {
		serviceCmd := flag.NewFlagSet("service", flag.ExitOnError)
		serviceConfig := serviceCmd.String("c", "", "配置文件路径")
		serviceCmd.Parse(os.Args[2:])

		action := serviceCmd.Arg(0)
		if action == "" {
			log.Fatal("必须指定一个参数: install/uninstall/start/stop/restart")
		}

		// 确定配置文件路径
		configPath := *serviceConfig
		if configPath == "" {
			configPath = defaultConfigPath
		}
		ap, _ := filepath.Abs(configPath)

		if err := preRun(ap); err != nil {
			log.Fatal(err)
		}
		runService(action, ap)
		return
	}

	// 普通模式：nezha-agent [-c config.yml]
	configPath := ""
	flag.StringVar(&configPath, "c", "", "配置文件路径")
	flag.Parse()

	if err := preRun(configPath); err != nil {
		log.Fatal(err)
	}

	// 通过 service 框架运行（空 action = 前台运行）
	runService("", "")
}

// runService 通过 service 框架管理 Agent 的系统服务生命周期
// action 为空时以前台模式运行；非空时执行 install/uninstall/start/stop/restart
func runService(action string, path string) {
	// Windows 服务失败后自动重启
	winConfig := map[string]interface{}{
		"OnFailure": "restart",
	}

	args := []string{"-c", path}
	name := filepath.Base(executablePath)
	// 非默认配置路径时，在服务名后追加路径哈希以区分多实例
	if path != defaultConfigPath && path != "" {
		hex := util.MD5Sum(path)[:7]
		name = fmt.Sprintf("%s-%s", name, hex)
	}

	svcConfig := &service.Config{
		Name:             name,
		DisplayName:      filepath.Base(executablePath),
		Arguments:        args,
		Description:      "哪吒监控 Agent",
		WorkingDirectory: filepath.Dir(executablePath),
		Option:           winConfig,
	}

	prg := &commands.Program{
		Exit: make(chan struct{}),
		Run:  run,
	}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		// service 框架不可用时退化为普通模式运行
		printf("创建服务时出错，以普通模式运行: %v", err)
		run()
		return
	}
	prg.Service = s

	// 尝试使用系统服务日志，失败时退化为控制台日志
	serviceLogger, err := logger.NewNezhaServiceLogger(s, nil)
	if err != nil {
		printf("获取 service logger 时出错: %+v", err)
		logger.InitDefaultLogger(agentConfig.Debug, service.ConsoleLogger)
	} else {
		logger.InitDefaultLogger(agentConfig.Debug, serviceLogger)
	}

	// install 时提前读取配置并打印 init 系统类型
	if action == "install" {
		initName := s.Platform()
		if err := agentConfig.Read(path); err != nil {
			log.Fatalf("init config failed: %v", err)
		}
		printf("Init system is: %s", initName)
	}

	// 非空 action 执行服务控制命令后退出
	if len(action) != 0 {
		err := service.Control(s, action)
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	// 空 action：前台运行服务
	err = s.Run()
	if err != nil {
		printf("服务运行出错: %v", err)
	}
}

// run 主循环：与 Dashboard 建立 gRPC 连接，上报状态并接收/执行延迟检测任务
func run() {
	auth := model.AuthHandler{
		Credentials: func() (string, string) {
			return agentConfig.ClientSecret, agentConfig.UUID
		},
		RequireTLS: func() bool {
			return agentConfig.TLS
		},
	}

	var conn *grpc.ClientConn
	retry := func() {
		initialized = false
		if conn != nil {
			conn.Close()
		}
		time.Sleep(delayWhenError)
		println("Try to reconnect ...")
	}

	for {
		var securityOption grpc.DialOption
		if agentConfig.TLS {
			if agentConfig.InsecureTLS {
				securityOption = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}))
			} else {
				securityOption = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12}))
			}
		} else {
			securityOption = grpc.WithTransportCredentials(insecure.NewCredentials())
		}
		var err error
		conn, err = grpc.NewClient(agentConfig.Server, securityOption, grpc.WithPerRPCCredentials(&auth))
		if err != nil {
			printf("与面板建立连接失败: %v", err)
			retry()
			continue
		}
		client = pb.NewNezhaServiceClient(conn)
		printf("Connection to %s established", agentConfig.Server)

		// 上报系统信息，让 Dashboard 初始化节点
		timeOutCtx, cancel := context.WithTimeout(context.Background(), networkTimeOut)
		dashboardBootTimeReceipt, err := client.ReportSystemInfo2(timeOutCtx, monitor.GetHost().PB())
		cancel()
		if err != nil {
			printf("上报系统信息失败: %v", err)
			retry()
			continue
		}

		geoipReported = geoipReported && prevDashboardBootTime > 0 && dashboardBootTimeReceipt.GetData() == prevDashboardBootTime
		prevDashboardBootTime = dashboardBootTimeReceipt.GetData()
		initialized = true

		wCtx, wCancel := context.WithCancel(context.Background())

		// 建立状态上报流（心跳+状态）
		reportState, err := doWithTimeout(func() (pb.NezhaService_ReportSystemStateClient, error) {
			return client.ReportSystemState(wCtx)
		}, networkTimeOut)
		if err != nil {
			printf("上报状态信息失败: %v", err)
			wCancel()
			retry()
			continue
		}
		go reportStateDaemon(reportState, wCancel)

		tasks, err := doWithTimeout(func() (pb.NezhaService_RequestTaskClient, error) {
			return client.RequestTask(wCtx)
		}, networkTimeOut)
		if err != nil {
			printf("请求任务失败: %v", err)
			wCancel()
			retry()
			continue
		}
		go receiveTasksDaemon(tasks, wCancel)

		select {
		case <-wCtx.Done():
			println("Worker exit...")
		}

		retry()
	}
}

// doWithTimeout 带超时的函数包装器。
// 仅用于 gRPC stream 操作（Send/Recv）——这些方法不接受 context 参数，
// 只能通过取消 stream 的 context 来解除阻塞。超时后内部 goroutine 会短暂泄漏，
// 直到调用方收到错误后取消 stream context（wCancel），goroutine 才会退出。
// gRPC unary 调用应直接使用 context.WithTimeout，不要用此函数。
func doWithTimeout[T any](fn func() (T, error), timeout time.Duration) (T, error) {
	var result T
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	type res struct {
		val T
		err error
	}
	ch := make(chan res, 1)

	go func() {
		val, err := fn()
		ch <- res{val, err}
	}()

	select {
	case <-ctx.Done():
		return result, ctx.Err()
	case r := <-ch:
		return r.val, r.err
	}
}

// reportStateDaemon 向 Dashboard 定期上报系统状态
func reportStateDaemon(stateClient pb.NezhaService_ReportSystemStateClient, cancel context.CancelFunc) {
	var err error
	for {
		lastReportHostInfo, lastReportIPInfo, err = reportState(stateClient, lastReportHostInfo, lastReportIPInfo)
		if err != nil {
			printf("reportStateDaemon exit: %v", err)
			cancel()
			return
		}
		time.Sleep(time.Second * time.Duration(agentConfig.ReportDelay))
	}
}

// reportState 发送一次状态上报，包含网络速度计算、硬件信息刷新、IP 上报
func reportState(statClient pb.NezhaService_ReportSystemStateClient, host, ip time.Time) (time.Time, time.Time, error) {
	if statClient.Context().Err() != nil {
		return host, ip, statClient.Context().Err()
	}
	if initialized {
		monitor.TrackNetworkSpeed()
		if _, err := doWithTimeout(func() (*pb.Receipt, error) {
			return nil, statClient.Send(monitor.GetState(agentConfig.SkipConnectionCount, agentConfig.SkipProcsCount).PB())
		}, time.Second*10); err != nil {
			return host, ip, err
		}
		_, err := doWithTimeout(statClient.Recv, time.Second*10)
		if err != nil {
			return host, ip, err
		}
	}
	// 每 10 分钟重新获取一次硬件信息
	if host.Before(time.Now().Add(-10 * time.Minute)) {
		if reportHost() {
			host = time.Now()
		}
	}
	// 更新 IP 信息
	if time.Since(ip) > time.Second*time.Duration(agentConfig.IPReportPeriod) || !geoipReported {
		if reportGeoIP(agentConfig.UseIPv6CountryCode, !geoipReported) {
			ip = time.Now()
			geoipReported = true
		}
	}
	return host, ip, nil
}

func reportHost() bool {
	if !hostStatus.CompareAndSwap(false, true) {
		return false
	}
	defer hostStatus.Store(false)
	if client != nil && initialized {
		// 使用带超时的 context 直接调用 gRPC unary API，
		// 避免旧实现中 doWithTimeout+context.Background() 导致超时后 goroutine 永久泄漏
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		receipt, err := client.ReportSystemInfo2(ctx, monitor.GetHost().PB())
		if err != nil {
			printf("ReportSystemInfo2 error: %v", err)
			return false
		}
		geoipReported = geoipReported && prevDashboardBootTime > 0 && receipt.GetData() == prevDashboardBootTime
	}
	return true
}

func reportGeoIP(use6, forceUpdate bool) bool {
	if !ipStatus.CompareAndSwap(false, true) {
		return false
	}
	defer ipStatus.Store(false)

	if client == nil || !initialized {
		return false
	}

	pbg := monitor.FetchIP(use6)
	if pbg == nil {
		return false
	}

	if !monitor.GeoQueryIPChanged && !forceUpdate {
		return true
	}

	// 使用带超时的 context 直接调用 gRPC unary API，
	// 避免旧实现中 doWithTimeout+context.Background() 导致超时后 goroutine 永久泄漏
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	geoip, err := client.ReportGeoIP(ctx, pbg)
	if err != nil {
		return false
	}

	prevDashboardBootTime = geoip.GetDashboardBootTime()
	monitor.CachedCountryCode = geoip.GetCountryCode()
	monitor.GeoQueryIPChanged = false

	return true
}

// newSerialTaskResultSender 包装 RequestTask 流的 Send，防止并发 Send 导致流损坏
func newSerialTaskResultSender(send func(*pb.TaskResult) error) func(*pb.TaskResult) error {
	var mu sync.Mutex
	return func(r *pb.TaskResult) error {
		mu.Lock()
		defer mu.Unlock()
		return send(r)
	}
}

// receiveTasksDaemon 持续接收 Dashboard 下发的任务
func receiveTasksDaemon(tasks pb.NezhaService_RequestTaskClient, cancel context.CancelFunc) {
	var task *pb.Task
	var err error
	send := newSerialTaskResultSender(tasks.Send)
	for {
		task, err = doWithTimeout(func() (*pb.Task, error) {
			return tasks.Recv()
		}, time.Second*30)
		if err != nil {
			printf("receiveTasks exit: %v", err)
			cancel()
			return
		}
		go runAgentTask(task, send, cancel)
	}
}

// runAgentTask 在 goroutine 中执行单个任务并发送结果
func runAgentTask(task *pb.Task, send func(*pb.TaskResult) error, cancel context.CancelFunc) {
	defer func() {
		if err := recover(); err != nil {
			println("task panic", task, err)
		}
	}()
	result := doTask(task)
	if result == nil {
		return
	}
	if err := send(result); err != nil {
		printf("send task result exit: %v", err)
		cancel()
	}
}

// doTask 根据任务类型分发到对应的处理器
func doTask(task *pb.Task) *pb.TaskResult {
	var result pb.TaskResult
	result.Id = task.GetId()
	result.Type = task.GetType()
	switch task.GetType() {
	case model.TaskTypeHTTPGet:
		handleHttpGetTask(task, &result)
	case model.TaskTypeICMPPing:
		handleIcmpPingTask(task, &result)
	case model.TaskTypeTCPPing:
		handleTcpPingTask(task, &result)
	case model.TaskTypeKeepalive:
		// Keepalive 任务静默返回成功，不产生噪音日志
		result.Successful = true
	default:
		printf("不支持的任务: %v", task)
		return nil
	}
	return &result
}

// lookupIP 解析域名到 IP
func lookupIP(host string) (string, error) {
	addrs, err := net.LookupIP(host)
	if err != nil {
		for _, server := range util.DNSServersAll {
			r := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{Timeout: time.Second * 10}
					return d.DialContext(ctx, "udp", server)
				},
			}
			addrs, err = r.LookupIP(context.Background(), "ip", host)
			if err == nil {
				break
			}
		}
	}
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", errors.New("lookup ip failed")
	}
	ip := addrs[rand.Intn(len(addrs))]
	return ip.String(), nil
}

// handleTcpPingTask TCP 延迟探测
func handleTcpPingTask(task *pb.Task, result *pb.TaskResult) {
	if agentConfig.DisableSendQuery {
		result.Data = "This server has disabled query sending"
		return
	}

	host, port, err := net.SplitHostPort(task.GetData())
	if err != nil {
		result.Data = err.Error()
		return
	}
	ipAddr, err := lookupIP(host)
	if err != nil {
		result.Data = err.Error()
		return
	}
	// 内网探测防护：校验解析后的 IP，阻止探测内网/保留地址
	if !agentConfig.AllowProbeInternal {
		if err := validateResolvedIP(ipAddr); err != nil {
			result.Data = err.Error()
			return
		}
	}
	addr := net.JoinHostPort(ipAddr, port)
	printf("TCP-Ping Task: Pinging %s", addr)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, time.Second*10)
	if err != nil {
		result.Data = err.Error()
	} else {
		conn.Close()
		result.Delay = float32(time.Since(start).Microseconds()) / 1000.0
		result.Successful = true
	}
}

// handleIcmpPingTask ICMP 延迟探测
func handleIcmpPingTask(task *pb.Task, result *pb.TaskResult) {
	if agentConfig.DisableSendQuery {
		result.Data = "This server has disabled query sending"
		return
	}

	ipAddr, err := lookupIP(task.GetData())
	if err != nil {
		result.Data = err.Error()
		return
	}
	// 内网探测防护：校验解析后的 IP，阻止探测内网/保留地址
	if !agentConfig.AllowProbeInternal {
		if err := validateResolvedIP(ipAddr); err != nil {
			result.Data = err.Error()
			return
		}
	}
	printf("ICMP-Ping Task: Pinging %s(%s)", task.GetData(), ipAddr)
	// 并发限制：最多同时执行 3 个 ICMP 任务，超出时直接拒绝，防止 DDoS 放大
	select {
	case icmpSem <- struct{}{}:
		defer func() { <-icmpSem }()
	default:
		result.Data = "too many concurrent ICMP tasks"
		return
	}
	pinger, err := ping.NewPinger(ipAddr)
	if err == nil {
		pinger.SetPrivileged(true)
		pinger.Count = 5
		pinger.Timeout = time.Second * 20
		err = pinger.Run() // 阻塞直到完成
	}
	if err == nil {
		stat := pinger.Statistics()
		if stat.PacketsRecv == 0 {
			result.Data = "packets recv 0"
			return
		}
		result.Delay = float32(stat.AvgRtt.Microseconds()) / 1000.0
		result.Successful = true
	} else {
		result.Data = err.Error()
	}
}

// handleHttpGetTask HTTP 延迟探测
func handleHttpGetTask(task *pb.Task, result *pb.TaskResult) {
	if agentConfig.DisableSendQuery {
		result.Data = "This server has disabled query sending"
		return
	}
	taskUrl := task.GetData()
	// SSRF 防护：校验 URL 协议和目标地址，阻止探测内网/保留地址
	if !agentConfig.AllowProbeInternal {
		if err := validateProbeURL(taskUrl); err != nil {
			result.Data = err.Error()
			return
		}
	}
	start := time.Now()
	printf("HTTP-GET Task: %s", taskUrl)
	resp, err := httpClient.Get(taskUrl)
	if err == nil {
		defer resp.Body.Close()
		// 限制响应体读取大小为 1MB，防止恶意大文件导致内存耗尽
		_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	}
	if err == nil {
		result.Delay = float32(time.Since(start).Microseconds()) / 1000.0
		if resp.StatusCode > 399 || resp.StatusCode < 200 {
			err = errors.New("\n应用错误: " + resp.Status)
		}
	}
	if err == nil {
		if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
			c := resp.TLS.PeerCertificates[0]
			result.Data = c.Issuer.CommonName + "|" + c.NotAfter.String()
		}
		result.Successful = true
	} else {
		result.Data = err.Error()
	}
}
