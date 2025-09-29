.PHONY: help deps build run check clean docker-build docker-run docker-up docker-down docker-logs

# 默认目标
.DEFAULT_GOAL := help

# 程序名称和版本
PROGRAM := xtrace-catch
BPF_OBJ := xdp_monitor.o
IMAGE_NAME := xtrace-catch
IMAGE_TAG := latest

# 编译器设置
CLANG := clang
GO := go

# Docker 设置
INTERFACE ?= eth0

# 帮助信息
help: ## 显示帮助信息
	@echo "XTrace-Catch eBPF 网络流量监控器"
	@echo ""
	@echo "可用命令："
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# 安装依赖
deps: ## 安装编译依赖
	@echo "安装 eBPF 编译依赖..."
	@if command -v apt-get >/dev/null 2>&1; then \
		echo "检测到 Debian/Ubuntu 系统..."; \
		sudo apt-get update; \
		sudo apt-get install -y clang llvm libbpf-dev linux-headers-$$(uname -r) build-essential pkg-config libibverbs-dev; \
	else \
		echo "❌ 推荐使用 Docker: make docker-up"; \
		echo "手动安装: clang llvm libbpf-dev linux-headers-$$(uname -r) libibverbs-dev"; \
		exit 1; \
	fi
	@echo "检查 Go 版本..."
	@$(GO) version || (echo "请安装 Go 1.24+ 版本" && exit 1)


# 编译 eBPF 程序
$(BPF_OBJ): xdp_monitor.c
	@echo "编译 eBPF 程序..."
	@if ! command -v $(CLANG) >/dev/null 2>&1; then \
		echo "❌ clang 未安装，推荐使用 Docker: make docker-up"; \
		exit 1; \
	fi
	@if [ ! -f /usr/include/linux/bpf.h ]; then \
		echo "❌ 缺少内核头文件，推荐使用 Docker: make docker-up"; \
		exit 1; \
	fi
	$(CLANG) -O2 -target bpf -c xdp_monitor.c -o $(BPF_OBJ) || \
		(echo "❌ eBPF 编译失败！推荐解决方案:"; \
		 echo "   1. 使用 Docker（推荐）: make docker-up"; \
		 echo "   2. 手动安装依赖: make deps"; \
		 echo "   3. 检查系统信息: make info"; \
		 exit 1)

# 编译 Go 程序（包含所有监控模式）
$(PROGRAM): $(BPF_OBJ) main.go rdma_monitor.go nccl_monitor.go go.mod
	@echo "编译多模式监控程序..."
	$(GO) build -o $(PROGRAM) main.go rdma_monitor.go nccl_monitor.go

# 构建程序
build: $(PROGRAM) ## 编译多模式监控程序

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

# 显示网络接口
interfaces: ## 显示可用的网络接口
	@echo "可用的网络接口："
	@ip link show | grep -E "^[0-9]+:" | awk '{print "  " $$2}' | sed 's/://'

# 运行 RDMA 监控模式
run-rdma: build ## 运行 RDMA 监控模式
	@echo "启动 RDMA 监控模式..."
	./$(PROGRAM) -m rdma -d mlx5_0 -i ibs8f0

# 运行 NCCL 监控模式
run-nccl: build ## 运行 NCCL 监控模式
	@echo "启动 NCCL 监控模式..."
	./$(PROGRAM) -m nccl -d mlx5_0 -i ibs8f0

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

# ===========================================
# Docker 相关命令
# ===========================================

# 构建 Docker 镜像
docker-build: ## 构建 Docker 镜像
	@echo "构建 Docker 镜像..."
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
	@echo "✅ 镜像构建完成: $(IMAGE_NAME):$(IMAGE_TAG)"

# 直接运行 Docker 容器
docker-run: docker-build ## 使用 Docker 运行程序
	@echo "使用 Docker 运行 xtrace-catch (接口: $(INTERFACE))..."
	@echo "⚠️  需要特权模式和主机网络访问权限"
	docker run --rm --privileged --network host \
		-v /sys/fs/bpf:/sys/fs/bpf \
		-v /proc:/host/proc:ro \
		-v /sys:/host/sys:ro \
		-e NETWORK_INTERFACE=$(INTERFACE) \
		$(IMAGE_NAME):$(IMAGE_TAG) -i $(INTERFACE)

# 使用 docker-compose 启动
docker-up: ## 使用 docker-compose 启动服务
	@echo "使用 docker-compose 启动服务 (模式: $(MODE), 接口: $(INTERFACE), 设备: $(DEVICE))..."
	MODE=$(MODE) INTERFACE=$(INTERFACE) DEVICE=$(DEVICE) docker-compose up --build -d
	@echo "✅ 服务已启动，使用 'make docker-logs' 查看日志"

# 启动 XDP 监控模式
docker-up-xdp: ## 启动 XDP 监控模式
	@echo "启动 XDP 监控模式..."
	MODE=xdp INTERFACE=$(INTERFACE) DEVICE=$(DEVICE) docker-compose up --build -d
	@echo "✅ XDP 监控已启动"

# 启动 RDMA 监控模式
docker-up-rdma: ## 启动 RDMA 监控模式
	@echo "启动 RDMA 监控模式..."
	MODE=rdma INTERFACE=$(INTERFACE) DEVICE=$(DEVICE) docker-compose up --build -d
	@echo "✅ RDMA 监控已启动"

# 启动 NCCL 监控模式
docker-up-nccl: ## 启动 NCCL 监控模式
	@echo "启动 NCCL 监控模式..."
	MODE=nccl INTERFACE=$(INTERFACE) DEVICE=$(DEVICE) docker-compose up --build -d
	@echo "✅ NCCL 监控已启动"

# 停止 docker-compose 服务
docker-down: ## 停止 docker-compose 服务
	@echo "停止服务..."
	docker-compose down
	@echo "✅ 服务已停止"

# 进入运行中的容器
docker-shell: ## 进入运行中的容器 shell
	@echo "进入容器 shell..."
	@if [ "$$(docker ps -q -f name=xtrace-catch)" ]; then \
		docker exec -it xtrace-catch bash; \
	else \
		echo "❌ 容器未运行，请先执行 'make docker-up'"; \
		exit 1; \
	fi

# 清理 Docker 资源
docker-clean: ## 清理 Docker 镜像和容器
	@echo "清理 Docker 资源..."
	-docker-compose down --rmi all --volumes --remove-orphans
	-docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true
	@echo "✅ Docker 资源清理完成"