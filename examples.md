# XTrace-Catch 使用示例

本文档提供了XTrace-Catch的各种使用场景和示例。

## 🚀 基本使用

### 1. XDP模式（默认，仅入口流量）

```bash
# 监控eth0接口的入口流量
sudo ./xtrace-catch -i eth0

# 监控InfiniBand接口的RoCE流量
sudo ./xtrace-catch -i ibs8f0 -f roce

# 高频监控（每500ms采集一次）
sudo ./xtrace-catch -i eth0 -t 500
```

### 2. TC模式（支持双向流量）

```bash
# 监控出口流量
sudo ./xtrace-catch -i eth0 -m tc -d egress

# 监控入口流量
sudo ./xtrace-catch -i eth0 -m tc -d ingress

# 监控双向流量
sudo ./xtrace-catch -i eth0 -m tc -d both
```

## 🐳 Docker使用示例

### 1. 基本Docker运行

```bash
# XDP模式
docker run --rm --privileged --network host \
  -v /sys/fs/bpf:/sys/fs/bpf:rw \
  xtrace-catch:latest -i eth0

# TC模式监控出口流量
docker run --rm --privileged --network host \
  -v /sys/fs/bpf:/sys/fs/bpf:rw \
  xtrace-catch:latest -i eth0 -m tc -d egress
```

### 2. 使用docker-compose

```bash
# 基本使用（XDP模式）
INTERFACE=eth0 docker-compose up

# TC模式监控出口流量
INTERFACE=eth0 MODE=tc DIRECTION=egress docker-compose up

# 启用VictoriaMetrics推送
INTERFACE=eth0 MODE=tc DIRECTION=both \
VICTORIAMETRICS_ENABLED=true \
VICTORIAMETRICS_REMOTE_WRITE=http://vm-server:8428/api/v1/write \
COLLECT_AGG=cluster-01 \
docker-compose up
```

## 📊 生产环境部署

### 1. 高性能场景（仅需入口监控）

```bash
# 使用XDP模式，最高性能
sudo ./xtrace-catch -i eth0 -m xdp -t 1000 -f roce --exclude-dns
```

### 2. 完整监控场景（需要双向监控）

```bash
# 使用TC模式，监控双向流量
sudo ./xtrace-catch -i eth0 -m tc -d both -t 5000 -f roce
```

### 3. 容器化生产部署

```bash
# 后台运行，完整功能
sudo docker run -d \
  --name xtrace-catch-prod \
  --privileged \
  --network host \
  -v /sys/fs/bpf:/sys/fs/bpf:rw \
  -e VICTORIAMETRICS_ENABLED=true \
  -e VICTORIAMETRICS_REMOTE_WRITE=http://vm-server:8428/api/v1/write \
  -e COLLECT_AGG=production \
  xtrace-catch:latest -i eth0 -m tc -d both -t 10000 --exclude-dns

# 查看日志
docker logs -f xtrace-catch-prod
```

## 🔧 高级配置

### 1. 性能优化

```bash
# 绑定CPU核心
taskset -c 0-3 sudo ./xtrace-catch -i eth0 -m tc -d both

# 设置CPU调度策略
echo performance | sudo tee /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor
```

### 2. 网络优化

```bash
# 增加网络缓冲区
echo 'net.core.rmem_max = 134217728' | sudo tee -a /etc/sysctl.conf
echo 'net.core.wmem_max = 134217728' | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

### 3. 系统限制

```bash
# 设置内存锁定限制
echo '* soft memlock unlimited' | sudo tee -a /etc/security/limits.conf
echo '* hard memlock unlimited' | sudo tee -a /etc/security/limits.conf
```

## 📈 监控和告警

### 1. VictoriaMetrics集成

```bash
# 启用VictoriaMetrics推送
export VICTORIAMETRICS_ENABLED=true
export VICTORIAMETRICS_REMOTE_WRITE=http://vm-server:8428/api/v1/write
export COLLECT_AGG=cluster-01

sudo ./xtrace-catch -i eth0 -m tc -d both
```

### 2. Prometheus查询示例

```promql
# 查询总流量
sum(rate(xtrace_network_bytes_total[5m])) by (interface)

# 查询RoCE流量
sum(rate(xtrace_network_bytes_total{traffic_type="RoCE_v2"}[5m])) by (interface)

# 查询出口流量
sum(rate(xtrace_network_bytes_total{direction="egress"}[5m])) by (interface)
```

## 🧪 测试和调试

### 1. 生成测试流量

```bash
# 使用ping生成基础流量
ping -c 1000 -i 0.01 8.8.8.8

# 使用iperf3生成高带宽流量
iperf3 -c target-server -t 60 -P 4
```

### 2. 调试模式

```bash
# 列出所有网络接口
./xtrace-catch --list

# 显示帮助信息
./xtrace-catch --help

# 查看系统网络接口
ip link show
```

### 3. 性能测试

```bash
# 测试XDP性能
timeout 60 sudo ./xtrace-catch -i eth0 -m xdp -t 1000

# 测试TC性能
timeout 60 sudo ./xtrace-catch -i eth0 -m tc -d both -t 1000
```

## 🔍 故障排除

### 1. 常见问题

```bash
# 检查BPF文件系统
ls -la /sys/fs/bpf/

# 检查TC规则
tc qdisc show dev eth0
tc filter show dev eth0

# 检查系统日志
dmesg | grep -i bpf
journalctl -u docker | grep xtrace
```

### 2. 清理TC规则

```bash
# 清理TC规则
sudo tc qdisc del dev eth0 clsact 2>/dev/null || true
sudo tc filter del dev eth0 ingress 2>/dev/null || true
sudo tc filter del dev eth0 egress 2>/dev/null || true
```

### 3. 权限问题

```bash
# 检查权限
sudo -l
groups

# 检查SELinux状态
getenforce
```

## 📋 最佳实践

### 1. 生产环境建议

- 使用TC模式进行双向监控
- 设置合适的采集间隔（5-10秒）
- 启用VictoriaMetrics推送
- 排除DNS流量减少噪音
- 使用资源限制防止资源耗尽

### 2. 性能优化建议

- 绑定CPU核心提高性能
- 设置CPU调度策略为performance
- 增加网络缓冲区大小
- 使用SSD存储提高I/O性能

### 3. 安全建议

- 使用非特权用户运行（如果可能）
- 限制容器资源使用
- 定期更新镜像版本
- 监控系统资源使用情况

## 🎯 使用场景

### 1. 数据中心网络监控

```bash
# 监控RoCE流量
sudo ./xtrace-catch -i ibs8f0 -m tc -d both -f roce -t 5000
```

### 2. 云原生环境

```bash
# 监控容器网络
sudo ./xtrace-catch -i docker0 -m tc -d both -t 10000
```

### 3. 边缘计算

```bash
# 轻量级监控
sudo ./xtrace-catch -i eth0 -m xdp -t 30000 --exclude-dns
```

### 4. 网络性能测试

```bash
# 高频监控
sudo ./xtrace-catch -i eth0 -m tc -d both -t 100
```

---

更多详细信息请参考 [README.md](README.md) 和 [README_CN.md](README_CN.md)。
