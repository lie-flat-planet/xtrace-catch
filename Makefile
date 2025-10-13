.PHONY: help build clean docker-build docker-clean version

# 默认目标
.DEFAULT_GOAL := help

# 程序名称和版本
PROGRAM := xtrace-catch
BPF_OBJ := xdp_monitor.o
IMAGE_NAME := xtrace-catch
VERSION := $(shell cat .version 2>/dev/null || echo "dev")

# 编译器设置
CLANG := clang
GO := go

# 帮助信息
help: ## 显示帮助信息
	@echo "XTrace-Catch 网络流量监控器"
	@echo ""
	@echo "常用命令："
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# 编译 eBPF 程序
$(BPF_OBJ): xdp_monitor.c
	@echo "编译 eBPF 程序..."
	$(CLANG) -O2 -target bpf -c xdp_monitor.c -o $(BPF_OBJ)

# 编译 Go 程序
$(PROGRAM): $(BPF_OBJ) main.go rdma_monitor.go nccl_monitor.go
	@echo "编译 Go 程序..."
	$(GO) build -o $(PROGRAM) main.go rdma_monitor.go nccl_monitor.go

# 构建程序
build: $(PROGRAM) ## 编译程序

# 清理编译文件
clean: ## 清理编译文件
	@echo "清理编译文件..."
	rm -f $(PROGRAM) $(BPF_OBJ)

# 构建 Docker 镜像
docker-build: ## 构建 Docker 镜像
	@echo "构建 Docker 镜像 (版本: $(VERSION))..."
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE_NAME):$(VERSION) .
	@echo "✅ 镜像构建完成: $(IMAGE_NAME):$(VERSION)"

# 清理 Docker 资源
docker-clean: ## 清理 Docker 资源
	@echo "清理 Docker 资源..."
	-docker rmi $(IMAGE_NAME):$(VERSION) 2>/dev/null || true
	@echo "✅ 清理完成"

# 显示版本
version: ## 显示当前版本
	@echo "$(VERSION)"

# 设置版本
set-version: ## 设置版本号 (VERSION=x.x.x)
	@if [ -z "$(NEW_VERSION)" ]; then \
		echo "用法: make set-version NEW_VERSION=x.x.x"; \
		exit 1; \
	fi
	@echo "$(NEW_VERSION)" > .version
	@echo "✅ 版本已更新为: $(NEW_VERSION)"
