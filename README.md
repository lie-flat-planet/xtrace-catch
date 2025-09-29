# XTrace-Catch: eBPF 网络流量监控器

这是一个基于 eBPF/XDP 技术的网络流量监控工具，用于实时捕获和分析网络数据包。

## ⚠️ 重要说明

**本项目使用 eBPF 和 XDP 技术，需要在 Linux 环境中运行。** 支持内核版本 4.1+。

## 🛠️ 快速开始

```bash
# 安装依赖
make deps

# 编译程序
make build

# 运行程序（需要 root 权限）
sudo make run
```

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
├── Makefile           # 编译脚本
├── .gitignore         # Git 忽略文件
└── README.md          # 项目说明
```

## 🛡️ 安全考虑

- 本程序需要 root 权限运行
- eBPF 程序会监控所有网络流量，请确保在合适的环境中使用
- 在生产环境中使用前，请充分测试
- 建议在隔离的测试环境中运行

## 📚 技术栈

- **Go 1.21+**: 主程序语言
- **eBPF**: 内核级数据包处理
- **XDP**: 高性能网络数据路径
- **Clang/LLVM**: eBPF 程序编译器

## 🚀 Makefile 命令参考

```bash
# 基本命令
make help         # 显示帮助信息
make deps         # 安装编译依赖
make build        # 编译程序
make clean        # 清理编译文件

# 运行命令
sudo make run                              # 使用默认接口
sudo make run-with-interface INTERFACE=eth0  # 指定接口

# 辅助命令
make check        # 检查代码语法
make interfaces   # 显示可用网络接口
make info         # 显示系统信息
make test-build   # 测试编译
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