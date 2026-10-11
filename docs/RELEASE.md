# 打包与发版

## 发一个版本

```bash
git tag v0.1.0 && git push origin v0.1.0
```

推 `v*` tag 会触发 `.github/workflows/release.yml`：构建前端 → 跑单元测试 → 交叉编译
amd64/arm64 → 用 [nfpm](https://nfpm.goreleaser.com/) 打出 deb 和 rpm → 创建 GitHub Release
并把包挂上去。产物形如：

```
baidupcs-console_0.1.0_amd64.deb
baidupcs-console_0.1.0_arm64.deb
baidupcs-console-0.1.0-1.x86_64.rpm
baidupcs-console-0.1.0-1.aarch64.rpm
```

**预发布版本**（tag 里带 `-`，如 `v0.1.0-test.1`）会被自动标成 Pre-release，
不会占着仓库的 Latest release 位置。

## 先手动验证一遍

不想直接推 tag 的话，去 Actions 页面手动触发 `release`（`workflow_dispatch`）——
它跑完构建和打包，产物留在该次运行的 artifacts 里，**不会创建 Release**。
第一次改 workflow 之后建议这么验一遍。

## 本地打包

```bash
VERSION=0.1.0 task package                 # amd64 + arm64
VERSION=0.1.0 ARCHES=amd64 task package    # 只出 amd64
```

需要 nfpm：`go install github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.47.0`。产物在 `dist/`。

## 版本号

构建时通过 `-ldflags -X main.version=...` 注入（见 `scripts/package.sh`）。直接 `go build` 则是 `dev`。

```bash
baidupcs-console --version     # 打印版本后退出
```

运行中的实例会把版本显示在概览页的「运行环境」里，也写进 `GET /api/health` 的 `version`。

## 包里装了什么

| 路径 | 说明 |
| --- | --- |
| `/usr/bin/baidupcs-console` | 二进制。前端已经 `go:embed` 进去，就这一个文件 |
| `/usr/lib/systemd/user/baidupcs-console.service` | systemd **用户** unit |
| `/usr/share/doc/baidupcs-console/` | README、接口文档、LICENSE |

安装后会打印一段提示，告诉用户怎么启用服务、怎么放行防火墙端口。

## 几个容易踩的点

**别被 Release 附件的文件名骗了。** nfpm 产出的包名是 `0.1.0~test.1`（`~` 让预发布排在正式版
**之前**，这是对的），但 GitHub 上传附件时会把 `~` 换成 `.`，附件名变成 `0.1.0.test.1`。
包**内部**的版本号不受影响（`dpkg-deb -f x.deb Version` 查得到），升级顺序是对的，别去"修"它。

**nfpm 会自动映射架构名。** 配置里写 `amd64`/`arm64`，它会转成 rpm 用的 `x86_64`/`aarch64`，
所以一份架构名对两种包都成立，不用分别渲染配置。

**先构建前端再交叉编译。** 后端是 `go:embed` 把前端嵌进去的，顺序反了打出来的包前端会是空的。

**预发布必须显式标记。** GitHub 不会只看 tag 名字自动判断 prerelease，
workflow 里按「版本号里有没有 `-`」来决定要不要加 `--prerelease`。

**两份 unit 别搞混。** `deploy/baidupcs-console.service` 是打进包的（`ExecStart=/usr/bin/...`），
`deploy/baidupcs-console.service.in` 是本地 `task deploy` 用的模板（装到 `~/.local/bin/`）。
改一处记得看另一处。
