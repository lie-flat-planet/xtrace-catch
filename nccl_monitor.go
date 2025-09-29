//go:build linux
// +build linux

package main

import (
	"C"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unsafe"
)

/*
#cgo CFLAGS: -I/usr/local/cuda/include
#cgo LDFLAGS: -lnccl -lrdma
#include <nccl.h>
#include <rdma/rdma_cma.h>
#include <infiniband/verbs.h>
#include <stdlib.h>

// 获取 NCCL 统计信息的 C 函数
typedef struct {
    uint64_t bytes_sent;
    uint64_t bytes_received;
    uint64_t packets_sent;
    uint64_t packets_received;
    double bandwidth;
    double latency;
} nccl_stats_t;

// 获取 RDMA 设备统计
int get_rdma_device_stats(int device_id, nccl_stats_t* stats) {
    struct ibv_device** dev_list;
    struct ibv_context* context;
    struct ibv_device_attr attr;
    int num_devices;
    
    dev_list = ibv_get_device_list(&num_devices);
    if (!dev_list || device_id >= num_devices) {
        return -1;
    }
    
    context = ibv_open_device(dev_list[device_id]);
    if (!context) {
        ibv_free_device_list(dev_list);
        return -1;
    }
    
    if (ibv_query_device(context, &attr) != 0) {
        ibv_close_device(context);
        ibv_free_device_list(dev_list);
        return -1;
    }
    
    // 填充统计信息
    stats->bytes_sent = attr.max_qp_wr * attr.max_sge * 1024; // 估算值
    stats->bytes_received = stats->bytes_sent;
    stats->packets_sent = attr.max_qp_wr;
    stats->packets_received = stats->packets_sent;
    stats->bandwidth = 0.0; // 需要实时计算
    stats->latency = 0.0;   // 需要实时计算
    
    ibv_close_device(context);
    ibv_free_device_list(dev_list);
    return 0;
}

// 获取网络接口统计
int get_interface_stats(const char* iface_name, nccl_stats_t* stats) {
    FILE* fp;
    char line[256];
    char filename[128];
    
    snprintf(filename, sizeof(filename), "/sys/class/net/%s/statistics/rx_bytes", iface_name);
    fp = fopen(filename, "r");
    if (fp) {
        if (fgets(line, sizeof(line), fp)) {
            stats->bytes_received = strtoull(line, NULL, 10);
        }
        fclose(fp);
    }
    
    snprintf(filename, sizeof(filename), "/sys/class/net/%s/statistics/tx_bytes", iface_name);
    fp = fopen(filename, "r");
    if (fp) {
        if (fgets(line, sizeof(line), fp)) {
            stats->bytes_sent = strtoull(line, NULL, 10);
        }
        fclose(fp);
    }
    
    return 0;
}
*/
import "C"

type NCCLStats struct {
	BytesSent      uint64
	BytesReceived  uint64
	PacketsSent    uint64
	PacketsReceived uint64
	Bandwidth      float64
	Latency        float64
}

type NCCLMonitor struct {
	deviceID   int
	interfaceName string
	stats      NCCLStats
	lastStats  NCCLStats
	startTime  time.Time
}

func NewNCCLMonitor(deviceID int, interfaceName string) *NCCLMonitor {
	return &NCCLMonitor{
		deviceID:      deviceID,
		interfaceName: interfaceName,
		startTime:     time.Now(),
	}
}

func (m *NCCLMonitor) GetRDMAStats() error {
	var cStats C.nccl_stats_t
	
	// 获取 RDMA 设备统计
	ret := C.get_rdma_device_stats(C.int(m.deviceID), &cStats)
	if ret != 0 {
		return fmt.Errorf("failed to get RDMA device stats")
	}
	
	// 获取网络接口统计
	ifName := C.CString(m.interfaceName)
	defer C.free(unsafe.Pointer(ifName))
	C.get_interface_stats(ifName, &cStats)
	
	m.stats = NCCLStats{
		BytesSent:      uint64(cStats.bytes_sent),
		BytesReceived:  uint64(cStats.bytes_received),
		PacketsSent:    uint64(cStats.packets_sent),
		PacketsReceived: uint64(cStats.packets_received),
		Bandwidth:      float64(cStats.bandwidth),
		Latency:        float64(cStats.latency),
	}
	
	return nil
}

func (m *NCCLMonitor) CalculateBandwidth() {
	now := time.Now()
	duration := now.Sub(m.startTime).Seconds()
	
	if duration > 0 {
		totalBytes := m.stats.BytesSent + m.stats.BytesReceived
		m.stats.Bandwidth = float64(totalBytes) / duration / (1024 * 1024) // MB/s
	}
}

func (m *NCCLMonitor) PrintStats() {
	m.CalculateBandwidth()
	
	fmt.Printf("=== NCCL RDMA 监控统计 ===\n")
	fmt.Printf("设备 ID: %d\n", m.deviceID)
	fmt.Printf("网络接口: %s\n", m.interfaceName)
	fmt.Printf("运行时间: %.2f 秒\n", time.Since(m.startTime).Seconds())
	fmt.Printf("\n")
	
	fmt.Printf("发送统计:\n")
	fmt.Printf("  字节数: %d\n", m.stats.BytesSent)
	fmt.Printf("  数据包: %d\n", m.stats.PacketsSent)
	fmt.Printf("\n")
	
	fmt.Printf("接收统计:\n")
	fmt.Printf("  字节数: %d\n", m.stats.BytesReceived)
	fmt.Printf("  数据包: %d\n", m.stats.PacketsReceived)
	fmt.Printf("\n")
	
	fmt.Printf("性能统计:\n")
	fmt.Printf("  总带宽: %.2f MB/s\n", m.stats.Bandwidth)
	fmt.Printf("  平均延迟: %.2f μs\n", m.stats.Latency)
	fmt.Printf("\n")
	
	// 计算增量
	if m.lastStats.BytesSent > 0 {
		deltaSent := m.stats.BytesSent - m.lastStats.BytesSent
		deltaReceived := m.stats.BytesReceived - m.lastStats.BytesReceived
		fmt.Printf("增量统计 (最近 5 秒):\n")
		fmt.Printf("  发送增量: %d 字节\n", deltaSent)
		fmt.Printf("  接收增量: %d 字节\n", deltaReceived)
		fmt.Printf("  增量带宽: %.2f MB/s\n", float64(deltaSent+deltaReceived)/5/(1024*1024))
	}
	
	m.lastStats = m.stats
}

func (m *NCCLMonitor) Start() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	// 捕获 Ctrl+C 退出
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	
	fmt.Printf("开始 NCCL RDMA 监控...\n")
	fmt.Printf("设备 ID: %d, 接口: %s\n", m.deviceID, m.interfaceName)
	fmt.Printf("按 Ctrl+C 退出\n\n")
	
	for {
		select {
		case <-ticker.C:
			if err := m.GetRDMAStats(); err != nil {
				log.Printf("获取统计信息失败: %v", err)
				continue
			}
			m.PrintStats()
			fmt.Printf("---\n")
			
		case <-stop:
			fmt.Printf("\n停止监控...\n")
			return
		}
	}
}

func main() {
	var deviceID int
	var interfaceName string
	var showHelp bool
	
	flag.IntVar(&deviceID, "d", 0, "RDMA 设备 ID")
	flag.StringVar(&interfaceName, "i", "ibs8f0", "网络接口名称")
	flag.BoolVar(&showHelp, "h", false, "显示帮助信息")
	flag.Parse()
	
	if showHelp {
		fmt.Printf("NCCL RDMA 监控工具\n\n")
		fmt.Printf("用法: %s [选项]\n\n", os.Args[0])
		fmt.Printf("选项:\n")
		flag.PrintDefaults()
		fmt.Printf("\n示例:\n")
		fmt.Printf("  %s -d 0 -i ibs8f0    # 监控设备 0 和接口 ibs8f0\n", os.Args[0])
		fmt.Printf("  %s -d 1 -i mlx5_0    # 监控设备 1 和接口 mlx5_0\n", os.Args[0])
		return
	}
	
	monitor := NewNCCLMonitor(deviceID, interfaceName)
	monitor.Start()
}
