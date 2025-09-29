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
	SrcIP   uint32
	DstIP   uint32
	SrcPort uint16
	DstPort uint16
	Proto   uint8
	Pad     [3]byte // alignment padding
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

	flag.StringVar(&iface, "i", "", "网络接口名称 (例如: eth0, enp0s3)")
	flag.StringVar(&iface, "interface", "", "网络接口名称 (例如: eth0, enp0s3)")
	flag.StringVar(&monitorMode, "m", "xdp", "监控模式: xdp, rdma, nccl")
	flag.StringVar(&monitorMode, "mode", "xdp", "监控模式: xdp, rdma, nccl")
	flag.StringVar(&rdmaDevice, "d", "mlx5_0", "RDMA 设备名称 (用于 rdma/nccl 模式)")
	flag.StringVar(&rdmaDevice, "device", "mlx5_0", "RDMA 设备名称 (用于 rdma/nccl 模式)")
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
		fmt.Fprintf(os.Stderr, "\n示例:\n")
		fmt.Fprintf(os.Stderr, "  %s -m xdp -i eth0        # XDP 模式监控 eth0 接口\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -m rdma -d mlx5_0     # RDMA 模式监控 mlx5_0 设备\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -m nccl -d mlx5_0     # NCCL 模式监控 mlx5_0 设备\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --list                 # 列出所有网络接口\n", os.Args[0])
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
		startXDPMonitor(iface)
	case "rdma":
		startRDMAMonitor(rdmaDevice, iface)
	case "nccl":
		startNCCLMonitor(rdmaDevice, iface)
	default:
		log.Fatalf("不支持的监控模式: %s (支持的模式: xdp, rdma, nccl)", monitorMode)
	}
}

// XDP 监控模式
func startXDPMonitor(iface string) {
	log.Printf("启动 XDP 监控模式，网络接口: %s", iface)

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
				fmt.Printf("%s:%d -> %s:%d proto=%d packets=%d bytes=%d\n",
					ipToStr(k.SrcIP), k.SrcPort,
					ipToStr(k.DstIP), k.DstPort,
					k.Proto, v.Packets, v.Bytes)
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
