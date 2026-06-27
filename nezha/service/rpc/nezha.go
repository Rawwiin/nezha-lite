// Modified by Nezha Lite for simplified dashboard functionality.
// Original source: https://github.com/nezhahq/nezha
// Licensed under the Apache License, Version 2.0

package rpc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	geoipx "github.com/nezhahq/nezha/pkg/geoip"
	"github.com/nezhahq/nezha/pkg/tsdb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/nezhahq/nezha/model"
	pb "github.com/nezhahq/nezha/proto"
	"github.com/nezhahq/nezha/service/singleton"
)

var _ pb.NezhaServiceServer = (*NezhaHandler)(nil)

var NezhaHandlerSingleton *NezhaHandler

// NezhaHandler 实现 gRPC NezhaService 服务端接口
// 精简版已移除 IOStream（终端/文件管理/内网穿透/MCP）功能
type NezhaHandler struct {
	Auth *authHandler
}

func NewNezhaHandler() *NezhaHandler {
	return &NezhaHandler{
		Auth: &authHandler{},
	}
}

// attachRequestTaskStream resolves the server for clientID and publishes the
// task stream. It mirrors the !ok || server == nil guard the other RPC entry
// points use: the server can be deleted between CheckRequestTask and this
// lookup, in which case Get returns a nil *Server and SetTaskStream would
// panic.
func attachRequestTaskStream(clientID uint64, stream pb.NezhaService_RequestTaskServer) (*model.Server, bool) {
	server, ok := singleton.ServerShared.Get(clientID)
	if !ok || server == nil {
		return nil, false
	}
	server.SetTaskStream(stream)
	return server, true
}

// clearRequestTaskStream detaches the dropped stream from whichever *Server is
// currently published for clientID. Edit and transfer rotation publish a new
// *Server that adopts the same stream holder, so cleanup must target the live
// map entry; the captured server is only the fallback for a removed entry.
func clearRequestTaskStream(clientID uint64, captured *model.Server, stream pb.NezhaService_RequestTaskServer) {
	if current, ok := singleton.ServerShared.Get(clientID); ok && current != nil {
		current.ClearTaskStreamIfCurrent(stream)
		return
	}
	captured.ClearTaskStreamIfCurrent(stream)
}

func (s *NezhaHandler) RequestTask(stream pb.NezhaService_RequestTaskServer) error {
	var clientID uint64
	var err error
	if clientID, err = s.Auth.CheckRequestTask(stream.Context()); err != nil {
		log.Printf("NEZHA>> RequestTask auth failed: %v", err)
		return err
	}

	server, ok := attachRequestTaskStream(clientID, stream)
	if !ok {
		log.Printf("NEZHA>> RequestTask: server not found for clientID=%d, TaskStream not established", clientID)
		return nil
	}
	log.Printf("NEZHA>> RequestTask: clientID=%d TaskStream established", clientID)
	defer clearRequestTaskStream(clientID, server, stream)
	var result *pb.TaskResult
	for {
		result, err = stream.Recv()
		if err != nil {
			log.Printf("NEZHA>> RequestTask error: %v, clientID: %d\n", err, clientID)
			return err
		}
		// 每次收到任务结果都刷新节点活跃时间，作为心跳补充
		if current, ok := singleton.ServerShared.Get(clientID); ok && current != nil {
			current.LastActive = time.Now()
		}
		if model.IsServiceSentinelNeeded(result.GetType()) {
			singleton.ServiceSentinelShared.Dispatch(singleton.ReportData{
				Data:     result,
				Reporter: clientID,
			})
		}
	}
}

func (s *NezhaHandler) ReportSystemState(stream pb.NezhaService_ReportSystemStateServer) error {
	clientID, err := s.Auth.Check(stream.Context())
	if err != nil {
		return err
	}
	var state *pb.State
	for {
		state, err = stream.Recv()
		if err != nil {
			log.Printf("NEZHA>> ReportSystemState error: %v, clientID: %d\n", err, clientID)
			return err
		}
		innerState := model.PB2State(state)

		server, ok := singleton.ServerShared.Get(clientID)
		if !ok || server == nil {
			return errors.New("server not found")
		}

		server.LastActive = time.Now()
		server.State = &innerState

		if singleton.TSDBEnabled() {
			maxTemp := 0.0
			for _, t := range innerState.Temperatures {
				if t.Temperature > maxTemp {
					maxTemp = t.Temperature
				}
			}
			maxGPU := 0.0
			for _, g := range innerState.GPU {
				if g > maxGPU {
					maxGPU = g
				}
			}
			if err := singleton.TSDBShared.WriteServerMetrics(&tsdb.ServerMetrics{
				ServerID:       clientID,
				Timestamp:      time.Now(),
				CPU:            innerState.CPU,
				MemUsed:        innerState.MemUsed,
				SwapUsed:       innerState.SwapUsed,
				DiskUsed:       innerState.DiskUsed,
				NetInSpeed:     innerState.NetInSpeed,
				NetOutSpeed:    innerState.NetOutSpeed,
				NetInTransfer:  innerState.NetInTransfer,
				NetOutTransfer: innerState.NetOutTransfer,
				Load1:          innerState.Load1,
				Load5:          innerState.Load5,
				Load15:         innerState.Load15,
				TCPConnCount:   innerState.TcpConnCount,
				UDPConnCount:   innerState.UdpConnCount,
				ProcessCount:   innerState.ProcessCount,
				Temperature:    maxTemp,
				Uptime:         innerState.Uptime,
				GPU:            maxGPU,
			}); err != nil {
				log.Printf("NEZHA>> Failed to write server metrics to TSDB: %v", err)
			}
		}

		// 应对 dashboard / agent 重启的情况，如果从未记录过，先打点，等到小时时间点时入库
		if server.PrevTransferInSnapshot == 0 || server.PrevTransferOutSnapshot == 0 {
			server.PrevTransferInSnapshot = state.NetInTransfer
			server.PrevTransferOutSnapshot = state.NetOutTransfer
		}

		if err = stream.Send(&pb.Receipt{Proced: true}); err != nil {
			return err
		}
	}
}

func (s *NezhaHandler) onReportSystemInfo(c context.Context, r *pb.Host) error {
	var clientID uint64
	var err error
	if clientID, err = s.Auth.Check(c); err != nil {
		return err
	}
	host := model.PB2Host(r)

	server, ok := singleton.ServerShared.Get(clientID)
	if !ok || server == nil {
		return errors.New("server not found")
	}

	/**
	 * 这里的 singleton 中的数据都是关机前的旧数据
	 * 当 agent 重启时，bootTime 变大，agent 端会先上报 host 信息，然后上报 state 信息
	 * 这时可以借助上报顺序的空档，立即记录停机前的数据并重置 Prev* 数据，并由接下来的 state 方法重新赋值
	 */
	if !server.LastActive.IsZero() && host.BootTime > server.Host.BootTime {
		singleton.RecordTransferHourlyUsage(server)
		server.PrevTransferInSnapshot = 0
		server.PrevTransferOutSnapshot = 0
	}

	server.Host = &host
	return nil
}

// ReportSystemInfo (v1) 已废弃：Agent 端只使用 ReportSystemInfo2，保留桩以满足 gRPC 接口
func (s *NezhaHandler) ReportSystemInfo(c context.Context, r *pb.Host) (*pb.Receipt, error) {
	return nil, status.Errorf(codes.Unimplemented, "ReportSystemInfo is deprecated, use ReportSystemInfo2")
}

func (s *NezhaHandler) ReportSystemInfo2(c context.Context, r *pb.Host) (*pb.Uint64Receipt, error) {
	if err := s.onReportSystemInfo(c, r); err != nil {
		return nil, err
	}
	return &pb.Uint64Receipt{Data: singleton.DashboardBootTime}, nil
}

// IOStream 已移除：精简版不再支持终端/文件管理/内网穿透/MCP 流通道
func (s *NezhaHandler) IOStream(stream pb.NezhaService_IOStreamServer) error {
	return status.Errorf(codes.Unimplemented, "IOStream has been removed in simplified build")
}

func (s *NezhaHandler) ReportGeoIP(c context.Context, r *pb.GeoIP) (*pb.GeoIP, error) {
	var clientID uint64
	var err error
	if clientID, err = s.Auth.Check(c); err != nil {
		return nil, err
	}

	geoip := model.PB2GeoIP(r)
	use6 := r.GetUse6()

	if geoip.IP.IPv4Addr == "" && geoip.IP.IPv6Addr == "" {
		ip, _ := c.Value(model.CtxKeyRealIP{}).(string)
		if ip == "" {
			ip, _ = c.Value(model.CtxKeyConnectingIP{}).(string)
		}
		geoip.IP.IPv4Addr = ip
	}

	server, ok := singleton.ServerShared.Get(clientID)
	if !ok || server == nil {
		return nil, fmt.Errorf("server not found")
	}

	joinedIP := geoip.IP.Join()

	// 检查并更新 DDNS
	if server.EnableDDNS && joinedIP != "" &&
		(server.GeoIP == nil || server.GeoIP.IP != geoip.IP) {
		ipv4 := geoip.IP.IPv4Addr
		ipv6 := geoip.IP.IPv6Addr

		if err := singleton.ServerShared.UpdateDDNS(server, &model.IP{IPv4Addr: ipv4, IPv6Addr: ipv6}); err != nil {
			log.Printf("NEZHA>> Failed to update DDNS for server %d: %v", err, server.ID)
		}
	}

	// 根据内置数据库查询 IP 地理位置
	var ip string
	if geoip.IP.IPv6Addr != "" && (use6 || geoip.IP.IPv4Addr == "") {
		ip = geoip.IP.IPv6Addr
	} else {
		ip = geoip.IP.IPv4Addr
	}

	netIP := net.ParseIP(ip)
	location, err := geoipx.Lookup(netIP)
	if err != nil {
		log.Printf("NEZHA>> geoip.Lookup: %v", err)
	}
	geoip.CountryCode = location

	// 将地区码写入到 Host
	server.GeoIP = &geoip

	return &pb.GeoIP{Ip: nil, CountryCode: location, DashboardBootTime: singleton.DashboardBootTime}, nil
}
