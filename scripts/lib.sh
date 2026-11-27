#!/usr/bin/env bash
# 公共函数。所有脚本以 `source "$(dirname "$0")/lib.sh"` 引入。

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# 逐步汇总结果：每个检查调用 pass/fail，结束时 summary 决定退出码。
FAILURES=0
CURRENT_STEP=""

begin() {
  CURRENT_STEP="$1"
  printf '==> %s ... ' "$1"
}

pass() {
  printf 'ok\n'
}

fail() {
  FAILURES=$((FAILURES + 1))
  printf 'FAIL\n'
  if [ "$#" -gt 0 ]; then
    printf '    %s\n' "$1"
  fi
}

summary() {
  if [ "$FAILURES" -ne 0 ]; then
    printf '\n%d check(s) failed\n' "$FAILURES"
    exit 1
  fi
  printf '\nall checks passed\n'
}

# 在模块目录里跑 tidy / build / vet / test / gofmt，全部通过才算 pass。
verify_module() {
  local dir="$1"
  local out
  if ! out="$(cd "$dir" && go mod tidy 2>&1 && go build ./... 2>&1 && go vet ./... 2>&1 && go test ./... -count=1 2>&1)"; then
    fail "$dir"
    printf '%s\n' "$out" | sed 's/^/    /'
    return
  fi
  local dirty
  dirty="$(cd "$dir" && gofmt -l . | grep -v '^$' || true)"
  if [ -n "$dirty" ]; then
    fail "$dir (gofmt)"
    printf '%s\n' "$dirty" | sed 's/^/    not gofmt-clean: /'
    return
  fi
  pass
}
