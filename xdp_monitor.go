//go:build linux
// +build linux

package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/prometheus/client_golang/prometheus"
)

// XDP 监控模式
func startXDPMonitor(iface string, filter string, excludeDNS bool, intervalMs int) {
	filterMsg := ""
	if filter != "" && filter != "all" {
		filterMsg = fmt.Sprintf("，过滤: %s", filter)
	}

	// 获取主机IP地址
	hostIP := getHostIP()
	log.Printf("启动 XDP 监控模式，网络接口: %s，主机IP: %s，采集间隔: %dms%s", iface, hostIP, intervalMs, filterMsg)

	// 用于保存上次统计数据的map
	lastStats := make(map[FlowKey]FlowStats)

	spec, err := ebpf.LoadCollectionSpec("xdp_monitor.o")
	if err != nil {
		log.Fatalf("failed to load spec: %v", err)
	}

	objs := struct {
		XdpMonitor *ebpf.Program `ebpf:"xdp_monitor"`
		Flows      *ebpf.Map     `ebpf:"flows"`
	}{}
	if err := spec.LoadAndAssign(&objs, nil); err != nil {
		log.Fatalf("failed to load objects: %v", err)
	}
	defer objs.XdpMonitor.Close()
	defer objs.Flows.Close()

	linkRef, err := link.AttachXDP(link.XDPOptions{
		Program:   objs.XdpMonitor,
		Interface: ifaceIndex(iface),
	})
	if err != nil {
		log.Fatalf("failed to attach XDP: %v", err)
	}
	defer linkRef.Close()

	log.Printf("XDP program loaded on %s", iface)

	// 捕获 Ctrl+C 退出
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// 将毫秒转换为 Duration
	duration := time.Duration(intervalMs) * time.Millisecond
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	// 记录上次采集时间，用于计算速率
	lastCollectTime := time.Now()

loop:
	for {
		select {
		case <-ticker.C:
			// 计算时间间隔（实际经过的时间，用于精确计算速率）
			now := time.Now()
			intervalSeconds := now.Sub(lastCollectTime).Seconds()
			lastCollectTime = now

			// 记录本次采集中活跃的流
			activeFlows := make(map[FlowKey]bool)

			// 总流量统计（用于显示汇总）
			var totalDeltaBytes, totalDeltaPackets uint64

			iter := objs.Flows.Iterate()
			var k FlowKey
			var v FlowStats
			for iter.Next(&k, &v) {
				// 标记为活跃流
				activeFlows[k] = true
				// 过滤无效流量：跳过 src_ip 和 dst_ip 都为 0 的数据
				if k.SrcIP == 0 && k.DstIP == 0 {
					continue
				}

				// 检查是否应该显示该流量
				if !shouldDisplayTraffic(k.Proto, k.SrcPort, k.DstPort, filter) {
					continue
				}

				// 排除DNS流量（如果启用）
				if excludeDNS && isDNSTraffic(k.DstIP) {
					continue
				}

				// 计算增量流量
				var deltaPackets, deltaBytes uint64
				if last, exists := lastStats[k]; exists {
					// 计算增量（处理可能的计数器回绕）
					if v.Packets >= last.Packets {
						deltaPackets = v.Packets - last.Packets
					} else {
						// 计数器回绕，使用当前值
						deltaPackets = v.Packets
					}
					if v.Bytes >= last.Bytes {
						deltaBytes = v.Bytes - last.Bytes
					} else {
						// 计数器回绕，使用当前值
						deltaBytes = v.Bytes
					}
				} else {
					// 第一次看到这个流，使用当前值
					deltaPackets = v.Packets
					deltaBytes = v.Bytes
				}

				// 保存当前统计用于下次计算
				lastStats[k] = v

				// 累加到总流量
				totalDeltaBytes += deltaBytes
				totalDeltaPackets += deltaPackets

				// 端口号需要从网络字节序转换回主机字节序来显示
				srcPort := binary.BigEndian.Uint16([]byte{byte(k.SrcPort >> 8), byte(k.SrcPort)})
				dstPort := binary.BigEndian.Uint16([]byte{byte(k.DstPort >> 8), byte(k.DstPort)})

				// 计算速率（用于metrics和显示）
				bytesPerSec := float64(deltaBytes) / intervalSeconds
				bitsPerSec := bytesPerSec * 8

				// 更新 VictoriaMetrics metrics（如果启用）
				if metricsEnabled {
					srcIPStr := ipToStr(k.SrcIP)
					dstIPStr := ipToStr(k.DstIP)
					srcPortStr := strconv.Itoa(int(srcPort))
					dstPortStr := strconv.Itoa(int(dstPort))
					protoStr := strconv.Itoa(int(k.Proto))
					trafficTypeStr := getTrafficType(k.Proto, k.SrcPort, k.DstPort)

					labels := prometheus.Labels{
						"src_ip":       srcIPStr,
						"dst_ip":       dstIPStr,
						"src_port":     srcPortStr,
						"dst_port":     dstPortStr,
						"protocol":     protoStr,
						"traffic_type": trafficTypeStr,
						"interface":    iface,
						"host_ip":      hostIP,
						"collect_agg":  collectAgg,
					}

					// 使用 Gauge 设置当前流量（每个流的增量值）
					networkFlowBytes.With(labels).Set(float64(deltaBytes))
					networkFlowPackets.With(labels).Set(float64(deltaPackets))

					// 速率指标（与 node_exporter irate 兼容）
					networkFlowBytesRate.With(labels).Set(bytesPerSec)
					networkFlowBitsRate.With(labels).Set(bitsPerSec)

					// 同时使用 Counter 累加总流量
					networkBytesTotal.With(labels).Add(float64(deltaBytes))
					networkPacketsTotal.With(labels).Add(float64(deltaPackets))
				}

				// 识别流量类型
				rocePort := uint16(0xb712) // 4791 in network byte order
				trafficType := ""
				switch k.Proto {
				case 0xFE:
					trafficType = " [RoCE v2]"
				case 6:
					trafficType = " [TCP]"
				case 17:
					// 检查是否是 RoCE v2 (UDP port 4791)
					if k.SrcPort == rocePort || k.DstPort == rocePort {
						trafficType = " [RoCE v2/UDP]"
					} else {
						trafficType = " [UDP]"
					}
				default:
					// 对于大于 255 的协议值，可能是以太网协议类型
					if k.Proto == 0x15 { // 0x8915 的低字节
						trafficType = " [RoCE v1/IBoE]"
					} else if k.Proto == 0x14 { // 0x8914 的低字节
						trafficType = " [InfiniBand]"
					}
				}

				// 只显示有实际流量的记录（跳过增量为0的）
				if deltaPackets > 0 {
					mbps := bitsPerSec / 1000000 // Mbps

					// 打印流量信息（增量值 + 速率）
					fmt.Printf("%s:%d -> %s:%d proto=%d%s packets=%d bytes=%d (%.2f MB/s, %.2f Mbps) host_ip=%s\n",
						ipToStr(k.SrcIP), srcPort,
						ipToStr(k.DstIP), dstPort,
						k.Proto, trafficType, deltaPackets, deltaBytes,
						bytesPerSec/1024/1024, mbps, hostIP)
				}
			}
			if err := iter.Err(); err != nil {
				log.Printf("iter error: %v", err)
			}

			// 清理不活跃的流（不在当前 BPF map 中的流）
			for key := range lastStats {
				if !activeFlows[key] {
					delete(lastStats, key)
				}
			}

			// 推送 metrics 到 VictoriaMetrics
			if metricsEnabled {
				if err := pushMetricsToVictoriaMetrics(); err != nil {
					log.Printf("推送 VictoriaMetrics metrics 失败: %v", err)
				}

				// 推送后重置 Gauge，避免旧值残留
				networkFlowBytes.Reset()
				networkFlowPackets.Reset()
				networkFlowBytesRate.Reset()
				networkFlowBitsRate.Reset()
			}
		case <-stop:
			break loop
		}
	}
}

// 检查是否应该显示该流量（根据过滤条件）
func shouldDisplayTraffic(proto uint8, srcPort, dstPort uint16, filter string) bool {
	if filter == "" || filter == "all" {
		return true
	}

	rocePort := uint16(0xb712) // 4791 in network byte order

	switch filter {
	case "roce":
		// 显示所有 RoCE 流量 (v1 + v2)
		return proto == 0xFE || proto == 0x15 || proto == 0x14 ||
			(proto == 17 && (srcPort == rocePort || dstPort == rocePort))
	case "roce_v1":
		// 仅显示 RoCE v1/IBoE 流量
		return proto == 0x15 || proto == 0x14
	case "roce_v2":
		// 仅显示 RoCE v2 流量
		return proto == 0xFE || (proto == 17 && (srcPort == rocePort || dstPort == rocePort))
	case "tcp":
		return proto == 6
	case "udp":
		return proto == 17
	case "ib":
		return proto == 0x14
	default:
		return true
	}
}

// 检查是否是DNS流量（常见的DNS服务器）
func isDNSTraffic(dstIP uint32) bool {
	// 将 IP 转换为字节数组
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, dstIP)

	// 常见的DNS服务器列表
	// 阿里云DNS
	if b[0] == 223 && b[1] == 5 && b[2] == 5 && b[3] == 5 {
		return true
	}
	if b[0] == 223 && b[1] == 6 && b[2] == 6 && b[3] == 6 {
		return true
	}
	// 114DNS
	if b[0] == 114 && b[1] == 114 && b[2] == 114 && b[3] == 114 {
		return true
	}
	if b[0] == 114 && b[1] == 114 && b[2] == 115 && b[3] == 115 {
		return true
	}
	// Google DNS
	if b[0] == 8 && b[1] == 8 && b[2] == 8 && b[3] == 8 {
		return true
	}
	if b[0] == 8 && b[1] == 8 && b[2] == 4 && b[3] == 4 {
		return true
	}
	// Cloudflare DNS
	if b[0] == 1 && b[1] == 1 && b[2] == 1 && b[3] == 1 {
		return true
	}
	if b[0] == 1 && b[1] == 0 && b[2] == 0 && b[3] == 1 {
		return true
	}

	return false
}
