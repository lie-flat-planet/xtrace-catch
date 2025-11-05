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
	"sync"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/prometheus/client_golang/prometheus"
)

// 多接口监控模式
func startMultiInterfaceMonitor(interfaces []string, filter string, excludeDNS bool, intervalMs int) {
	filterMsg := ""
	if filter != "" && filter != "all" {
		filterMsg = fmt.Sprintf("，过滤: %s", filter)
	}

	// 获取主机IP地址
	hostIP := getHostIP()
	log.Printf("启动多接口 XDP 监控模式，接口数量: %d，主机IP: %s，采集间隔: %dms%s", len(interfaces), hostIP, intervalMs, filterMsg)
	log.Printf("监控接口列表: %v", interfaces)

	// 捕获 Ctrl+C 退出
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// 用于等待所有 goroutine 完成
	var wg sync.WaitGroup

	// 用于同步采集完成，通知推送 goroutine
	// buffer 设置为 len(interfaces)*2，避免短暂阻塞
	var collectDone chan struct{}
	if metricsEnabled {
		collectDone = make(chan struct{}, len(interfaces)*2)
	}

	// 为每个接口启动独立的监控 goroutine
	for _, iface := range interfaces {
		wg.Add(1)
		go func(interfaceName string) {
			defer wg.Done()
			monitorInterface(interfaceName, filter, excludeDNS, intervalMs, stop, collectDone)
		}(iface)
	}

	// 如果启用了 metrics，启动统一的推送 goroutine
	if metricsEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 持续等待所有采集 goroutine 完成采集并推送
			// 注意：各接口的采集 ticker 独立，存在微小时序差异（通常<100ms）
			// 这对 Counter 类型无影响，对 Gauge 类型影响可忽略
			collectedCount := 0
			for {
				select {
				case <-collectDone:
					collectedCount++
					// 收到所有接口的采集完成信号后推送
					if collectedCount >= len(interfaces) {
						// 所有接口采集完成，统一推送所有接口的 metrics
						if err := pushMetricsToVictoriaMetrics(); err != nil {
							log.Printf("推送 VictoriaMetrics metrics 失败: %v", err)
						}

						// 推送后重置 Gauge，避免旧值残留
						networkFlowBytesRate.Reset()
						networkFlowBitsRate.Reset()

						// 重置计数器，等待下一轮采集
						collectedCount = 0
					}
				case <-stop:
					return
				}
			}
		}()
	}

	// 等待所有 goroutine 完成
	wg.Wait()
	log.Printf("所有接口监控已停止")
}

// 核心监控函数
func monitorInterface(iface string, filter string, excludeDNS bool, intervalMs int, stopChan chan os.Signal, collectDone chan struct{}) {
	filterMsg := ""
	if filter != "" && filter != "all" {
		filterMsg = fmt.Sprintf("，过滤: %s", filter)
	}

	// 获取主机IP地址
	hostIP := getHostIP()
	log.Printf("[%s] 启动 XDP 监控，主机IP: %s，采集间隔: %dms%s", iface, hostIP, intervalMs, filterMsg)

	// 用于保存上次统计数据的map
	lastStats := make(map[FlowKey]FlowStats)

	spec, err := ebpf.LoadCollectionSpec("xdp_monitor.o")
	if err != nil {
		log.Printf("[%s] 加载 eBPF 规范失败: %v，跳过该接口", iface, err)
		return
	}

	objs := struct {
		XdpMonitor *ebpf.Program `ebpf:"xdp_monitor"`
		Flows      *ebpf.Map     `ebpf:"flows"`
	}{}
	if err := spec.LoadAndAssign(&objs, nil); err != nil {
		log.Printf("[%s] 加载 eBPF 对象失败: %v，跳过该接口", iface, err)
		return
	}
	defer objs.XdpMonitor.Close()
	defer objs.Flows.Close()

	linkRef, err := link.AttachXDP(link.XDPOptions{
		Program:   objs.XdpMonitor,
		Interface: ifaceIndex(iface),
	})
	if err != nil {
		log.Printf("[%s] 附加 XDP 程序失败: %v，跳过该接口", iface, err)
		return
	}
	defer linkRef.Close()

	log.Printf("[%s] XDP program loaded", iface)

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
				last, exists := lastStats[k]
				deltaPackets, deltaBytes := calculateDelta(v, last, exists)
				lastStats[k] = v

				// 端口号转换和速率计算
				srcPort, dstPort := convertPorts(k.SrcPort, k.DstPort)
				bytesPerSec, bitsPerSec := calculateRates(deltaBytes, intervalSeconds)
				trafficTypeStr := getTrafficType(k.Proto, k.SrcPort, k.DstPort)

				// 更新 VictoriaMetrics metrics（如果启用）
				if metricsEnabled {
					updateMetrics(k, srcPort, dstPort, trafficTypeStr, deltaBytes, deltaPackets,
						bytesPerSec, bitsPerSec, iface, hostIP)
				}

				// 转换为显示格式
				trafficType := formatTrafficType(trafficTypeStr)

				// 只显示有实际流量的记录（跳过增量为0的）
				if deltaPackets > 0 {
					mbps := bitsPerSec / 1000000 // Mbps

					// 打印流量信息（增量值 + 速率），包含接口名称
					fmt.Printf("[%s] %s:%d -> %s:%d proto=%d%s packets=%d bytes=%d (%.2f MB/s, %.2f Mbps) host_ip=%s\n",
						iface, ipToStr(k.SrcIP), srcPort,
						ipToStr(k.DstIP), dstPort,
						k.Proto, trafficType, deltaPackets, deltaBytes,
						bytesPerSec/1024/1024, mbps, hostIP)
				}
			}
			if err := iter.Err(); err != nil {
				log.Printf("[%s] iter error: %v", iface, err)
			}

			// 清理不活跃的流（不在当前 BPF map 中的流）
			for key := range lastStats {
				if !activeFlows[key] {
					delete(lastStats, key)
				}
			}

			// 通知采集完成（如果启用了 metrics）
			if metricsEnabled && collectDone != nil {
				// 使用非阻塞发送，但增加重试机制
				select {
				case collectDone <- struct{}{}:
					// 成功发送采集完成信号
				default:
					// channel 已满，异步发送避免阻塞采集
					go func() {
						select {
						case collectDone <- struct{}{}:
							// 异步发送成功
						case <-time.After(time.Second):
							// 超时，说明推送 goroutine 可能已停止
							log.Printf("[%s] 警告: 采集完成信号发送超时", iface)
						}
					}()
				}
			}
		case <-stopChan:
			log.Printf("[%s] 接收到停止信号，正在关闭监控...", iface)
			break loop
		}
	}

	log.Printf("[%s] XDP 监控已停止", iface)
}

// 检查是否应该显示该流量（根据过滤条件）
func shouldDisplayTraffic(proto uint8, srcPort, dstPort uint16, filter string) bool {
	if filter == "" || filter == "all" {
		return true
	}

	switch filter {
	case "roce":
		// 显示所有 RoCE 流量 (v1 + v2)
		return proto == 0xFE || proto == 0x15 || proto == 0x14 ||
			(proto == 17 && (srcPort == roceV2Port || dstPort == roceV2Port))
	case "roce_v1":
		// 仅显示 RoCE v1/IBoE 流量
		return proto == 0x15 || proto == 0x14
	case "roce_v2":
		// 仅显示 RoCE v2 流量
		return proto == 0xFE || (proto == 17 && (srcPort == roceV2Port || dstPort == roceV2Port))
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

// 计算流量增量（处理计数器回绕）
func calculateDelta(current, last FlowStats, exists bool) (deltaPackets, deltaBytes uint64) {
	if exists {
		// 计算增量（处理可能的计数器回绕）
		if current.Packets >= last.Packets {
			deltaPackets = current.Packets - last.Packets
		} else {
			// 计数器回绕，使用当前值
			deltaPackets = current.Packets
		}
		if current.Bytes >= last.Bytes {
			deltaBytes = current.Bytes - last.Bytes
		} else {
			// 计数器回绕，使用当前值
			deltaBytes = current.Bytes
		}
	} else {
		// 第一次看到这个流，使用当前值
		deltaPackets = current.Packets
		deltaBytes = current.Bytes
	}

	return deltaPackets, deltaBytes
}

// 转换端口号从网络字节序到主机字节序
func convertPorts(srcPort, dstPort uint16) (uint16, uint16) {
	src := binary.BigEndian.Uint16([]byte{byte(srcPort >> 8), byte(srcPort)})
	dst := binary.BigEndian.Uint16([]byte{byte(dstPort >> 8), byte(dstPort)})
	return src, dst
}

// 计算流量速率
func calculateRates(deltaBytes uint64, intervalSeconds float64) (bytesPerSec, bitsPerSec float64) {
	bytesPerSec = float64(deltaBytes) / intervalSeconds
	bitsPerSec = bytesPerSec * 8
	return bytesPerSec, bitsPerSec
}

// 更新 VictoriaMetrics metrics
func updateMetrics(k FlowKey, srcPort, dstPort uint16, trafficType string,
	deltaBytes, deltaPackets uint64, bytesPerSec, bitsPerSec float64, iface, hostIP string) {

	labels := prometheus.Labels{
		"src_ip":       ipToStr(k.SrcIP),
		"dst_ip":       ipToStr(k.DstIP),
		"src_port":     strconv.Itoa(int(srcPort)),
		"dst_port":     strconv.Itoa(int(dstPort)),
		"protocol":     strconv.Itoa(int(k.Proto)),
		"traffic_type": trafficType,
		"interface":    iface,
		"host_ip":      hostIP,
		"collect_agg":  collectAgg,
	}

	// 速率指标（与 node_exporter irate 兼容）
	networkFlowBytesRate.With(labels).Set(bytesPerSec)
	networkFlowBitsRate.With(labels).Set(bitsPerSec)

	// Counter 累加总流量
	networkBytesTotal.With(labels).Add(float64(deltaBytes))
	networkPacketsTotal.With(labels).Add(float64(deltaPackets))
}

// 格式化流量类型用于显示（添加方括号和空格）
func formatTrafficType(trafficType string) string {
	switch trafficType {
	case "RoCE_v2":
		return " [RoCE v2]"
	case "RoCE_v2_UDP":
		return " [RoCE v2/UDP]"
	case "TCP":
		return " [TCP]"
	case "UDP":
		return " [UDP]"
	case "RoCE_v1_IBoE":
		return " [RoCE v1/IBoE]"
	case "InfiniBand":
		return " [InfiniBand]"
	default:
		return " [Other]"
	}
}
