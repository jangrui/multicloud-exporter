#!/usr/bin/env bash
set -euo pipefail

# 为了保证本地与 CI 行为一致，这里固定 golangci-lint 版本（与 .github/workflows/ci.yml 对齐）。
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION:-v2.7.2}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${ROOT_DIR}/bin"
GOLANGCI_LINT_BIN="${BIN_DIR}/golangci-lint"

mkdir -p "${BIN_DIR}"

if [[ ! -x "${GOLANGCI_LINT_BIN}" ]]; then
  echo "Installing golangci-lint ${GOLANGCI_LINT_VERSION} -> ${GOLANGCI_LINT_BIN}" >&2
  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
    | sh -s -- -b "${BIN_DIR}" "${GOLANGCI_LINT_VERSION}"
fi

exec "${GOLANGCI_LINT_BIN}" "$@"


