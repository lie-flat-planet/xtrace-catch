//go:build linux
// +build linux

package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/prometheus/client_golang/prometheus"
)

// TC 监控模式 - 支持出口流量检测
func startTCMonitor(iface string, filter string, excludeDNS bool, intervalMs int, direction string) {
	filterMsg := ""
	if filter != "" && filter != "all" {
		filterMsg = fmt.Sprintf("，过滤: %s", filter)
	}

	// 获取主机IP地址
	hostIP := getHostIP()
	log.Printf("启动 TC %s 监控模式，网络接口: %s，主机IP: %s，采集间隔: %dms%s",
		direction, iface, hostIP, intervalMs, filterMsg)

	// 加载TC eBPF程序
	spec, err := ebpf.LoadCollectionSpec("tc_monitor.o")
	if err != nil {
		log.Fatalf("failed to load TC spec: %v", err)
	}

	objs := struct {
		TcMonitor *ebpf.Program `ebpf:"tc_egress_monitor"`
		Flows     *ebpf.Map     `ebpf:"flows"`
	}{}
	if err := spec.LoadAndAssign(&objs, nil); err != nil {
		log.Fatalf("failed to load TC objects: %v", err)
	}
	defer objs.TcMonitor.Close()
	defer objs.Flows.Close()

	// 清理可能存在的旧TC规则
	cleanupTCRules(iface, direction)

	// 创建TC qdisc和filter
	if err := setupTCRules(iface, direction); err != nil {
		log.Fatalf("failed to setup TC rules: %v", err)
	}

	// 使用tc命令附加eBPF程序
	if err := attachTCProgram(iface, direction); err != nil {
		log.Fatalf("failed to attach TC program: %v", err)
	}

	log.Printf("TC %s program loaded on %s", direction, iface)

	// 捕获 Ctrl+C 退出
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// 将毫秒转换为 Duration
	duration := time.Duration(intervalMs) * time.Millisecond
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	// 确保程序退出时清理TC规则
	defer cleanupTCRules(iface, direction)

loop:
	for {
		select {
		case <-ticker.C:
			iter := objs.Flows.Iterate()
			var k FlowKey
			var v FlowStats
			for iter.Next(&k, &v) {
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

				// 端口号需要从网络字节序转换回主机字节序来显示
				srcPort := binary.BigEndian.Uint16([]byte{byte(k.SrcPort >> 8), byte(k.SrcPort)})
				dstPort := binary.BigEndian.Uint16([]byte{byte(k.DstPort >> 8), byte(k.DstPort)})

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
						"direction":    direction, // 添加方向标签
					}

					networkBytesTotal.With(labels).Add(float64(v.Bytes))
					networkPacketsTotal.With(labels).Add(float64(v.Packets))
					networkFlowBytes.With(labels).Set(float64(v.Bytes))
					networkFlowPackets.With(labels).Set(float64(v.Packets))
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

				// 打印流量信息，添加方向标识
				fmt.Printf("[%s] %s:%d -> %s:%d proto=%d%s packets=%d bytes=%d host_ip=%s\n",
					direction, ipToStr(k.SrcIP), srcPort,
					ipToStr(k.DstIP), dstPort,
					k.Proto, trafficType, v.Packets, v.Bytes, hostIP)
			}
			if err := iter.Err(); err != nil {
				log.Printf("iter error: %v", err)
			}

			// 推送 metrics 到 VictoriaMetrics
			if metricsEnabled {
				if err := pushMetricsToVictoriaMetrics(); err != nil {
					log.Printf("推送 VictoriaMetrics metrics 失败: %v", err)
				}
			}
		case <-stop:
			break loop
		}
	}
}

// 设置TC规则
func setupTCRules(iface string, direction string) error {
	// 创建qdisc
	cmd := exec.Command("tc", "qdisc", "add", "dev", iface, "clsact")
	if err := cmd.Run(); err != nil {
		// 如果qdisc已存在，忽略错误
		log.Printf("TC qdisc may already exist: %v", err)
	}

	return nil
}

// 附加TC程序
func attachTCProgram(iface string, direction string) error {
	var cmd *exec.Cmd
	if direction == "egress" {
		cmd = exec.Command("tc", "filter", "add", "dev", iface, "egress", "bpf", "direct-action", "obj", "tc_monitor.o", "sec", "tc")
	} else {
		cmd = exec.Command("tc", "filter", "add", "dev", iface, "ingress", "bpf", "direct-action", "obj", "tc_monitor.o", "sec", "tc")
	}

	return cmd.Run()
}

// 清理TC规则
func cleanupTCRules(iface string, direction string) {
	log.Printf("清理 TC %s 规则...", direction)

	// 删除filter
	var cmd *exec.Cmd
	if direction == "egress" {
		cmd = exec.Command("tc", "filter", "del", "dev", iface, "egress")
	} else {
		cmd = exec.Command("tc", "filter", "del", "dev", iface, "ingress")
	}
	cmd.Run() // 忽略错误，可能规则不存在

	// 删除qdisc
	cmd = exec.Command("tc", "qdisc", "del", "dev", iface, "clsact")
	cmd.Run() // 忽略错误，可能qdisc不存在
}
