.PHONY: lint

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

.PHONY: mappings-check

mappings-check:
	go run ./cmd/mappings-check

.PHONY: check
check: lint
	go test -v -race -cover ./...
