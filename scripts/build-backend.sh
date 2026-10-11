#!/usr/bin/env bash
# 构建后端。会把 webapp/dist 里现有的前端产物一起嵌进去。
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require go

if [[ ! -f "${WEBAPP_DIR}/dist/index.html" ]]; then
  warn "webapp/dist 里没有前端产物，二进制里将只有占位内容。"
  warn "先跑 'task web' 或 'task build' 才能得到完整的前端。"
fi

ensure_dist_dir

log "编译后端"
cd "${ROOT_DIR}"
go build -o "${BINARY}" .

log "产出: ${BINARY}"
