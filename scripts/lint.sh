#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "${ROOT_DIR}"

go vet ./...
"${ROOT_DIR}/scripts/golangci-lint.sh" --version
"${ROOT_DIR}/scripts/golangci-lint.sh" run ./...


