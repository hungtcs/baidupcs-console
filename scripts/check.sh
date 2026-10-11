#!/usr/bin/env bash
# 静态检查：后端 go vet + go test，前端 tsc 类型检查 + oxlint。
# 不做任何浏览器验证。
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require go
require pnpm

log "后端: go vet"
cd "${ROOT_DIR}"
go vet ./...

log "后端: go test"
go test ./...

log "后端: gofmt 检查"
unformatted="$(gofmt -l . | grep -v '^webapp/' || true)"
if [[ -n "${unformatted}" ]]; then
  die "以下文件未格式化，请执行 gofmt -w：
${unformatted}"
fi

log "前端: TypeScript 类型检查"
cd "${WEBAPP_DIR}"
pnpm exec tsc -b

log "前端: oxlint"
pnpm lint

log "全部检查通过"
