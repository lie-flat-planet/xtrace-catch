//go:build linux
// +build linux

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

type FlowKey struct {
	SrcIP     uint32
	DstIP     uint32
	SrcPort   uint16
	DstPort   uint16
	Proto     uint8
	PktLenLow uint8  // 包长度低8位
	FirstU16  uint16 // 前2个字节
}

type FlowStats struct {
	Packets uint64
	Bytes   uint64
}

// RDMA 和 NCCL 监控的结构体定义在各自的文件中

func ipToStr(ip uint32) string {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, ip)
	return net.IPv4(b[0], b[1], b[2], b[3]).String()
}

func main() {
	// 命令行参数解析
	var iface string
	var showHelp bool
	var listInterfaces bool
	var monitorMode string
	var rdmaDevice string
	var filterTraffic string

	flag.StringVar(&iface, "i", "", "网络接口名称 (例如: eth0, enp0s3)")
	flag.StringVar(&iface, "interface", "", "网络接口名称 (例如: eth0, enp0s3)")
	flag.StringVar(&monitorMode, "m", "xdp", "监控模式: xdp, rdma, nccl")
	flag.StringVar(&monitorMode, "mode", "xdp", "监控模式: xdp, rdma, nccl")
	flag.StringVar(&rdmaDevice, "d", "mlx5_0", "RDMA 设备名称 (用于 rdma/nccl 模式)")
	flag.StringVar(&rdmaDevice, "device", "mlx5_0", "RDMA 设备名称 (用于 rdma/nccl 模式)")
	flag.StringVar(&filterTraffic, "f", "", "过滤流量类型: roce, roce_v1, roce_v2, tcp, udp, ib, all")
	flag.StringVar(&filterTraffic, "filter", "", "过滤流量类型: roce, roce_v1, roce_v2, tcp, udp, ib, all")
	flag.BoolVar(&showHelp, "h", false, "显示帮助信息")
	flag.BoolVar(&showHelp, "help", false, "显示帮助信息")
	flag.BoolVar(&listInterfaces, "l", false, "列出所有可用的网络接口")
	flag.BoolVar(&listInterfaces, "list", false, "列出所有可用的网络接口")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "XTrace-Catch: 多模式网络流量监控器\n\n")
		fmt.Fprintf(os.Stderr, "用法: %s [选项]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n监控模式:\n")
		fmt.Fprintf(os.Stderr, "  xdp  - eBPF/XDP 模式 (默认，监控经过网络栈的流量)\n")
		fmt.Fprintf(os.Stderr, "  rdma - RDMA 模式 (监控 RDMA 设备统计)\n")
		fmt.Fprintf(os.Stderr, "  nccl - NCCL 模式 (监控 RDMA 硬件统计)\n")
		fmt.Fprintf(os.Stderr, "\n流量过滤:\n")
		fmt.Fprintf(os.Stderr, "  roce       - 所有 RoCE 流量 (v1 + v2)\n")
		fmt.Fprintf(os.Stderr, "  roce_v1    - 仅 RoCE v1/IBoE 流量\n")
		fmt.Fprintf(os.Stderr, "  roce_v2    - 仅 RoCE v2 流量\n")
		fmt.Fprintf(os.Stderr, "  tcp        - 仅 TCP 流量\n")
		fmt.Fprintf(os.Stderr, "  udp        - 仅 UDP 流量\n")
		fmt.Fprintf(os.Stderr, "  ib         - 仅 InfiniBand 流量\n")
		fmt.Fprintf(os.Stderr, "  all        - 所有流量 (默认)\n")
		fmt.Fprintf(os.Stderr, "\n示例:\n")
		fmt.Fprintf(os.Stderr, "  %s -m xdp -i eth0                  # XDP 模式监控 eth0 接口\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -m xdp -i ibs8f0 -f roce        # 仅显示 RoCE 流量\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -m xdp -i ibs8f0 -f roce_v2     # 仅显示 RoCE v2 流量\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -m rdma -d mlx5_0               # RDMA 模式监控 mlx5_0 设备\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -m nccl -d mlx5_0               # NCCL 模式监控 mlx5_0 设备\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --list                           # 列出所有网络接口\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n环境变量:\n")
		fmt.Fprintf(os.Stderr, "  NETWORK_INTERFACE  设置默认网络接口\n")
		fmt.Fprintf(os.Stderr, "  MONITOR_MODE       设置默认监控模式\n")
	}

	flag.Parse()

	// 显示帮助
	if showHelp {
		flag.Usage()
		return
	}

	// 列出网络接口
	if listInterfaces {
		listNetworkInterfaces()
		return
	}

	// 确定监控模式：命令行参数 > 环境变量 > 默认值
	if monitorMode == "" {
		monitorMode = os.Getenv("MONITOR_MODE")
	}
	if monitorMode == "" {
		monitorMode = "xdp" // 默认值
	}

	// 确定网络接口：命令行参数 > 环境变量 > 默认值
	if iface == "" {
		iface = os.Getenv("NETWORK_INTERFACE")
	}
	if iface == "" {
		if monitorMode == "xdp" {
			iface = "eth0" // XDP 模式默认接口
		} else {
			iface = "ibs8f0" // RDMA 模式默认接口
		}
	}

	// 验证网络接口是否存在
	if !isValidInterface(iface) {
		log.Printf("警告: 网络接口 '%s' 可能不存在", iface)
		log.Printf("可用接口列表:")
		listNetworkInterfaces()
		log.Fatalf("请使用 -i 参数指定正确的网络接口")
	}

	// 根据监控模式启动相应的监控程序
	switch monitorMode {
	case "xdp":
		startXDPMonitor(iface, filterTraffic)
	case "rdma":
		startRDMAMonitor(rdmaDevice, iface)
	case "nccl":
		startNCCLMonitor(rdmaDevice, iface)
	default:
		log.Fatalf("不支持的监控模式: %s (支持的模式: xdp, rdma, nccl)", monitorMode)
	}
}

// 检查是否应该显示该流量
func shouldDisplayTraffic(proto uint8, srcPort, dstPort uint16, filter string) bool {
	if filter == "" || filter == "all" {
		return true
	}

	// 端口号在 eBPF 中是网络字节序（大端），需要转换
	// 4791 的大端字节序是 0xb712 (十进制 46866)
	rocePort := uint16(0xb712) // 4791 in network byte order

	// 判断流量类型
	isRoCEv2 := proto == 0xFE || (proto == 17 && (srcPort == rocePort || dstPort == rocePort))
	isRoCEv1 := proto == 0x15 // 0x8915 的低字节
	isIB := proto == 0x14     // 0x8914 的低字节
	isTCP := proto == 6
	isUDP := proto == 17

	switch filter {
	case "roce":
		return isRoCEv2 || isRoCEv1
	case "roce_v1":
		return isRoCEv1
	case "roce_v2":
		return isRoCEv2
	case "tcp":
		return isTCP
	case "udp":
		return isUDP
	case "ib":
		return isIB
	default:
		return true
	}
}

// XDP 监控模式
func startXDPMonitor(iface string, filter string) {
	filterMsg := ""
	if filter != "" && filter != "all" {
		filterMsg = fmt.Sprintf("，过滤: %s", filter)
	}
	log.Printf("启动 XDP 监控模式，网络接口: %s%s", iface, filterMsg)

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

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-ticker.C:
			iter := objs.Flows.Iterate()
			var k FlowKey
			var v FlowStats
			for iter.Next(&k, &v) {
				// 检查是否应该显示该流量
				if !shouldDisplayTraffic(k.Proto, k.SrcPort, k.DstPort, filter) {
					continue
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

				// 端口号需要从网络字节序转换回主机字节序来显示
				srcPort := binary.BigEndian.Uint16([]byte{byte(k.SrcPort >> 8), byte(k.SrcPort)})
				dstPort := binary.BigEndian.Uint16([]byte{byte(k.DstPort >> 8), byte(k.DstPort)})

				// 显示调试信息
				debugInfo := ""
				if k.SrcIP == 0 && k.DstIP == 0 {
					avgLen := uint64(0)
					if v.Packets > 0 {
						avgLen = v.Bytes / v.Packets
					}
					debugInfo = fmt.Sprintf(" [first_u16: 0x%04x, pkt_len: %d, avg: %d]",
						k.FirstU16, k.PktLenLow, avgLen)
				}

				fmt.Printf("%s:%d -> %s:%d proto=%d%s%s packets=%d bytes=%d\n",
					ipToStr(k.SrcIP), srcPort,
					ipToStr(k.DstIP), dstPort,
					k.Proto, trafficType, debugInfo, v.Packets, v.Bytes)
			}
			if err := iter.Err(); err != nil {
				log.Printf("iter error: %v", err)
			}
		case <-stop:
			break loop
		}
	}
}

// RDMA 监控模式
func startRDMAMonitor(deviceName, iface string) {
	log.Printf("启动 RDMA 监控模式，设备: %s，接口: %s", deviceName, iface)

	// 检查必要的工具
	if _, err := exec.LookPath("ibstat"); err != nil {
		log.Fatalf("ibstat 工具未找到，请安装: sudo apt-get install infiniband-diags")
	}

	// 创建并启动 RDMA 监控器
	monitor := NewRDMAMonitor(deviceName, iface)
	monitor.Start()
}

// NCCL 监控模式
func startNCCLMonitor(deviceName, iface string) {
	log.Printf("启动 NCCL 监控模式，设备: %s，接口: %s", deviceName, iface)

	// 检查必要的工具
	if _, err := exec.LookPath("ibv_devinfo"); err != nil {
		log.Fatalf("ibv_devinfo 工具未找到，请安装: sudo apt-get install perftest rdma-core")
	}

	// 创建并启动 NCCL 监控器
	// 将设备名称转换为设备 ID（简化处理）
	deviceID := 0
	if deviceName == "mlx5_1" {
		deviceID = 1
	}

	monitor := NewNCCLMonitor(deviceID, iface)
	monitor.Start()
}

// RDMA 和 NCCL 监控的实现函数在各自的文件中

// 获取网卡 index
func ifaceIndex(name string) int {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		log.Fatalf("lookup network iface %s: %v", name, err)
	}
	return iface.Index
}

// 检查网络接口是否有效
func isValidInterface(name string) bool {
	_, err := net.InterfaceByName(name)
	return err == nil
}

// 列出所有网络接口
func listNetworkInterfaces() {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("无法获取网络接口列表: %v", err)
		return
	}

	fmt.Printf("可用的网络接口:\n")
	for _, iface := range interfaces {
		status := "down"
		if iface.Flags&net.FlagUp != 0 {
			status = "up"
		}
		fmt.Printf("  %-15s %s (flags: %v)\n", iface.Name, status, iface.Flags)
	}
}
