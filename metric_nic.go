//go:build linux
// +build linux

package main

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

// NIC 流量 Key（按 IP 对聚合，不包含端口）
type NICKey struct {
	SrcIP uint32
	DstIP uint32
	Proto uint8
}

// NIC 速率数据
type NICRate struct {
	bytesPerSec float64
	bitsPerSec  float64
	trafficType string
}

// NIC 速率映射表（按 IP 对聚合）
type NICRates map[NICKey]NICRate

// 创建新的 NIC 速率映射表
func newNICRates() NICRates {
	return make(NICRates)
}

// Add 添加 NIC 流量速率（按 IP 对聚合，不关心端口）
func (r NICRates) Add(srcIP, dstIP uint32, proto uint8,
	bytesPerSec, bitsPerSec float64, trafficType string) {

	key := NICKey{
		SrcIP: srcIP,
		DstIP: dstIP,
		Proto: proto,
	}

	// 如果已经存在该 IP 对，累加速率
	if existing, exists := r[key]; exists {
		r[key] = NICRate{
			bytesPerSec: existing.bytesPerSec + bytesPerSec,
			bitsPerSec:  existing.bitsPerSec + bitsPerSec,
			trafficType: trafficType, // 保留流量类型
		}
	} else {
		r[key] = NICRate{
			bytesPerSec: bytesPerSec,
			bitsPerSec:  bitsPerSec,
			trafficType: trafficType,
		}
	}
}

// UpdateMetrics 更新 NIC 速率 metrics（按 IP 对聚合）
func (r NICRates) UpdateMetrics(iface, hostIP string) {
	for nicKey, rate := range r {
		labels := prometheus.Labels{
			"interface":    iface,
			"src_ip":       ipToStr(nicKey.SrcIP),
			"dst_ip":       ipToStr(nicKey.DstIP),
			"protocol":     strconv.Itoa(int(nicKey.Proto)),
			"traffic_type": rate.trafficType,
			"host_ip":      hostIP,
			"collect_agg":  collectAgg,
		}
		networkNICBytesRate.With(labels).Set(rate.bytesPerSec)
		networkNICBitsRate.With(labels).Set(rate.bitsPerSec)
	}
}
