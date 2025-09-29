# 多阶段构建 Dockerfile for eBPF 网络流量监控器
FROM golang:1.24-bullseye AS builder

# 避免交互式安装
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Asia/Shanghai
ENV CGO_ENABLED=0

# 安装 eBPF 编译依赖
RUN apt-get update && apt-get install -y \
    # eBPF 编译工具
    clang \
    llvm \
    libbpf-dev \
    linux-headers-generic \
    linux-libc-dev \
    # 基础工具
    build-essential \
    pkg-config \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# 创建工作目录
WORKDIR /app

# 复制 Go 模块文件（利用 Docker 缓存）
COPY go.mod go.sum ./

# 下载 Go 依赖
RUN go mod download

# 复制源代码
COPY main.go .
COPY xdp_monitor.c .

# 编译 eBPF 程序
RUN clang -O2 -g -target bpf -c xdp_monitor.c -o xdp_monitor.o \
    -I/usr/include/x86_64-linux-gnu \
    -I/usr/include/asm \
    -I/usr/include/asm-generic \
    -Wall -Wno-unused-value -Wno-pointer-sign \
    -Wno-compare-distinct-pointer-types \
    -Werror

# 编译 Go 程序
RUN go build -ldflags="-s -w" -o xtrace-catch main.go

# 验证编译结果
RUN ls -la xtrace-catch xdp_monitor.o

# ===================
# 运行时镜像 - 使用更小的 Ubuntu
FROM ubuntu:22.04 AS runtime

# 避免交互式安装
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Asia/Shanghai

# 只安装运行时必需的依赖
RUN apt-get update && apt-get install -y \
    # 启用 universe 存储库以获取更多包
    software-properties-common \
    && add-apt-repository universe \
    && apt-get update \
    && apt-get install -y \
    # eBPF 运行时库 (在 Ubuntu 22.04 中可能是 libbpf0 或 libbpf-dev)
    libbpf0 \
    # 网络工具（用于调试）
    iproute2 \
    iputils-ping \
    net-tools \
    # 基础工具
    ca-certificates \
    bash \
    && rm -rf /var/lib/apt/lists/*

# 创建非特权用户
RUN groupadd -r xtrace && useradd -r -g xtrace xtrace

# 创建工作目录
WORKDIR /app

# 从构建镜像复制编译好的程序
COPY --from=builder /app/xtrace-catch /app/
COPY --from=builder /app/xdp_monitor.o /app/

# 创建启动脚本
RUN echo '#!/bin/bash' > /app/entrypoint.sh && \
    echo '' >> /app/entrypoint.sh && \
    echo '# Check BPF filesystem' >> /app/entrypoint.sh && \
    echo 'if [ ! -d /sys/fs/bpf ]; then' >> /app/entrypoint.sh && \
    echo '    echo "❌ BPF filesystem not mounted"' >> /app/entrypoint.sh && \
    echo '    echo "Please use: docker run -v /sys/fs/bpf:/sys/fs/bpf ..."' >> /app/entrypoint.sh && \
    echo '    exit 1' >> /app/entrypoint.sh && \
    echo 'fi' >> /app/entrypoint.sh && \
    echo '' >> /app/entrypoint.sh && \
    echo '# Show system info' >> /app/entrypoint.sh && \
    echo 'echo "=== eBPF Runtime Environment ==="' >> /app/entrypoint.sh && \
    echo 'echo "Kernel: $(uname -r)"' >> /app/entrypoint.sh && \
    echo 'echo "Architecture: $(uname -m)"' >> /app/entrypoint.sh && \
    echo 'echo "Container ID: $(hostname)"' >> /app/entrypoint.sh && \
    echo '' >> /app/entrypoint.sh && \
    echo '# Show network interfaces' >> /app/entrypoint.sh && \
    echo 'echo ""' >> /app/entrypoint.sh && \
    echo 'echo "=== Available Network Interfaces ==="' >> /app/entrypoint.sh && \
    echo 'ip link show | grep -E "^[0-9]+" | awk '"'"'{print "  " $2}'"'"' | sed '"'"'s/:$//'"'"'' >> /app/entrypoint.sh && \
    echo '' >> /app/entrypoint.sh && \
    echo '# Run main program' >> /app/entrypoint.sh && \
    echo 'echo ""' >> /app/entrypoint.sh && \
    echo 'echo "=== Starting Network Traffic Monitor ==="' >> /app/entrypoint.sh && \
    echo 'exec ./xtrace-catch "$@"' >> /app/entrypoint.sh && \
    chmod +x /app/entrypoint.sh

# 设置入口点
ENTRYPOINT ["/app/entrypoint.sh"]

# 默认使用 eth0 接口
CMD ["-i", "eth0"]

# 添加标签
LABEL \
    maintainer="xtrace-catch" \
    description="eBPF Network Traffic Monitor" \
    version="1.0" \
    go.version="1.24" \
    requires.privileged="true" \
    requires.net="host" \
    requires.volumes="/sys/fs/bpf"