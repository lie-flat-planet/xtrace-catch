# XTrace-Catch: eBPF Network Traffic Monitor

A high-performance network traffic monitoring tool based on eBPF/XDP technology, supporting Ethernet and InfiniBand protocols for real-time packet capture and analysis.

## 📖 Language Versions

- **English**: [README.md](./README.md) (Current)
- **中文**: [README_CN.md](./README_CN.md)

## ⚠️ Important Notice

**This project uses eBPF and XDP technology, natively supporting Linux environments.**
- **Linux Systems**: Can run directly, supports kernel version 4.1+
- **macOS/Windows**: Run through Docker (recommended approach)

## 🖥️ Cross-Platform Support

### Linux Systems
- ✅ Native support, best performance
- ✅ Can monitor real host network traffic
- ✅ Supports all network interfaces

### macOS Systems
- ✅ Supported through Docker
- ⚠️ Monitors Docker virtual network traffic
- ⚠️ Requires Docker Desktop

### Windows Systems
- ✅ Supported through Docker Desktop + WSL2
- ⚠️ Monitors WSL2 virtual network traffic

## 🛠️ Quick Start

### Method 1: Using Docker (Recommended)

**No dependencies installation required, one-click run:**

```bash
# XDP mode - monitor traffic through network stack
make docker-up-xdp INTERFACE=eth0

# RDMA mode - monitor RDMA device statistics
make docker-up-rdma INTERFACE=ibs8f0 DEVICE=mlx5_0

# NCCL mode - monitor RDMA hardware statistics
make docker-up-nccl INTERFACE=ibs8f0 DEVICE=mlx5_0

# General method - specify mode through environment variables
make docker-up MODE=rdma INTERFACE=ibs8f0 DEVICE=mlx5_0

# View running logs
make docker-logs

# Stop service
make docker-down
```

### Method 2: Local Compilation

If you prefer local compilation (requires dependency installation):

```bash
# Install dependencies
make deps

# Compile program
make build

# Run program (requires root privileges)
sudo make run
```

## 🐳 Docker Usage Guide

### Quick Run

```bash
# One-click start (recommended)
make docker-up

# View network interface information
make docker-network-info

# Specify specific network interface
make docker-up INTERFACE=eth1

# View real-time logs
make docker-logs
```

### Docker Commands Reference

```bash
# Basic operations
make docker-build     # Build image
make docker-run       # Run container directly
make docker-up        # Start service in background
make docker-down      # Stop service
make docker-logs      # View logs

# Debug and maintenance
make docker-shell     # Enter container shell
make docker-info      # Display Docker environment info
make docker-test      # Quick build test
make docker-clean     # Clean all resources
```

### Direct Docker Command

For advanced users who prefer direct Docker commands:

```bash
# Run with direct Docker command (production-ready)
sudo docker run -d \
  --name xtrace-catch \
  --privileged \
  --network host \
  --restart unless-stopped \
  -v /sys/fs/bpf:/sys/fs/bpf:rw \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -e NETWORK_INTERFACE=ens7f0 \
  xtrace-catch:latest

# View logs
docker logs -f xtrace-catch

# Stop container
docker stop xtrace-catch

# Remove container
docker rm xtrace-catch
```

**Command Parameters Explanation:**
- `--privileged`: Required for eBPF program loading
- `--network host`: Use host network for traffic monitoring
- `--restart unless-stopped`: Auto-restart on system reboot
- `-v /sys/fs/bpf:/sys/fs/bpf:rw`: Mount eBPF filesystem
- `-v /proc:/host/proc:ro`: Read-only access to process information
- `-v /sys:/host/sys:ro`: Read-only access to system information
- `-e NETWORK_INTERFACE=ens7f0`: Specify network interface to monitor

### Docker Advantages

- ✅ **Zero dependency installation** - No need to install eBPF compilation environment
- ✅ **Consistent environment** - All dependencies pre-installed
- ✅ **Avoid network issues** - Image contains all required components
- ✅ **Isolated execution** - No impact on host system
- ✅ **Quick deployment** - One-click start and stop

### 🍎 Using on macOS

**Prerequisites:** Install Docker Desktop

```bash
# 1. Download and install Docker Desktop
# https://www.docker.com/products/docker-desktop

# 2. Start Docker Desktop

# 3. One-click run (will auto-build and start)
make docker-up

# 4. View network traffic monitoring logs
make docker-logs

# 5. Stop monitoring
make docker-down
```

**Testing network traffic on Mac:**
```bash
# In another terminal window, enter container to generate some network traffic
make docker-shell

# Execute in container (generate test traffic)
curl -s http://httpbin.org/get > /dev/null
ping -c 5 8.8.8.8
wget -q -O /dev/null http://example.com
```

**Notes:**
- On Mac, it monitors Docker virtual machine network traffic
- To see more traffic, you can generate network activity inside the container
- Performance may be slightly lower than native Linux, but sufficient for learning and testing

## 📋 Usage Instructions

### 1. Command Line Parameters

```bash
# Show help information
./xtrace-catch -h
./xtrace-catch --help

# List all available network interfaces
./xtrace-catch -l
./xtrace-catch --list

# XDP mode - monitor traffic through network stack
sudo ./xtrace-catch -m xdp -i eth0

# RDMA mode - monitor RDMA device statistics
./xtrace-catch -m rdma -d mlx5_0 -i ibs8f0

# NCCL mode - monitor RDMA hardware statistics
./xtrace-catch -m nccl -d mlx5_0 -i ibs8f0

# Run with default mode
sudo ./xtrace-catch
```

### 2. Network Interface Configuration Priority

The program determines the network interface to monitor in the following priority order:
1. **Command line parameters** - `./xtrace-catch -i eth0`
2. **Environment variables** - `export NETWORK_INTERFACE=eth0`
3. **Default value** - `eth0`

### 3. VictoriaMetrics Integration (XDP Mode Only)

Push metrics to VictoriaMetrics using remote write API through environment variables:

```bash
# Enable VictoriaMetrics push
export VICTORIAMETRICS_ENABLED=true
export VICTORIAMETRICS_REMOTE_WRITE=http://localhost:8428/api/v1/import/prometheus

# Run with VictoriaMetrics enabled
sudo ./xtrace-catch -m xdp -i eth0
```

**Setup VictoriaMetrics:**
```bash
# Run VictoriaMetrics using Docker
docker run -d \
  --name victoriametrics \
  -p 8428:8428 \
  -v victoria-metrics-data:/victoria-metrics-data \
  victoriametrics/victoria-metrics:latest

# Or install locally
# Download from: https://github.com/VictoriaMetrics/VictoriaMetrics/releases
```

**VictoriaMetrics Endpoints:**
- Import endpoint: `http://localhost:8428/api/v1/import/prometheus`
- Query endpoint: `http://localhost:8428/api/v1/query`
- UI dashboard: `http://localhost:8428/vmui`

**Available Metrics:**
- `xtrace_network_bytes_total` - Total network traffic in bytes (Counter)
- `xtrace_network_packets_total` - Total network packets (Counter)
- `xtrace_network_flow_bytes` - Current network flow bytes (Gauge)
- `xtrace_network_flow_packets` - Current network flow packets (Gauge)

All metrics include labels: `src_ip`, `dst_ip`, `src_port`, `dst_port`, `protocol`, `traffic_type`

**Docker Usage:**
```bash
# Run VictoriaMetrics
docker run -d \
  --name victoriametrics \
  -p 8428:8428 \
  -v victoria-metrics-data:/victoria-metrics-data \
  victoriametrics/victoria-metrics:latest

# Run xtrace-catch with VictoriaMetrics push
docker run -d \
  --name xtrace-catch \
  --privileged \
  --network host \
  -e NETWORK_INTERFACE=eth0 \
  -e VICTORIAMETRICS_ENABLED=true \
  -e VICTORIAMETRICS_REMOTE_WRITE=http://localhost:8428/api/v1/import/prometheus \
  xtrace-catch:latest
```

**Query Metrics:**
```bash
# Query specific metrics
curl 'http://localhost:8428/api/v1/query?query=xtrace_network_bytes_total'

# View all metrics
curl 'http://localhost:8428/api/v1/export?match[]=xtrace_network_bytes_total'

# Access VictoriaMetrics UI
open http://localhost:8428/vmui
```

### 4. Using Makefile

```bash
# View all available commands
make help

# Run with default interface
sudo make run

# Run with specified interface
sudo make run-with-interface INTERFACE=enp0s3

# List network interfaces
make interfaces

# Display system information
make info
```

### 4. Program Features

- Program outputs network traffic statistics every 5 seconds
- Press Ctrl+C to safely exit
- Requires root privileges to load eBPF programs
- Automatically validates network interface existence
- High-performance kernel-level packet processing

### 5. Output Format

**Ethernet Traffic:**
```
准备监控网络接口: eth0
XDP program loaded on eth0
192.168.1.100:80 -> 192.168.1.1:12345 proto=6 packets=10 bytes=1500
10.0.0.1:443 -> 10.0.0.5:45678 proto=6 packets=5 bytes=800
```

**InfiniBand Traffic:**
```
准备监控网络接口: ibs8f0
XDP program loaded on ibs8f0
194:0 -> 193:0 proto=8 packets=1000 bytes=65536000
```

**Output Description:**
- **Ethernet traffic**: `proto=6` indicates TCP protocol, `proto=17` indicates UDP protocol
- **InfiniBand traffic**: `194:0` indicates source QPN:LID, `proto=8` indicates RDMA_WRITE opcode
- `packets` is packet count, `bytes` is total byte count

## 🔧 Frequently Asked Questions

### Q1: Permission denied error?
A: eBPF requires root privileges, please run the program with `sudo`.

### Q2: Network interface not found?
A: Use `./xtrace-catch -l` to view available interfaces, or `ip link show` command to view system network interfaces.

### Q3: Compilation failed, header files not found?
A: Ensure kernel headers are installed: `sudo apt-get install linux-headers-$(uname -r)`

### Q4: eBPF program loading failed?
A: Check if kernel version supports eBPF, usually requires kernel version >= 4.1. Use `make info` to view system information.

### Q5: No network traffic visible in virtual machine?
A: Ensure virtual machine network mode allows traffic monitoring, bridge mode usually works better.

### Q6: How to monitor InfiniBand traffic?
A: Use `ibdev2netdev` command to view InfiniBand device corresponding network interfaces, then use that interface to start monitoring:
```bash
# View InfiniBand device mapping
ibdev2netdev

# Start monitoring with corresponding network interface
make docker-up INTERFACE=ibs8f0
```

### Q7: RDMA test has no traffic output?
A: Ensure:
1. Using correct InfiniBand network interface
2. RDMA device status is normal
3. Network interface is in UP state

### Q8: Why can't native InfiniBand traffic be detected?
A: This is an inevitable result of InfiniBand design. Native InfiniBand uses hardware passthrough technology, where packets are transmitted directly between user space and hardware, completely bypassing the kernel network stack, so XDP programs cannot detect them.

**Monitoring Level Comparison:**
- **NCCL and other RDMA tools**: Count all passing vehicles at toll stations (application layer)
- **XDP programs**: Count at certain road sections (network stack), but some vehicles use dedicated lanes (hardware passthrough)

**Solutions:**
1. Use specialized RDMA monitoring tools (like `ibstat`, `ibv_devinfo`)
2. Configure RoCE mode to route RDMA traffic through Ethernet stack
3. Use application layer statistics (like NCCL built-in statistics)

## 📁 Project Structure

```
.
├── main.go              # Go main program
├── xdp_monitor.c        # eBPF C program
├── go.mod              # Go module definition
├── go.sum              # Go dependency checksum
├── Dockerfile          # Docker image build file
├── docker-compose.yml  # Docker Compose configuration
├── .dockerignore       # Docker ignore file
├── Makefile           # Compilation script (supports Docker)
├── .gitignore         # Git ignore file
└── README.md          # Project documentation
```

## 🛡️ Security Considerations

- This program requires root privileges to run
- eBPF programs will monitor all network traffic, please ensure use in appropriate environments
- Please fully test before using in production environments
- Recommend running in isolated test environments

## 📚 Technology Stack

- **Go 1.24**: Main program language
- **eBPF**: Kernel-level packet processing
- **XDP**: High-performance network data path
- **Clang/LLVM**: eBPF program compiler
- **InfiniBand**: RDMA protocol monitoring support (limited support)
- **Docker**: Cross-platform deployment support

### 🔍 Monitoring Capability Description

**This tool can monitor:**
- ✅ Ethernet traffic (TCP/UDP)
- ✅ RoCE traffic (if passing through kernel network stack)
- ✅ InfiniBand traffic encapsulated in Ethernet

**This tool cannot monitor:**
- ❌ Native InfiniBand hardware passthrough traffic
- ❌ RDMA traffic bypassing kernel
- ❌ Traffic processed directly at hardware level

**Why these limitations?**
- **XDP works in kernel network stack**: Can only see packets passing through network stack
- **InfiniBand design goal**: To pursue lowest latency, packets are processed directly by hardware
- **Monitoring level difference**: Application layer tools (like NCCL) can directly access hardware statistics, while kernel layer tools (like XDP) are limited by network stack

## 🚀 Makefile Command Reference

```bash
# Docker operations (recommended)
make docker-up       # Start Docker service
make docker-up-xdp   # Start XDP monitoring mode
make docker-up-rdma  # Start RDMA monitoring mode
make docker-up-nccl  # Start NCCL monitoring mode
make docker-down     # Stop Docker service
make docker-logs     # View running logs
make docker-build    # Build image
make docker-shell    # Enter container shell
make docker-clean    # Clean Docker resources

# Local compilation
make deps         # Install compilation dependencies
make build        # Compile program
sudo make run                              # Use default interface
sudo make run-with-interface INTERFACE=eth0  # Specify interface

# Helper commands
make help         # Show help information
make interfaces   # Show available network interfaces
make info         # Show system information
make clean        # Clean compilation files
```

## 🚀 Performance Features

- **Zero-copy processing** - Process packets directly in network card DMA buffer
- **Kernel space execution** - Avoid user/kernel space switching overhead
- **XDP early interception** - Process at earliest network stack stage, highest performance
- **Atomic operation statistics** - Multi-core safe statistics updates
- **Efficient hash table** - Supports monitoring 10240 network flows simultaneously

## 🤝 Contributing

Welcome to submit Issues and Pull Requests!

## 📄 License

This project uses GPL v3 license, see [LICENSE](./LICENSE) file for details.

---

## 📖 Language Versions

- **English**: [README_EN.md](./README_EN.md) (Current)
- **中文**: [README.md](./README.md)
