# ==============================================================================
# 变量定义
# ==============================================================================
SHELL := /bin/bash
APP_NAME := multicloud-exporter
MODULE_NAME := multicloud-exporter

# 版本信息
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# 构建参数
LDFLAGS := -w -s \
	-X '$(MODULE_NAME)/internal/version.Version=$(VERSION)' \
	-X '$(MODULE_NAME)/internal/version.CommitSHA=$(COMMIT_SHA)' \
	-X '$(MODULE_NAME)/internal/version.BuildTime=$(BUILD_TIME)'

# 目录定义
BIN_DIR := bin
DIST_DIR := dist

# 工具定义
GOPATH_BIN := $(shell go env GOPATH)/bin
# 优先使用系统 PATH 中的 golangci-lint
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null)

# 如果系统没有，则指向 GOPATH/bin 下的
ifeq ($(GOLANGCI_LINT),)
	GOLANGCI_LINT := $(GOPATH_BIN)/golangci-lint
endif

GOIMPORTS := $(GOPATH_BIN)/goimports

# ==============================================================================
# 通用命令
# ==============================================================================
.PHONY: help
help: ## 显示帮助信息
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: all
all: lint test build ## 执行完整流程：Lint -> Test -> Build

# ==============================================================================
# 开发与构建
# ==============================================================================
.PHONY: build
build: ## 构建二进制文件 (输出到 bin/ 目录)
	@echo "Building $(APP_NAME) $(VERSION)..."
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) ./cmd/multicloud-exporter

.PHONY: build-linux
build-linux: ## 交叉编译 Linux 版本
	@echo "Building for Linux..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME)-linux-amd64 ./cmd/multicloud-exporter

.PHONY: run
run: ## 本地运行 (使用 .local/scripts/run.sh)
	@bash .local/scripts/run.sh

.PHONY: clean
clean: ## 清理构建产物
	@echo "Cleaning..."
	@rm -rf $(BIN_DIR) $(DIST_DIR)
	@go clean -testcache

# ==============================================================================
# 测试与质量保证
# ==============================================================================
.PHONY: test
test: ## 运行单元测试
	@echo "Running tests..."
	go test -v -race -cover ./...

.PHONY: lint
lint: tools ## 代码静态检查
	@echo "Running golangci-lint..."
	$(GOLANGCI_LINT) run ./...
	@echo "Validating mapping structure..."
	@go test ./internal/config >/dev/null

.PHONY: fmt
fmt: tools ## 格式化代码 (goimports)
	@echo "Formatting code..."
	$(GOIMPORTS) -w -local $(MODULE_NAME) .
	go mod tidy

.PHONY: check
check: fmt lint test build ## 提交前检查 (Format + Lint + Test + Build)
	@echo "All checks passed!"

.PHONY: mappings-check
mappings-check: ## 检查指标映射配置
	go run ./cmd/mappings-check

# ==============================================================================
# Docker 支持
# ==============================================================================
# IMAGE_REPO ?= multicloud-exporter
# IMAGE_TAG ?= $(VERSION)

# .PHONY: docker-build
# docker-build: ## 构建 Docker 镜像
# 	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) \
# 		--build-arg VERSION=$(VERSION) \
# 		--build-arg COMMIT_SHA=$(COMMIT_SHA) \
# 		--build-arg BUILD_TIME=$(BUILD_TIME) \
# 		.

# .PHONY: docker-push
# docker-push: ## 推送 Docker 镜像
# 	docker push $(IMAGE_REPO):$(IMAGE_TAG)

# ==============================================================================
# Helm 支持
# ==============================================================================
.PHONY: helm-lint
helm-lint: ## 检查 Helm Chart 语法
	helm lint chart/

.PHONY: helm-template
helm-template: ## 渲染 Helm 模板 (Dry Run)
	helm template $(APP_NAME) chart/ --debug

# ==============================================================================
# 工具链管理
# ==============================================================================
.PHONY: tools
tools: ## 安装开发工具依赖
	@if [ ! -x "$(GOLANGCI_LINT)" ]; then \
		echo "golangci-lint not found. Installing..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	@if [ ! -f $(GOIMPORTS) ]; then \
		echo "Installing goimports..."; \
		go install golang.org/x/tools/cmd/goimports@latest; \
	fi
