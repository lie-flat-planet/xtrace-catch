//go:build linux
// +build linux

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// RDMA 监控相关结构体
type RDMAStats struct {
	DeviceName  string
	PortState   string
	ActiveWidth string
	ActiveSpeed string
	RxBytes     uint64
	TxBytes     uint64
	RxPackets   uint64
	TxPackets   uint64
	RxErrors    uint64
	TxErrors    uint64
	LastUpdate  time.Time
}

type RDMAMonitor struct {
	deviceName    string
	interfaceName string
	stats         RDMAStats
	lastStats     RDMAStats
	startTime     time.Time
}

// RDMA 监控器构造函数
func NewRDMAMonitor(deviceName, interfaceName string) *RDMAMonitor {
	return &RDMAMonitor{
		deviceName:    deviceName,
		interfaceName: interfaceName,
		startTime:     time.Now(),
	}
}

// RDMA 监控器获取统计信息
func (m *RDMAMonitor) GetRDMAStats() error {
	// 获取 ibstat 信息
	cmd := exec.Command("ibstat", m.deviceName)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get ibstat: %v", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "State:") {
			m.stats.PortState = strings.TrimSpace(strings.Split(line, ":")[1])
		} else if strings.Contains(line, "Active width:") {
			m.stats.ActiveWidth = strings.TrimSpace(strings.Split(line, ":")[1])
		} else if strings.Contains(line, "Active speed:") {
			m.stats.ActiveSpeed = strings.TrimSpace(strings.Split(line, ":")[1])
		}
	}

	// 获取网络接口统计
	if err := m.getInterfaceStats(); err != nil {
		log.Printf("获取接口统计失败: %v", err)
	}

	m.stats.LastUpdate = time.Now()
	return nil
}

// 获取网络接口统计
func (m *RDMAMonitor) getInterfaceStats() error {
	// 读取 /proc/net/dev
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.Contains(line, m.interfaceName) {
			fields := strings.Fields(line)
			if len(fields) >= 10 {
				// 格式: interface rx_bytes rx_packets rx_errors ... tx_bytes tx_packets tx_errors
				if rxBytes, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					m.stats.RxBytes = rxBytes
				}
				if rxPackets, err := strconv.ParseUint(fields[2], 10, 64); err == nil {
					m.stats.RxPackets = rxPackets
				}
				if rxErrors, err := strconv.ParseUint(fields[3], 10, 64); err == nil {
					m.stats.RxErrors = rxErrors
				}
				if txBytes, err := strconv.ParseUint(fields[9], 10, 64); err == nil {
					m.stats.TxBytes = txBytes
				}
				if txPackets, err := strconv.ParseUint(fields[10], 10, 64); err == nil {
					m.stats.TxPackets = txPackets
				}
				if txErrors, err := strconv.ParseUint(fields[11], 10, 64); err == nil {
					m.stats.TxErrors = txErrors
				}
			}
			break
		}
	}

	return nil
}

// RDMA 监控器打印统计信息
func (m *RDMAMonitor) PrintStats() {
	fmt.Printf("=== RDMA 监控统计 ===\n")
	fmt.Printf("设备: %s\n", m.deviceName)
	fmt.Printf("接口: %s\n", m.interfaceName)
	fmt.Printf("运行时间: %.2f 秒\n", time.Since(m.startTime).Seconds())
	fmt.Printf("\n")

	fmt.Printf("设备状态:\n")
	fmt.Printf("  端口状态: %s\n", m.stats.PortState)
	fmt.Printf("  活动宽度: %s\n", m.stats.ActiveWidth)
	fmt.Printf("  活动速度: %s\n", m.stats.ActiveSpeed)
	fmt.Printf("\n")

	fmt.Printf("发送统计:\n")
	fmt.Printf("  字节数: %d\n", m.stats.TxBytes)
	fmt.Printf("  数据包: %d\n", m.stats.TxPackets)
	fmt.Printf("  错误数: %d\n", m.stats.TxErrors)
	fmt.Printf("\n")

	fmt.Printf("接收统计:\n")
	fmt.Printf("  字节数: %d\n", m.stats.RxBytes)
	fmt.Printf("  数据包: %d\n", m.stats.RxPackets)
	fmt.Printf("  错误数: %d\n", m.stats.RxErrors)
	fmt.Printf("\n")

	// 计算增量
	if m.lastStats.TxBytes > 0 {
		deltaTxBytes := m.stats.TxBytes - m.lastStats.TxBytes
		deltaRxBytes := m.stats.RxBytes - m.lastStats.RxBytes
		deltaTxPackets := m.stats.TxPackets - m.lastStats.TxPackets
		deltaRxPackets := m.stats.RxPackets - m.lastStats.RxPackets

		fmt.Printf("增量统计 (最近 5 秒):\n")
		fmt.Printf("  发送: %d 字节, %d 数据包\n", deltaTxBytes, deltaTxPackets)
		fmt.Printf("  接收: %d 字节, %d 数据包\n", deltaRxBytes, deltaRxPackets)

		if deltaTxBytes > 0 || deltaRxBytes > 0 {
			totalBytes := deltaTxBytes + deltaRxBytes
			bandwidth := float64(totalBytes) / 5 / (1024 * 1024) // MB/s
			fmt.Printf("  带宽: %.2f MB/s\n", bandwidth)
		}
	}

	m.lastStats = m.stats
}

// RDMA 监控器启动
func (m *RDMAMonitor) Start() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// 捕获 Ctrl+C 退出
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	fmt.Printf("开始 RDMA 监控...\n")
	fmt.Printf("设备: %s, 接口: %s\n", m.deviceName, m.interfaceName)
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
