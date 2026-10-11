#!/usr/bin/env bash
# 各脚本共用的环境准备。用 `source` 引入，不要直接执行。
set -euo pipefail

# 仓库根目录（本文件在 <root>/scripts/ 下）。
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export ROOT_DIR

BINARY="${ROOT_DIR}/build/bpcs-console"
export BINARY

WEBAPP_DIR="${ROOT_DIR}/webapp"
export WEBAPP_DIR

log()  { printf '\033[36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33m警告:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[31m错误:\033[0m %s\n' "$*" >&2; exit 1; }

require() {
  command -v "$1" >/dev/null 2>&1 || die "找不到命令 $1，请先安装"
}

# go:embed 要求 webapp/dist 必须存在，所以哪怕清空也要留个占位文件。
ensure_dist_dir() {
  mkdir -p "${WEBAPP_DIR}/dist"
  touch "${WEBAPP_DIR}/dist/.gitkeep"
}
