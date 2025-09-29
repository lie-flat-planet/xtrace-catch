//go:build linux
// +build linux

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// NCCL 监控相关结构体
type NCCLStats struct {
	BytesSent       uint64
	BytesReceived   uint64
	PacketsSent     uint64
	PacketsReceived uint64
	Bandwidth       float64
	Latency         float64
}

type NCCLMonitor struct {
	deviceID      int
	interfaceName string
	stats         NCCLStats
	lastStats     NCCLStats
	startTime     time.Time
}

// NCCL 监控器构造函数
func NewNCCLMonitor(deviceID int, interfaceName string) *NCCLMonitor {
	return &NCCLMonitor{
		deviceID:      deviceID,
		interfaceName: interfaceName,
		startTime:     time.Now(),
	}
}

// NCCL 监控器获取统计信息
func (m *NCCLMonitor) GetNCCLStats() error {
	// 简化的 NCCL 统计获取
	// 实际实现需要调用 NCCL 库或硬件接口
	m.stats.BytesSent = uint64(time.Now().Unix() % 1000000)
	m.stats.BytesReceived = uint64(time.Now().Unix() % 1000000)
	m.stats.PacketsSent = uint64(time.Now().Unix() % 10000)
	m.stats.PacketsReceived = uint64(time.Now().Unix() % 10000)

	return nil
}

// NCCL 监控器计算带宽
func (m *NCCLMonitor) CalculateBandwidth() {
	now := time.Now()
	duration := now.Sub(m.startTime).Seconds()

	if duration > 0 {
		totalBytes := m.stats.BytesSent + m.stats.BytesReceived
		m.stats.Bandwidth = float64(totalBytes) / duration / (1024 * 1024) // MB/s
	}
}

// NCCL 监控器打印统计信息
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

// NCCL 监控器启动
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
			if err := m.GetNCCLStats(); err != nil {
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
