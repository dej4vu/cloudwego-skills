#!/usr/bin/env bash
# 验证 fixture：tidy / build / vet / test / gofmt。只依赖 bash 与 Go 工具链。
#
#   scripts/verify.sh              全量（会 go mod tidy，可能改 go.mod / go.sum）
#   scripts/verify.sh --build-only 只编译，不改任何文件

source "$(dirname "$0")/lib.sh"
source "$(dirname "$0")/versions.env"

cd "$REPO_ROOT"

if [ "${1:-}" = "--build-only" ]; then
  for mod in "${FIXTURE_MODULES[@]}"; do
    begin "build $mod"
    if out="$(cd "$mod" && go build ./... 2>&1 && go vet ./... 2>&1)"; then
      pass
    else
      fail "$mod"
      printf '%s\n' "$out" | sed 's/^/    /'
    fi
  done
  summary
  exit
fi

for mod in "${FIXTURE_MODULES[@]}"; do
  verify_module "$mod"
done

summary
