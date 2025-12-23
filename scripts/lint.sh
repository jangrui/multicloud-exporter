#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "${ROOT_DIR}"

go vet ./...
"${ROOT_DIR}/scripts/golangci-lint.sh" --version
"${ROOT_DIR}/scripts/golangci-lint.sh" run ./...

# Validate mapping structure and ensure mappings match offline products metadata.
go test ./internal/config >/dev/null
# 校验所有支持的云厂商映射一致性 (aliyun, tencent, aws)
go run ./cmd/mappings-check -providers "${MAPPINGS_CHECK_PROVIDERS:-aliyun,tencent,aws}"
