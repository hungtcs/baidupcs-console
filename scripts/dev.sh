#!/usr/bin/env bash
# 开发模式：后端跑 :8080，前端 dev server 跑 :5173 并把 /api 代理到后端。
# 改前端有热更新；改后端需要重启本脚本。
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require go
require pnpm

# 退出时把两个子进程一起收掉。
pids=()
cleanup() {
  trap - INT TERM EXIT
  for pid in "${pids[@]:-}"; do
    kill "${pid}" 2>/dev/null || true
  done
  wait 2>/dev/null || true
}
trap cleanup INT TERM EXIT

ensure_dist_dir

log "启动后端 → http://localhost:8080"
cd "${ROOT_DIR}"
go run . &
pids+=($!)

log "启动前端 dev server → http://localhost:5173"
cd "${WEBAPP_DIR}"
pnpm dev &
pids+=($!)

log "浏览器请打开 http://localhost:5173（Ctrl+C 退出）"
wait
