//go:build linux
// +build linux

package main

import (
	"encoding/binary"
	"net"
	"strconv"
)

// RoCE v2 使用的 UDP 端口（网络字节序：4791 = 0xb712）
const roceV2Port = uint16(0xb712)

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

// ConvertPorts 转换端口号从网络字节序到主机字节序
func (k *FlowKey) ConvertPorts() (srcPort, dstPort uint16) {
	srcPort = binary.BigEndian.Uint16([]byte{byte(k.SrcPort >> 8), byte(k.SrcPort)})
	dstPort = binary.BigEndian.Uint16([]byte{byte(k.DstPort >> 8), byte(k.DstPort)})
	return srcPort, dstPort
}

// GetTrafficType 获取流量类型字符串
func (k *FlowKey) GetTrafficType() string {
	srcPort, dstPort := k.ConvertPorts()

	switch k.Proto {
	case 0xFE:
		return "RoCE_v2"
	case 6:
		return "TCP"
	case 17:
		if srcPort == roceV2Port || dstPort == roceV2Port {
			return "RoCE_v2_UDP"
		}
		return "UDP"
	case 0x15:
		return "RoCE_v1_IBoE"
	case 0x14:
		return "InfiniBand"
	default:
		return "Other"
	}
}

// CalculateDelta 计算流量增量（处理计数器回绕）
func (current FlowStats) CalculateDelta(last FlowStats, exists bool) (deltaPackets, deltaBytes uint64) {
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

// CalculateRates 计算流量速率
func CalculateRates(deltaBytes uint64, intervalSeconds float64) (bytesPerSec, bitsPerSec float64) {
	bytesPerSec = float64(deltaBytes) / intervalSeconds
	bitsPerSec = bytesPerSec * 8
	return bytesPerSec, bitsPerSec
}

// UpdateMetrics 更新流级别速率 metrics
func (k *FlowKey) UpdateMetrics(srcPort, dstPort uint16, trafficType string,
	bytesPerSec, bitsPerSec float64, iface, hostIP string) {

	labels := map[string]string{
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
}
