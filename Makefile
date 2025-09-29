.PHONY: help deps build run check clean

# 默认目标
.DEFAULT_GOAL := help

# 程序名称
PROGRAM := xtrace-catch
BPF_OBJ := xdp_monitor.o

# 编译器设置
CLANG := clang
GO := go

# 帮助信息
help: ## 显示帮助信息
	@echo "XTrace-Catch eBPF 网络流量监控器"
	@echo ""
	@echo "可用命令："
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# 安装依赖 (仅适用于 Ubuntu/Debian)
deps: ## 安装编译依赖
	@echo "安装 eBPF 编译依赖..."
	sudo apt-get update
	sudo apt-get install -y clang llvm libbpf-dev linux-headers-$$(uname -r)
	@echo "检查 Go 版本..."
	@$(GO) version || (echo "请安装 Go 1.21+ 版本" && exit 1)

# 编译 eBPF 程序
$(BPF_OBJ): xdp_monitor.c
	@echo "编译 eBPF 程序..."
	$(CLANG) -O2 -target bpf -c xdp_monitor.c -o $(BPF_OBJ)

# 编译 Go 程序
$(PROGRAM): $(BPF_OBJ) main.go go.mod
	@echo "编译 Go 程序..."
	$(GO) build -o $(PROGRAM) main.go

# 构建所有程序
build: $(PROGRAM) ## 编译 eBPF 和 Go 程序

# 运行程序 (需要 root 权限)
run: build ## 运行程序 (需要 sudo 权限)
	@echo "启动网络流量监控..."
	@if [ "$$(id -u)" != "0" ]; then \
		echo "需要 root 权限，请使用: sudo make run"; \
		exit 1; \
	fi
	./$(PROGRAM)

# 运行程序并指定网络接口
run-with-interface: build ## 运行程序并指定网络接口 (INTERFACE=eth0)
	@echo "启动网络流量监控 (接口: $(INTERFACE))..."
	@if [ "$$(id -u)" != "0" ]; then \
		echo "需要 root 权限，请使用: sudo make run-with-interface INTERFACE=eth0"; \
		exit 1; \
	fi
	@if [ -z "$(INTERFACE)" ]; then \
		echo "请指定网络接口: make run-with-interface INTERFACE=eth0"; \
		exit 1; \
	fi
	./$(PROGRAM) -i $(INTERFACE)

# 检查语法和格式
check: ## 检查代码语法
	@echo "检查 Go 代码..."
	$(GO) vet ./...
	$(GO) fmt ./...
	@echo "检查 eBPF 程序语法..."
	$(CLANG) -fsyntax-only -target bpf xdp_monitor.c

# 显示网络接口
interfaces: ## 显示可用的网络接口
	@echo "可用的网络接口："
	@ip link show | grep -E "^[0-9]+:" | awk '{print "  " $$2}' | sed 's/://'

# 测试编译
test-build: ## 测试编译但不生成可执行文件
	@echo "测试 eBPF 编译..."
	$(CLANG) -fsyntax-only -target bpf xdp_monitor.c
	@echo "测试 Go 编译..."
	$(GO) build -o /dev/null main.go
	@echo "编译测试通过！"

# 清理编译文件
clean: ## 清理编译生成的文件
	@echo "清理编译文件..."
	rm -f $(PROGRAM) $(BPF_OBJ)

# 显示系统信息
info: ## 显示系统和依赖信息
	@echo "系统信息："
	@echo "  内核版本: $$(uname -r)"
	@echo "  架构: $$(uname -m)"
	@echo "  发行版: $$(lsb_release -d 2>/dev/null | cut -f2 || echo 'Unknown')"
	@echo ""
	@echo "工具版本："
	@echo "  Clang: $$($(CLANG) --version | head -1 || echo 'Not installed')"
	@echo "  Go: $$($(GO) version || echo 'Not installed')"
	@echo ""
	@echo "eBPF 支持检查："
	@if [ -d "/sys/fs/bpf" ]; then \
		echo "  BPF 文件系统: ✓ 已挂载"; \
	else \
		echo "  BPF 文件系统: ✗ 未挂载"; \
	fi
	@if grep -q CONFIG_BPF=y /boot/config-$$(uname -r) 2>/dev/null; then \
		echo "  内核 BPF 支持: ✓ 已启用"; \
	else \
		echo "  内核 BPF 支持: ? 无法确定"; \
	fi