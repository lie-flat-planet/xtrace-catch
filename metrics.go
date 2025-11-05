//go:build linux
// +build linux

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/prometheus/prompb"
)

// VictoriaMetrics metrics (全局变量)
var (
	metricsEnabled       bool
	vmRemoteWriteURL     string
	vmRegistry           *prometheus.Registry
	networkFlowBytesRate *prometheus.GaugeVec // bytes/s 速率
	networkFlowBitsRate  *prometheus.GaugeVec // bits/s 速率（Mbps）
	networkNICBytesRate  *prometheus.GaugeVec // NIC网卡的速率 bytes/s
	networkNICBitsRate   *prometheus.GaugeVec // NIC网卡的速率 bits/s
	collectAgg           string               // 算网标签
)

// 初始化 VictoriaMetrics metrics
func initVictoriaMetrics(remoteWriteURL string) {
	// 创建独立的 registry
	vmRegistry = prometheus.NewRegistry()

	networkFlowBytesRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "xtrace_network_flow_bytes_rate",
			Help: "Network flow rate in bytes per second (compatible with node_exporter irate)",
		},
		[]string{"src_ip", "dst_ip", "src_port", "dst_port", "protocol", "traffic_type", "interface", "host_ip", "collect_agg"},
	)

	networkFlowBitsRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "xtrace_network_flow_bits_rate",
			Help: "Network flow rate in bits per second (Mbps when divided by 1e6)",
		},
		[]string{"src_ip", "dst_ip", "src_port", "dst_port", "protocol", "traffic_type", "interface", "host_ip", "collect_agg"},
	)

	networkNICBytesRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "xtrace_network_nic_bytes_rate",
			Help: "Network traffic rate per NIC interface in bytes per second (aggregated by IP pair)",
		},
		[]string{"interface", "src_ip", "dst_ip", "protocol", "traffic_type", "host_ip", "collect_agg"},
	)

	networkNICBitsRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "xtrace_network_nic_bits_rate",
			Help: "Network traffic rate per NIC interface in bits per second (aggregated by IP pair)",
		},
		[]string{"interface", "src_ip", "dst_ip", "protocol", "traffic_type", "host_ip", "collect_agg"},
	)

	// 注册 metrics 到独立的 registry
	vmRegistry.MustRegister(networkFlowBytesRate)
	vmRegistry.MustRegister(networkFlowBitsRate)
	vmRegistry.MustRegister(networkNICBytesRate)
	vmRegistry.MustRegister(networkNICBitsRate)

	vmRemoteWriteURL = remoteWriteURL

	// 检测使用的协议格式
	format := "Text Format"
	if strings.Contains(remoteWriteURL, "/api/v1/write") {
		format = "Remote Write Protocol (Protobuf + Snappy)"
	}
	log.Printf("VictoriaMetrics Remote Write 配置: %s [%s]", remoteWriteURL, format)
}

// 推送 metrics 到 VictoriaMetrics
func pushMetricsToVictoriaMetrics() error {
	// 收集所有 metrics
	metricsFamilies, err := vmRegistry.Gather()
	if err != nil {
		return fmt.Errorf("收集 metrics 失败: %w", err)
	}

	// 根据 URL 判断使用哪种格式
	useRemoteWrite := strings.Contains(vmRemoteWriteURL, "/api/v1/write")

	var req *http.Request
	if useRemoteWrite {
		// 使用 Prometheus Remote Write Protocol (Protobuf + Snappy)
		req, err = createRemoteWriteRequest(metricsFamilies)
	} else {
		// 使用 Prometheus Text Format
		req, err = createTextFormatRequest(metricsFamilies)
	}

	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("VictoriaMetrics 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// 创建 Text Format 请求
func createTextFormatRequest(metricsFamilies []*dto.MetricFamily) (*http.Request, error) {
	var buf bytes.Buffer
	encoder := expfmt.NewEncoder(&buf, expfmt.FmtText)
	for _, mf := range metricsFamilies {
		if err := encoder.Encode(mf); err != nil {
			return nil, fmt.Errorf("编码 metrics 失败: %w", err)
		}
	}

	req, err := http.NewRequest("POST", vmRemoteWriteURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")
	return req, nil
}

// 创建 Remote Write 请求（Protobuf + Snappy）
func createRemoteWriteRequest(metricsFamilies []*dto.MetricFamily) (*http.Request, error) {
	// 转换为 Prometheus Remote Write format
	writeRequest := &prompb.WriteRequest{}

	for _, mf := range metricsFamilies {
		for _, m := range mf.Metric {
			ts := &prompb.TimeSeries{}

			// 添加 metric name label
			ts.Labels = append(ts.Labels, prompb.Label{
				Name:  "__name__",
				Value: *mf.Name,
			})

			// 添加其他 labels
			for _, label := range m.Label {
				ts.Labels = append(ts.Labels, prompb.Label{
					Name:  *label.Name,
					Value: *label.Value,
				})
			}

			// 添加样本值
			now := time.Now().UnixMilli()
			var value float64

			switch mf.GetType() {
			case dto.MetricType_COUNTER:
				if m.Counter != nil {
					value = *m.Counter.Value
				}
			case dto.MetricType_GAUGE:
				if m.Gauge != nil {
					value = *m.Gauge.Value
				}
			case dto.MetricType_UNTYPED:
				if m.Untyped != nil {
					value = *m.Untyped.Value
				}
			}

			ts.Samples = []prompb.Sample{{
				Value:     value,
				Timestamp: now,
			}}

			writeRequest.Timeseries = append(writeRequest.Timeseries, *ts)
		}
	}

	// 序列化为 Protobuf
	data, err := proto.Marshal(writeRequest)
	if err != nil {
		return nil, fmt.Errorf("protobuf 编码失败: %w", err)
	}

	// 使用 Snappy 压缩
	compressed := snappy.Encode(nil, data)

	req, err := http.NewRequest("POST", vmRemoteWriteURL, bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	return req, nil
}
