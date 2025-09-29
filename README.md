# XTrace-Catch: eBPF 网络流量监控器

这是一个基于 eBPF/XDP 技术的网络流量监控工具，用于实时捕获和分析网络数据包。

## ⚠️ 重要说明

**eBPF 和 XDP 是 Linux 内核特有的技术，无法直接在 macOS 上运行。** 本项目需要在 Linux 环境中执行。

## 🚀 在 macOS 上运行

### 使用 Lima（轻量级 Linux VM）

1. **安装 Lima**
   ```bash
   brew install lima
   ```

2. **创建 Linux VM**
   ```bash
   limactl start --name=dev template://ubuntu-lts
   limactl shell dev
   ```

3. **在 VM 中设置项目**
   ```bash
   # 安装依赖
   sudo apt-get update
   sudo apt-get install -y clang llvm libbpf-dev linux-headers-$(uname -r) golang-go

   # 克隆或复制项目到 VM 中
   # 然后执行编译和运行
   make deps
   make build
   sudo make run
   ```

## 🛠️ 本地编译（仅限 Linux）

如果你在 Linux 环境中，可以直接编译运行：

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
```

### 4. 程序特性

- 程序会每 5 秒输出一次网络流量统计
- 按 Ctrl+C 可以安全退出
- 需要 root 权限来加载 eBPF 程序
- 自动验证网络接口是否存在

### 5. 输出格式

```
准备监控网络接口: eth0
XDP program loaded on eth0
192.168.1.100:80 -> 192.168.1.1:12345 proto=6 packets=10 bytes=1500
10.0.0.1:443 -> 10.0.0.5:45678 proto=6 packets=5 bytes=800
```

## 🔧 常见问题

### Q1: 权限不足错误？
A: eBPF 需要 root 权限，请使用 `sudo` 运行程序。

### Q2: 找不到网络接口？
A: 检查网络接口名称，使用 `ip link show` 查看可用接口，常见名称有 `eth0`、`enp0s3` 等。

### Q3: 编译失败，找不到头文件？
A: 确保安装了内核头文件：`sudo apt-get install linux-headers-$(uname -r)`

### Q4: 在虚拟机中看不到网络流量？
A: 确保虚拟机的网络模式允许监控流量，桥接模式通常效果更好。

### Q5: eBPF 程序加载失败？
A: 检查内核版本是否支持 eBPF，通常需要内核版本 >= 4.1。

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

## 🚀 编译命令

```bash
# 安装所有依赖
make deps

# 编译 eBPF 和 Go 程序
make build

# 运行程序
sudo make run

# 检查语法
make check

# 清理编译文件
make clean

# 显示帮助
make help
```

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

本项目使用 GPL 许可证，详见 xdp_monitor.c 中的许可证声明。