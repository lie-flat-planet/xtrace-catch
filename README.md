# XTrace-Catch: eBPF 网络流量监控器

这是一个基于 eBPF/XDP 技术的网络流量监控工具，用于实时捕获和分析网络数据包。

## ⚠️ 重要说明

**本项目使用 eBPF 和 XDP 技术，原生支持 Linux 环境。** 
- **Linux 系统**：可以直接运行，支持内核版本 4.1+
- **macOS/Windows**：通过 Docker 运行（推荐方式）

## 🖥️ 跨平台支持

### Linux 系统
- ✅ 原生支持，性能最佳
- ✅ 可以监控真实的主机网络流量
- ✅ 支持所有网络接口

### macOS 系统  
- ✅ 通过 Docker 支持
- ⚠️ 监控的是 Docker 虚拟网络流量
- ⚠️ 需要 Docker Desktop

### Windows 系统
- ✅ 通过 Docker Desktop + WSL2 支持
- ⚠️ 监控的是 WSL2 虚拟网络流量

## 🛠️ 快速开始

### 方法1：使用 Docker（推荐）

**无需安装任何依赖，一键运行：**

```bash
# 最简单的方式 - 使用默认网络接口
make docker-up

# 指定网络接口
make docker-up INTERFACE=enp0s3

# 查看运行日志
make docker-logs

# 停止服务
make docker-down
```

### 方法2：本地编译

如果你喜欢本地编译（需要安装依赖）：

```bash
# 安装依赖
make deps

# 编译程序
make build

# 运行程序（需要 root 权限）
sudo make run
```

## 🐳 Docker 使用指南

### 快速运行

```bash
# 一键启动（推荐）
make docker-up

# 查看网络接口信息
make docker-network-info

# 指定特定网络接口
make docker-up INTERFACE=eth1

# 查看实时日志
make docker-logs
```

### Docker 命令详解

```bash
# 基础操作
make docker-build     # 构建镜像
make docker-run       # 直接运行容器
make docker-up        # 后台启动服务
make docker-down      # 停止服务
make docker-logs      # 查看日志

# 调试和维护
make docker-shell     # 进入容器 Shell
make docker-info      # 显示 Docker 环境信息
make docker-test      # 快速测试构建
make docker-clean     # 清理所有资源
```

### Docker 优势

- ✅ **零依赖安装** - 无需安装 eBPF 编译环境
- ✅ **一致性环境** - 所有依赖都已预装
- ✅ **避免网络问题** - 镜像包含所有必需组件
- ✅ **隔离运行** - 不影响主机系统
- ✅ **快速部署** - 一键启动和停止

### 🍎 在 macOS 上使用

**前提条件：** 安装 Docker Desktop

```bash
# 1. 下载并安装 Docker Desktop
# https://www.docker.com/products/docker-desktop

# 2. 启动 Docker Desktop

# 3. 一键运行（会自动构建并启动）
make docker-up

# 4. 查看网络流量监控日志
make docker-logs

# 5. 停止监控
make docker-down
```

**在 Mac 上测试网络流量：**
```bash
# 在另一个终端窗口中，进入容器生成一些网络流量
make docker-shell

# 在容器内执行（生成测试流量）
curl -s http://httpbin.org/get > /dev/null
ping -c 5 8.8.8.8
wget -q -O /dev/null http://example.com
```

**注意事项：**
- 在 Mac 上会监控 Docker 虚拟机的网络流量
- 如果想看到更多流量，可以在容器内生成网络活动
- 性能可能略低于原生 Linux，但足够用于学习和测试

## 📋 使用说明

### 1. 命令行参数

```bash
# 显示帮助信息
./xtrace-catch -h
./xtrace-catch --help

# 列出所有可用的网络接口
./xtrace-catch -l
./xtrace-catch --list

# 指定网络接口运行 (推荐)
sudo ./xtrace-catch -i eth0
sudo ./xtrace-catch --interface enp0s3

# 使用默认接口运行
sudo ./xtrace-catch
```

### 2. 网络接口配置优先级

程序按以下优先级确定要监控的网络接口：
1. **命令行参数** - `./xtrace-catch -i eth0`
2. **环境变量** - `export NETWORK_INTERFACE=eth0`
3. **默认值** - `eth0`

### 3. 使用 Makefile

```bash
# 查看所有可用命令
make help

# 使用默认接口运行
sudo make run

# 指定接口运行
sudo make run-with-interface INTERFACE=enp0s3

# 列出网络接口
make interfaces

# 显示系统信息
make info
```

### 4. 程序特性

- 程序会每 5 秒输出一次网络流量统计
- 按 Ctrl+C 可以安全退出
- 需要 root 权限来加载 eBPF 程序
- 自动验证网络接口是否存在
- 高性能内核级数据包处理

### 5. 输出格式

```
准备监控网络接口: eth0
XDP program loaded on eth0
192.168.1.100:80 -> 192.168.1.1:12345 proto=6 packets=10 bytes=1500
10.0.0.1:443 -> 10.0.0.5:45678 proto=6 packets=5 bytes=800
```

**输出说明：**
- `proto=6` 表示 TCP 协议
- `proto=17` 表示 UDP 协议
- `packets` 为数据包数量
- `bytes` 为总字节数

## 🔧 常见问题

### Q1: 权限不足错误？
A: eBPF 需要 root 权限，请使用 `sudo` 运行程序。

### Q2: 找不到网络接口？
A: 使用 `./xtrace-catch -l` 查看可用接口，或 `ip link show` 命令查看系统网络接口。

### Q3: 编译失败，找不到头文件？
A: 确保安装了内核头文件：`sudo apt-get install linux-headers-$(uname -r)`

### Q4: eBPF 程序加载失败？
A: 检查内核版本是否支持 eBPF，通常需要内核版本 >= 4.1。使用 `make info` 查看系统信息。

### Q5: 在虚拟机中看不到网络流量？
A: 确保虚拟机的网络模式允许监控流量，桥接模式通常效果更好。

## 📁 项目结构

```
.
├── main.go              # Go 主程序
├── xdp_monitor.c        # eBPF C 程序
├── go.mod              # Go 模块定义
├── go.sum              # Go 依赖校验
├── Dockerfile          # Docker 镜像构建文件
├── docker-compose.yml  # Docker Compose 配置
├── .dockerignore       # Docker 忽略文件
├── Makefile           # 编译脚本（支持 Docker）
├── .gitignore         # Git 忽略文件
└── README.md          # 项目说明
```

## 🛡️ 安全考虑

- 本程序需要 root 权限运行
- eBPF 程序会监控所有网络流量，请确保在合适的环境中使用
- 在生产环境中使用前，请充分测试
- 建议在隔离的测试环境中运行

## 📚 技术栈

- **Go 1.24**: 主程序语言
- **eBPF**: 内核级数据包处理
- **XDP**: 高性能网络数据路径
- **Clang/LLVM**: eBPF 程序编译器

## 🚀 Makefile 命令参考

```bash
# Docker 操作（推荐）
make docker-up       # 启动 Docker 服务
make docker-down     # 停止 Docker 服务
make docker-logs     # 查看运行日志
make docker-build    # 构建镜像
make docker-clean    # 清理 Docker 资源

# 本地编译
make deps         # 安装编译依赖
make build        # 编译程序
sudo make run                              # 使用默认接口
sudo make run-with-interface INTERFACE=eth0  # 指定接口

# 辅助命令
make help         # 显示帮助信息
make check        # 检查代码语法
make interfaces   # 显示可用网络接口
make info         # 显示系统信息
make clean        # 清理编译文件
```

## 🚀 性能特性

- **零拷贝处理** - 直接在网卡 DMA 缓冲区处理数据包
- **内核空间执行** - 避免用户态/内核态切换开销
- **XDP 早期拦截** - 在网络栈最早期处理，性能最高
- **原子操作统计** - 多核安全的统计更新
- **高效哈希表** - 支持同时监控 10240 个网络流

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

本项目使用 GPL 许可证，详见 xdp_monitor.c 中的许可证声明。