# XTrace-Catch: XDP Network Traffic Monitor

A high-performance network traffic monitoring tool based on eBPF/XDP technology, focused on real-time packet capture and analysis with support for RoCE/InfiniBand traffic monitoring.

[中文文档](README_CN.md)

## ✨ Features

- 🚀 **High Performance**: Based on XDP technology, captures packets at the earliest stage of the network stack
- 📊 **Low Overhead**: CPU usage < 5%, minimal impact on system performance
- 🔍 **Traffic Identification**: Automatically identifies TCP, UDP, RoCE v1/v2, InfiniBand traffic
- 📈 **Metrics Push**: Supports pushing to VictoriaMetrics (Prometheus compatible)
- 🎯 **Traffic Filtering**: Filter by protocol type (roce, tcp, udp, etc.)
- 🐳 **Containerized**: One-command Docker deployment, no manual dependency installation

## 🛠️ Quick Start

### Method 1: Docker (Recommended)

```bash
# Basic usage
docker run --rm --privileged --network host \
  -v /sys/fs/bpf:/sys/fs/bpf:rw \
  xtrace-catch:latest -i eth0

# Filter RoCE traffic
docker run --rm --privileged --network host \
  -v /sys/fs/bpf:/sys/fs/bpf:rw \
  xtrace-catch:latest -i ib0 -f roce

# Using docker-compose
INTERFACE=eth0 docker-compose up
```

### Method 2: Local Build

```bash
# Build
make build

# Run (requires root privileges)
sudo ./xtrace-catch -i eth0

# Filter RoCE traffic
sudo ./xtrace-catch -i ib0 -f roce
```

## 📋 System Requirements

### Linux System
- Kernel version: 4.1+ (5.4+ recommended)
- Root privileges required (for loading eBPF programs)

### Dependencies
```bash
# Ubuntu/Debian
sudo apt-get install -y clang llvm libbpf-dev linux-headers-$(uname -r)

# RHEL/CentOS
sudo yum install -y clang llvm libbpf-devel kernel-devel
```

## 🎯 Usage

### Command Line Arguments

```bash
./xtrace-catch [options]

Options:
  -i, --interface string   Network interface name (default: eth0)
  -f, --filter string      Filter traffic type: roce, roce_v1, roce_v2, tcp, udp, ib, all
  -t, --interval int       Data collection and push interval (milliseconds), default 5000ms, range 100-3600000
  --exclude-dns           Exclude DNS traffic (filters common DNS servers)
  -h, --help              Show help message
  -l, --list              List all available network interfaces
```

### Traffic Filtering

```bash
# Show all RoCE traffic (v1 + v2)
sudo ./xtrace-catch -i ib0 -f roce

# Show only RoCE v2 traffic
sudo ./xtrace-catch -i ib0 -f roce_v2

# Show only TCP traffic
sudo ./xtrace-catch -i eth0 -f tcp

# Exclude DNS traffic (223.5.5.5, 8.8.8.8, etc.)
sudo ./xtrace-catch -i eth0 --exclude-dns

# Collect data every 500ms (high frequency monitoring)
sudo ./xtrace-catch -i eth0 -t 500

# Collect data every 10 seconds (reduce data volume)
sudo ./xtrace-catch -i eth0 -t 10000

# Every 30 seconds, RoCE only, exclude DNS
sudo ./xtrace-catch -i ib0 -f roce -t 30000 --exclude-dns

# Show all traffic (default 5000ms)
sudo ./xtrace-catch -i eth0
```

### Output Example

```
192.168.1.10:45678 -> 192.168.1.20:4791 proto=17 [RoCE v2/UDP] packets=1500 bytes=2048000 host_ip=192.168.1.10
10.0.0.1:0 -> 10.0.0.2:0 proto=21 [RoCE v1/IBoE] packets=2500 bytes=3072000 host_ip=192.168.1.10
192.168.1.30:80 -> 192.168.1.40:50234 proto=6 [TCP] packets=100 bytes=65536 host_ip=192.168.1.10
```

## 📊 VictoriaMetrics Integration

### Environment Variables

```bash
export VICTORIAMETRICS_ENABLED=true
export VICTORIAMETRICS_REMOTE_WRITE=http://vm-server:8428/api/v1/write
export COLLECT_AGG=cluster-01

sudo ./xtrace-catch -i ib0 -f roce
```

### Docker Run

#### Basic Example

```bash
# Run in foreground, monitoring only
docker run --rm --privileged --network host \
  -v /sys/fs/bpf:/sys/fs/bpf:rw \
  xtrace-catch:latest -i eth0
```

#### Complete Example (with VictoriaMetrics + DNS filtering)

```bash
# Run in background, full features
sudo docker run -d \
  --name xtrace-catch-eth0 \
  --privileged \
  --network host \
  -v /sys/fs/bpf:/sys/fs/bpf:rw \
  -e VICTORIAMETRICS_ENABLED=true \
  -e VICTORIAMETRICS_REMOTE_WRITE=http://<your-vm-server>:8428/api/v1/write \
  -e COLLECT_AGG=<your-cluster-name> \
  <your-registry>/xtrace-catch:latest -i eth0 -t 10000 --exclude-dns

# View logs
docker logs -f xtrace-catch-eth0

# Stop container
docker stop xtrace-catch-eth0
```

### Supported Endpoint Formats

- **Text Format**: `http://<vm-server>:8428/api/v1/import/prometheus`
- **Remote Write**: `http://<vm-server>:8428/api/v1/write` (Protobuf + Snappy)

The program automatically detects the URL format and selects the correct encoding.

### Metrics Description

Pushed metrics include the following labels:
- `src_ip`, `dst_ip`: Source/destination IP addresses
- `src_port`, `dst_port`: Source/destination ports
- `protocol`: Protocol number
- `traffic_type`: Traffic type (RoCE_v2, TCP, UDP, etc.)
- `interface`: Network interface name
- `host_ip`: Host IP address
- `collect_agg`: Custom label (for distinguishing clusters/nodes)

Metric names:
- `xtrace_network_bytes_total`: Total traffic bytes (Counter)
- `xtrace_network_packets_total`: Total packet count (Counter)
- `xtrace_network_flow_bytes`: Current flow bytes (Gauge)
- `xtrace_network_flow_packets`: Current flow packets (Gauge)

## 🐳 Docker Deployment

### Build Image

```bash
# Using Makefile
make docker-build

# Or build directly
docker build -t xtrace-catch:latest .
```

### Using docker-compose

Edit `docker-compose.yml` configuration file:

```yaml
version: '3.8'

services:
  xtrace-catch:
    image: xtrace-catch:latest
    container_name: xtrace-catch
    privileged: true
    network_mode: host
    volumes:
      - /sys/fs/bpf:/sys/fs/bpf
    environment:
      - NETWORK_INTERFACE=eth0
      - VICTORIAMETRICS_ENABLED=true
      - VICTORIAMETRICS_REMOTE_WRITE=http://vm-server:8428/api/v1/write
      - COLLECT_AGG=cluster-01
    command: ["-i", "eth0", "-f", "roce"]
    restart: unless-stopped
```

Run:
```bash
# Start
docker-compose up -d

# View logs
docker-compose logs -f

# Stop
docker-compose down
```

## 🔧 RoCE Traffic Monitoring

XTrace-Catch supports monitoring the following RoCE traffic:

### RoCE v1 (IBoE)
- Ethernet protocol type: `0x8915`
- Transmitted directly on Ethernet frames

### RoCE v2
- Uses UDP protocol
- Destination port: `4791`
- Supports IP routing

### Output Example

```bash
# RoCE v2 traffic
192.168.0.84:4791 -> 192.168.0.85:4791 proto=254 [RoCE v2] packets=1500 bytes=2048000

# RoCE v1/IBoE traffic
1.0.0.0:0 -> 2.0.0.0:0 proto=21 [RoCE v1/IBoE] packets=2500 bytes=3072000
```

## 📁 Project Structure

```
xtrace-catch/
├── main.go            # Main program entry
├── xdp_monitor.go     # XDP monitoring implementation
├── metrics.go         # VictoriaMetrics push logic
├── xdp_monitor.c      # eBPF/XDP program (C code)
├── Makefile           # Build script
├── Dockerfile         # Docker image build
├── docker-compose.yml # Docker Compose configuration
└── README.md          # Documentation
```

## 🤝 FAQ

### Q1: Why does it require --privileged permission?

eBPF programs need to be loaded into the kernel, which requires privileged mode. This is a security requirement of eBPF technology.

### Q2: Can it be used in production?

Yes. XDP technology is designed for production environments with minimal performance overhead (< 5% CPU) and no impact on network performance.

### Q3: Which network interfaces are supported?

All standard Linux network interfaces are supported, including:
- Ethernet interfaces (eth0, ens33, etc.)
- InfiniBand interfaces (ib0, ib1, etc.)
- Virtual interfaces (veth, bridge, etc.)

### Q4: Why can't I see any traffic?

Check the following:
1. Is the network interface name correct? (use `-l` to list all interfaces)
2. Is there actual network traffic on that interface?
3. Are you using the correct traffic filter parameters?
4. Is a firewall or security policy blocking the traffic?

### Q5: What's the difference from tcpdump?

| Feature | XTrace-Catch (XDP) | tcpdump |
|---------|-------------------|---------|
| Performance overhead | Very low (< 5%) | Medium (10-20%) |
| Capture location | Kernel earliest stage (NIC driver) | After network stack |
| RoCE support | ✅ Native | ⚠️ Partial |
| Real-time | ✅ Very high | ⚠️ Medium |
| Memory usage | Very low (~1MB) | Higher (depends on buffer) |

### Q6: VictoriaMetrics push failed?

1. Check if the URL is correct
2. Verify VictoriaMetrics service is accessible
3. Check error logs for detailed information
4. Test network connection: `curl -X POST <vm-url>`

## 📊 Performance Metrics

Test results in 100 Gbps network environment:

| Network Load | CPU Usage | Memory | Latency Increase |
|-------------|-----------|---------|------------------|
| 1 Gbps      | < 1%      | ~1 MB   | < 1 μs          |
| 10 Gbps     | 1-3%      | ~2 MB   | < 2 μs          |
| 100 Gbps    | 3-8%      | ~5 MB   | < 5 μs          |

## 📝 Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NETWORK_INTERFACE` | Network interface name | `eth0` |
| `VICTORIAMETRICS_ENABLED` | Enable VictoriaMetrics | `false` |
| `VICTORIAMETRICS_REMOTE_WRITE` | VictoriaMetrics URL | `http://localhost:8428/api/v1/import/prometheus` |
| `COLLECT_AGG` | Custom aggregation label | `default` |

## 📜 License

This project is licensed under the Apache License 2.0.

## 🙋 Support

For questions or suggestions, please submit an Issue or Pull Request.

---

**Note**: This tool requires Linux kernel 4.1+ support. Kernel 5.4+ is recommended for best performance and stability.
