#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "${ROOT_DIR}"

go vet ./...
"${ROOT_DIR}/scripts/golangci-lint.sh" --version
"${ROOT_DIR}/scripts/golangci-lint.sh" run ./...

# Validate mapping structure and ensure mappings match offline products metadata.
go test ./internal/config >/dev/null
# 默认仅校验 aws（离线 products 快照与线上 mapping 的一致性可能因账号/区域差异导致 tencent/aliyun 误报）
# 如需全量校验可运行：MAPPINGS_CHECK_PROVIDERS=aliyun,tencent,aws make lint
go run ./cmd/mappings-check -providers "${MAPPINGS_CHECK_PROVIDERS:-aws}"
