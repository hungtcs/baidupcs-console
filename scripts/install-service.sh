#!/usr/bin/env bash
# 把后端装成 systemd 用户服务：开机自启、崩了自动拉起、日志进 journal。
#
# 前提是已经 `task build` 过（或者由 `task deploy` 调起来）。
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

require systemctl

UNIT_NAME="baidupcs-console.service"
BIN_DIR="${HOME}/.local/bin"
UNIT_DIR="${HOME}/.config/systemd/user"
DATA_DIR="${HOME}/.local/share/baidupcs-console"
INSTALLED="${BIN_DIR}/baidupcs-console"
LISTEN="${BPCS_LISTEN:-0.0.0.0:8080}"

[[ -f "${BINARY}" ]] || die "找不到 ${BINARY}，先跑一次 task build"

# 探测 CLI 路径并写进 unit，免得依赖用户管理器的 PATH。
CLI_PATH="$(command -v BaiduPCS-Go || true)"
if [[ -z "${CLI_PATH}" ]]; then
  warn "PATH 里找不到 BaiduPCS-Go，unit 里会写成 BaiduPCS-Go（靠 systemd 的 PATH 去找）"
  CLI_PATH="BaiduPCS-Go"
else
  log "BaiduPCS-Go: ${CLI_PATH}"
fi

log "安装二进制到 ${INSTALLED}"
install -Dm755 "${BINARY}" "${INSTALLED}"

log "创建数据目录 ${DATA_DIR}"
mkdir -p "${DATA_DIR}"

log "生成 unit 到 ${UNIT_DIR}/${UNIT_NAME}"
mkdir -p "${UNIT_DIR}"
sed -e "s|@CLI_PATH@|${CLI_PATH}|g" \
    -e "s|@LISTEN@|${LISTEN}|g" \
    "${ROOT_DIR}/deploy/${UNIT_NAME}.in" > "${UNIT_DIR}/${UNIT_NAME}"

# 先清掉上一次失败留下的计数，否则「起不来 → 处理 → 再启」会被 StartLimit 拦住。
systemctl --user reset-failed "${UNIT_NAME}" 2>/dev/null || true
systemctl --user daemon-reload

log "开启开机自启并启动"
systemctl --user enable "${UNIT_NAME}" >/dev/null
systemctl --user restart "${UNIT_NAME}"

# 用户服务默认只在该用户有会话时运行；开了 linger 就变成不登录也常驻。
if ! loginctl show-user "${USER}" --property=Linger 2>/dev/null | grep -q 'Linger=yes'; then
  log "开启 linger（不登录也开机自启）"
  loginctl enable-linger "${USER}"
fi

sleep 1
echo
systemctl --user --no-pager status "${UNIT_NAME}" || true
echo
log "监听 ${LISTEN}；日志：journalctl --user -u ${UNIT_NAME} -f"
