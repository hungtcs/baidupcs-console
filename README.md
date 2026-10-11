# BaiduPCS-Console

给 [BaiduPCS-Go](https://github.com/qjfoidnh/BaiduPCS-Go) 套一个网页控制台，用来在局域网里远程浏览网盘、发起下载、看实时进度。

BaiduPCS-Go 本身不提供网页接口，所以这里用一个独立的 Go 后端去 exec 这个 CLI。

## 设计取舍

终端输出是权威的：后端把 `BaiduPCS-Go` 当后台进程跑起来，把它的 stdout/stderr **原始字节**
通过 WebSocket 透传给前端的 [xterm.js](https://xtermjs.org/)。CLI 的进度条是用 `\r` 覆盖刷新的，
xterm 天然就能正确渲染，ANSI 颜色也一样。

在此之上做了两处**额外**解析，只为了让界面更好用，不取代终端：

- **`ls`** → 可点击的网盘目录浏览；
- **`download` 的进度行** → 终端上方的进度条（百分比、速度、剩余时间）。

两处解析都保留 CLI 的**原样文本**（如 `465.96MB`、`100.99KB/s`），前端直接显示，避免从字节数
反推格式带来的偏差。解析是旁路：正则匹配不上就什么都不做，绝不因为解析不出进度而丢掉终端字节。

## 依赖

- Go 1.25+
- Node 20+ / pnpm（只在构建前端时需要）
- 本机已安装并登录 `BaiduPCS-Go`（`BaiduPCS-Go login`）

## 构建与运行

用 [Task](https://taskfile.dev/) 作任务入口，具体逻辑在 `scripts/` 下。

```bash
task build     # 构建前端 + 后端 → build/bpcs-console
./build/bpcs-console
```

默认监听 `0.0.0.0:8080`，浏览器打开 `http://<本机IP>:8080`。

| 命令 | 说明 |
| --- | --- |
| `task build` | 构建前端 + 后端 |
| `task backend` | 只构建后端（复用 `webapp/dist` 里已有的前端产物） |
| `task web` | 只构建前端 |
| `task run` | 构建并运行 |
| `task dev` | 开发模式：后端 `:8080` + 前端 dev server `:5173`（`/api` 已代理，带热更新） |
| `task check` | 静态检查：`go vet` + `go test` + `tsc` + `oxlint` |
| `task test` | 只跑后端单元测试 |
| `task clean` | 清理构建产物 |

## 配置

全部走环境变量，可选地写进 `.env`（参考 `.env.example`）。优先级：进程环境变量 > `.env` > 内置默认值。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BPCS_LISTEN` | `0.0.0.0:8080` | HTTP 监听地址 |
| `BPCS_CLI_PATH` | `BaiduPCS-Go` | CLI 可执行文件路径 |
| `BPCS_SAVE_ROOT` | `~/Downloads` | 下载落盘根目录 |
| `BPCS_DATA_DIR` | `data` | 任务记录目录，一个任务一个 JSON |
| `BPCS_MODE` | `locate` | 下载模式：`locate` / `pcs` / `stream` |
| `BPCS_MAX_THREADS` | `0` | CLI 的 `-p` 参数，0 用 CLI 默认 |
| `BPCS_OUTPUT_BUFFER_BYTES` | `4194304` | 每个任务在内存里保留的输出上限 |

## ⚠️ 安全

**这个服务不鉴权，并且默认监听 `0.0.0.0`。** 它按局域网自用设计，任何能访问到这个端口的人
都可以浏览你的网盘内容并发起下载。**不要暴露到公网。** 需要收紧的话把 `BPCS_LISTEN` 改成
`127.0.0.1:8080`，再通过 SSH 隧道访问。

## 已知行为

- **CLI 出错时退出码也是 0**，所以不能靠退出码判断下载成败，要看终端输出。任务列表里的
  「已结束」只表示进程退出了，不代表下载成功。
- 目标文件已存在时 CLI 会跳过（打印 `文件已存在: ..., 跳过...`），所以重复下载会瞬间「结束」。
- 「重新开始」复用 CLI 的断点续传：已下载的分片会保留，从断点接着下。
- **任务列表会持久化**：每个任务一个 JSON 文件，存在 `<BPCS_DATA_DIR>/tasks/<任务ID>.json`，重启后自动恢复。
- 退出程序（Ctrl+C）时会先杀掉所有还在跑的下载进程，再关闭 HTTP 服务。被中断的任务重启后显示为「已停止」，点「重新开始」即可从断点续传。
- **终端输出不持久化**：输出只放在内存里（每任务上限 4MB）。重启后任务还在，但点开终端是空的，要看到新的输出得「重新开始」。
- 用 air 之类的热重载工具时注意：每次改 `.go` 文件都会重启服务，从而中断正在跑的下载。任务记录不会丢，但下载会停。

## 接口

见 [docs/API.md](docs/API.md)。

## 目录结构

```
main.go                  程序入口、优雅退出
assets.go                go:embed 前端产物（必须在根包，embed 不能引用上级目录）
internal/config/         dotenv + cleanenv 加载配置
internal/pcs/            调用 BaiduPCS-Go 并解析 ls/meta 的文本输出
internal/session/        后台进程、输出环形缓冲、订阅广播、任务持久化
internal/httpapi/        echo v5 路由、终端 WebSocket
webapp/                  Vite + React + TypeScript 前端
docs/API.md              接口契约
```
