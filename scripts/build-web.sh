#!/usr/bin/env bash
# 构建前端，产物落在 webapp/dist（会被 go:embed 打进二进制）。
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require pnpm

log "安装前端依赖"
cd "${WEBAPP_DIR}"
pnpm install --frozen-lockfile

log "构建前端"
pnpm build

ensure_dist_dir
log "前端产物: ${WEBAPP_DIR}/dist"
