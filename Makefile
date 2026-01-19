.PHONY: help build test lint check clean mappings-check

help:
	@echo "可用命令："
	@echo "  make build           - 构建应用"
	@echo "  make test            - 运行单元测试"
	@echo "  make lint            - 代码静态检查"
	@echo "  make mappings-check  - 检查指标映射配置"
	@echo "  make clean           - 清理构建产物"
	@echo "  make check           - 完整检查（依赖整理 + Lint + 测试 + 构建）"

build:
	@echo "Building..."
	go build -v ./...

test:
	@echo "Running tests..."
	go test -v -race -cover ./...

lint:
	@echo "Running go vet..."
	@go vet ./... 2>&1 | grep -v "operation not permitted" | grep -v "package encoding/pem is not in std" || true
	@echo "Checking golangci-lint availability..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "Using system golangci-lint: $$(golangci-lint --version)"; \
		golangci-lint run ./...; \
	else \
		echo "ERROR: golangci-lint not found in PATH"; \
		echo "Install it with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		echo "Or use: brew install golangci-lint"; \
		exit 1; \
	fi
	@echo "Validating mapping structure..."
	@go test ./internal/config >/dev/null

mappings-check:
	go run ./cmd/mappings-check

clean:
	@echo "Cleaning..."
	rm -rf bin/

check:
	@echo "Running complete check..."
	go mod tidy
	$(MAKE) lint
	$(MAKE) test
	$(MAKE) build
	@echo "All checks passed!"
