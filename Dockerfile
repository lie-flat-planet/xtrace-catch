# 多阶段构建 Dockerfile for eBPF 网络流量监控器
FROM golang:1.24-bullseye AS builder

# 版本信息
ARG VERSION=unknown
LABEL version="${VERSION}" \
      description="XTrace-Catch: XDP 网络流量监控器"

# 环境变量
ENV DEBIAN_FRONTEND=noninteractive \
    TZ=Asia/Shanghai \
    CGO_ENABLED=0

# 安装 eBPF 编译依赖
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        clang \
        llvm \
        libbpf-dev \
        linux-headers-generic \
        linux-libc-dev \
        build-essential \
        pkg-config \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# 工作目录
WORKDIR /app

# 复制 Go 模块文件（利用 Docker 缓存）
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY *.go *.c ./

# 编译 eBPF 程序和 Go 程序
RUN clang -O2 -g -target bpf -c xdp_monitor.c -o xdp_monitor.o \
        -I/usr/include/x86_64-linux-gnu \
        -I/usr/include/asm \
        -I/usr/include/asm-generic \
        -Wall -Wno-unused-value -Wno-pointer-sign \
        -Wno-compare-distinct-pointer-types \
        -Werror && \
    go build -ldflags="-s -w" -o xtrace-catch . && \
    ls -la xtrace-catch xdp_monitor.o

# ===================
# 运行时镜像 - 使用更小的 Ubuntu
FROM ubuntu:22.04 AS runtime

# 版本信息和标签
ARG VERSION=unknown
LABEL version="${VERSION}" \
      description="XTrace-Catch: XDP 网络流量监控器" \
      maintainer="XTrace-Catch Team" \
      go.version="1.24" \
      requires.privileged="true" \
      requires.net="host" \
      requires.volumes="/sys/fs/bpf" \
      mode="xdp"

# 环境变量
ENV DEBIAN_FRONTEND=noninteractive \
    TZ=Asia/Shanghai \
    VERSION=${VERSION}

# 安装运行时依赖 + 配置系统
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        software-properties-common && \
    add-apt-repository universe && \
    apt-get update && \
    apt-get install -y --no-install-recommends \
        libbpf0 \
        iproute2 \
        iputils-ping \
        net-tools \
        ca-certificates \
        bash && \
    rm -rf /var/lib/apt/lists/* && \
    # 设置内存锁定限制
    printf "* soft memlock unlimited\n* hard memlock unlimited\n" >> /etc/security/limits.conf

# 工作目录
WORKDIR /app

# 从构建镜像复制编译好的程序
COPY --from=builder /app/xtrace-catch /app/xdp_monitor.o /app/

# 创建启动脚本（使用 heredoc）
RUN cat > /app/entrypoint.sh <<'EOF' && chmod +x /app/entrypoint.sh
#!/bin/bash
set -e

# Check BPF filesystem
if [ ! -d /sys/fs/bpf ]; then
    echo "❌ BPF filesystem not mounted"
    echo "Please use: docker run -v /sys/fs/bpf:/sys/fs/bpf ..."
    exit 1
fi

# Set memory lock limit
ulimit -l unlimited

# Run main program
echo "=== Starting XDP Network Traffic Monitor ==="
exec ./xtrace-catch "$@"
EOF

# 设置入口点和默认参数
ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["-i", "eth0"]