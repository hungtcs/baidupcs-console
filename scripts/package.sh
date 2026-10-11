#!/usr/bin/env bash
# 交叉编译并打成 deb / rpm。
#
#   VERSION=0.1.0 ./scripts/package.sh
#   VERSION=0.1.0 ARCHES=amd64 ./scripts/package.sh      # 只出一个架构
#
# 前提：前端已经构建过（webapp/dist 里有 index.html），否则包里的前端是空的。
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

NFPM_VERSION="v2.47.0"

VERSION="${VERSION:-}"
ARCHES="${ARCHES:-amd64 arm64}"
DIST_DIR="${ROOT_DIR}/dist"

require go

[[ -n "${VERSION}" ]] || die "需要 VERSION，例如：VERSION=0.1.0 task package"

# 用 go install 装的 nfpm 通常在 GOPATH/bin，不一定在 PATH 里。
if ! command -v nfpm >/dev/null 2>&1; then
  export PATH="${PATH}:$(go env GOPATH)/bin"
fi
command -v nfpm >/dev/null 2>&1 \
  || die "找不到 nfpm，先装：go install github.com/goreleaser/nfpm/v2/cmd/nfpm@${NFPM_VERSION}"

if [[ ! -f "${WEBAPP_DIR}/dist/index.html" ]]; then
  warn "webapp/dist 里没有前端产物，打出来的包里前端会是空的。先跑 task web。"
fi

mkdir -p "${DIST_DIR}"

log "交叉编译（版本 ${VERSION}，架构 ${ARCHES}）"
for arch in ${ARCHES}; do
  out="${DIST_DIR}/baidupcs-console_linux_${arch}"
  # 前端已经 go:embed 进二进制，所以最终就一个可执行文件。
  # -trimpath 去掉构建机的路径，-s -w 去掉符号表和调试信息。
  CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o "${out}" .
  log "  ${out}"
done

log "打包 deb / rpm"
for arch in ${ARCHES}; do
  [[ -f "${DIST_DIR}/baidupcs-console_linux_${arch}" ]] || die "缺少 ${arch} 的二进制"

  cfg="$(mktemp)"
  # nfpm 会把 amd64 / arm64 自动映射成 rpm 用的 x86_64 / aarch64，
  # 所以一份架构名对两种包都成立，不用分别渲染。
  sed -e "s|\${ARCH}|${arch}|g" -e "s|\${VERSION}|${VERSION}|g" \
    "${ROOT_DIR}/deploy/nfpm.yaml" > "${cfg}"

  nfpm package -f "${cfg}" -p deb -t "${DIST_DIR}"
  nfpm package -f "${cfg}" -p rpm -t "${DIST_DIR}"
  rm -f "${cfg}"
done

echo
log "产物："
find "${DIST_DIR}" -maxdepth 1 -name '*.deb' -o -maxdepth 1 -name '*.rpm' | sort | sed 's|^|  |'
