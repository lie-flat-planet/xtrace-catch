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
)

// FlowKey 和 FlowStats 结构体定义
type FlowKey struct {
	SrcIP     uint32
	DstIP     uint32
	SrcPort   uint16
	DstPort   uint16
	Proto     uint8
	PktLenLow uint8  // 包长度低8位
	FirstU16  uint16 // 前2个字节
	Padding   uint32 // 填充字段，保持结构对齐
}

type FlowStats struct {
	Packets    uint64
	Bytes      uint64
	LastUpdate uint64 // 最后更新时间（纳秒）
}

// 将 IP 地址从 uint32 转换为字符串
func ipToStr(ip uint32) string {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, ip)
	return net.IPv4(b[0], b[1], b[2], b[3]).String()
}

// 获取主机IP地址（获取第一个非环回的IPv4地址）
func getHostIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "unknown"
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return "unknown"
}

// 获取网络接口索引
func ifaceIndex(ifaceName string) int {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		log.Fatalf("failed to get interface %s: %v", ifaceName, err)
	}
	return iface.Index
}

// 检查网络接口是否存在
func isValidInterface(ifaceName string) bool {
	_, err := net.InterfaceByName(ifaceName)
	return err == nil
}

// 列出所有网络接口
func listNetworkInterfaces() {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Fatalf("无法获取网络接口列表: %v", err)
	}

	log.Printf("可用的网络接口:")
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 {
			log.Printf("  - %s (索引: %d, 状态: UP)", iface.Name, iface.Index)
		} else {
			log.Printf("  - %s (索引: %d, 状态: DOWN)", iface.Name, iface.Index)
		}
	}
}

func main() {
	// 命令行参数解析
	var iface string
	var showHelp bool
	var listInterfaces bool
	var filterTraffic string
	var excludeDNS bool
	var intervalMs int
	var countL2 bool

	flag.StringVar(&iface, "i", "", "网络接口名称 (例如: eth0, enp0s3)")
	flag.StringVar(&iface, "interface", "", "网络接口名称 (例如: eth0, enp0s3)")
	flag.StringVar(&filterTraffic, "f", "", "过滤流量类型: roce, roce_v1, roce_v2, tcp, udp, ib, all")
	flag.StringVar(&filterTraffic, "filter", "", "过滤流量类型: roce, roce_v1, roce_v2, tcp, udp, ib, all")
	flag.BoolVar(&excludeDNS, "exclude-dns", false, "排除DNS流量（过滤常见DNS服务器）")
	flag.IntVar(&intervalMs, "t", 5000, "数据采集和推送间隔（毫秒），默认5000ms")
	flag.IntVar(&intervalMs, "interval", 5000, "数据采集和推送间隔（毫秒），默认5000ms")
	flag.BoolVar(&countL2, "count-l2", false, "统计完整包长（包含L2层开销，类似node_exporter）")
	flag.BoolVar(&showHelp, "h", false, "显示帮助信息")
	flag.BoolVar(&showHelp, "help", false, "显示帮助信息")
	flag.BoolVar(&listInterfaces, "l", false, "列出所有可用的网络接口")
	flag.BoolVar(&listInterfaces, "list", false, "列出所有可用的网络接口")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "XTrace-Catch: XDP 网络流量监控器\n\n")
		fmt.Fprintf(os.Stderr, "用法: %s [选项]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n流量过滤:\n")
		fmt.Fprintf(os.Stderr, "  roce       - 所有 RoCE 流量 (v1 + v2)\n")
		fmt.Fprintf(os.Stderr, "  roce_v1    - 仅 RoCE v1/IBoE 流量\n")
		fmt.Fprintf(os.Stderr, "  roce_v2    - 仅 RoCE v2 流量\n")
		fmt.Fprintf(os.Stderr, "  tcp        - 仅 TCP 流量\n")
		fmt.Fprintf(os.Stderr, "  udp        - 仅 UDP 流量\n")
		fmt.Fprintf(os.Stderr, "  ib         - 仅 InfiniBand 流量\n")
		fmt.Fprintf(os.Stderr, "  all        - 所有流量 (默认)\n")
		fmt.Fprintf(os.Stderr, "\n其他选项:\n")
		fmt.Fprintf(os.Stderr, "  --exclude-dns     排除DNS流量（过滤223.5.5.5等常见DNS服务器）\n")
		fmt.Fprintf(os.Stderr, "  -t, --interval    数据采集和推送间隔（毫秒），默认5000ms，范围100-3600000\n")
		fmt.Fprintf(os.Stderr, "  --count-l2        统计完整包长（包含L2层开销），用于与node_exporter对比\n")
		fmt.Fprintf(os.Stderr, "\n示例:\n")
		fmt.Fprintf(os.Stderr, "  %s -i eth0                        # 监控 eth0 接口\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -i ibs8f0 -f roce              # 仅显示 RoCE 流量\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -i ibs8f0 -f roce_v2           # 仅显示 RoCE v2 流量\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -i eth0 --exclude-dns          # 排除DNS流量\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -i eth0 -t 500                 # 每500ms采集一次（高频）\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -i eth0 -t 10000               # 每10秒采集一次数据\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -i ibs8f0 --count-l2           # 统计完整包（含L2开销），与node_exporter对比\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --list                         # 列出所有网络接口\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n环境变量:\n")
		fmt.Fprintf(os.Stderr, "  NETWORK_INTERFACE             设置默认网络接口\n")
		fmt.Fprintf(os.Stderr, "  VICTORIAMETRICS_ENABLED       启用 VictoriaMetrics 推送 (true/1 启用)\n")
		fmt.Fprintf(os.Stderr, "  VICTORIAMETRICS_REMOTE_WRITE  VictoriaMetrics remote write URL\n")
		fmt.Fprintf(os.Stderr, "                                支持: /api/v1/import/prometheus (Text Format)\n")
		fmt.Fprintf(os.Stderr, "                                      /api/v1/write (Remote Write Protocol)\n")
		fmt.Fprintf(os.Stderr, "                                (默认: http://localhost:8428/api/v1/import/prometheus)\n")
		fmt.Fprintf(os.Stderr, "  COLLECT_AGG                   算网标签，用于标识数据来源 (默认: default)\n")
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

	// 确定网络接口：命令行参数 > 环境变量 > 默认值
	if iface == "" {
		iface = os.Getenv("NETWORK_INTERFACE")
	}
	if iface == "" {
		iface = "eth0" // 默认接口
	}

	// 验证网络接口是否存在
	if !isValidInterface(iface) {
		log.Printf("警告: 网络接口 '%s' 可能不存在", iface)
		log.Printf("可用接口列表:")
		listNetworkInterfaces()
		log.Fatalf("请使用 -i 参数指定正确的网络接口")
	}

	// 检查是否启用 VictoriaMetrics
	metricsEnabled = false
	if enabled := os.Getenv("VICTORIAMETRICS_ENABLED"); enabled == "true" || enabled == "1" {
		metricsEnabled = true

		// 获取 VictoriaMetrics Remote Write URL
		remoteWriteURL := os.Getenv("VICTORIAMETRICS_REMOTE_WRITE")
		if remoteWriteURL == "" {
			remoteWriteURL = "http://localhost:8428/api/v1/import/prometheus" // 默认 VictoriaMetrics URL
		}

		// 获取算网标签
		collectAgg = os.Getenv("COLLECT_AGG")
		if collectAgg == "" {
			collectAgg = "default" // 默认值
		}
		log.Printf("算网标签 (collect_agg): %s", collectAgg)

		// 初始化 VictoriaMetrics metrics
		initVictoriaMetrics(remoteWriteURL)
	}

	// 验证间隔参数
	if intervalMs < 100 {
		log.Fatal("间隔时间必须大于等于100毫秒")
	}
	if intervalMs > 3600000 {
		log.Fatal("间隔时间不能超过3600000毫秒（1小时）")
	}

	// 启动 XDP 监控
	startXDPMonitor(iface, filterTraffic, excludeDNS, intervalMs, countL2)
}
