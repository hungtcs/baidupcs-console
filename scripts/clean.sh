#!/usr/bin/env bash
# 清理构建产物。
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

log "删除 build/ 与 webapp/dist/"
rm -rf "${ROOT_DIR}/build" "${WEBAPP_DIR}/dist"

# dist 目录必须存在，否则 go:embed 会让 go build 直接失败。
ensure_dist_dir

log "已清理"
