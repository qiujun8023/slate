#!/usr/bin/env bash
# 提交前检查，CI 跑的也是这个脚本。用法：scripts/check.sh [backend|frontend]，不带参数两端都跑。
# CI=true 时额外要求 go generate 不产生改动（生成物与源码一致）。
set -euo pipefail
cd "$(dirname "$0")/.."

backend() {
  (
    cd backend
    unformatted="$(gofmt -l .)"
    if [ -n "$unformatted" ]; then
      echo "gofmt needed:" >&2
      echo "$unformatted" >&2
      exit 1
    fi
    go vet ./...
    go generate ./...
    go test ./...
  )
  if [ "${CI:-}" = "true" ] && [ -n "$(git status --porcelain -- backend frontend/src/api)" ]; then
    echo "go generate changed files; run it locally and commit the result:" >&2
    git status --porcelain -- backend frontend/src/api >&2
    exit 1
  fi
}

frontend() {
  (
    cd frontend
    bun run format:check
    bun run lint
    bun run test
    bun run build
  )
}

case "${1:-all}" in
  backend) backend ;;
  frontend) frontend ;;
  all) backend && frontend ;;
  *)
    echo "usage: $0 [backend|frontend]" >&2
    exit 2
    ;;
esac
