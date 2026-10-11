# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 工作方式约定

- **不要自己启动后端。** 后端由用户用 `air` 热重载启动（配置见 `.air.toml`，产物 `./tmp/main`）。不要跑 `go run .`、不要跑编译出的二进制、不要用 `task dev` / `task run`。验证后端改动只用 `go test` 和 `go vet`。
- **不要自己开浏览器或 Playwright 验证前端。** 只做静态检查（`pnpm build`、`tsc -b`、`oxlint`）。界面交互、WebSocket、xterm 渲染都由用户自测并反馈。
- **技术选型先问用户，不要自行拍板。** 引入新依赖、更换库或框架、调整方案，都要先把备选列出来让用户选。已有选型（echo v5、cleanenv、xterm）是用户指定的，不要替换。

## 常用命令

```bash
task build      # 前端 + 后端 → build/bpcs-console（单个二进制）
task backend    # 只构建后端，复用 webapp/dist 里已有的前端产物
task web        # 只构建前端
task check      # go vet + go test + gofmt + tsc -b + oxlint
task test       # 只跑后端单元测试
task clean      # 清理构建产物（会重建 webapp/dist/.gitkeep）
```

跑单个测试：

```bash
go test ./internal/pcs/ -run TestParseLsRoot -v
go test ./internal/session/ -run TestRingBuffer -v
go test ./internal/session/ -run TestManagerRestoresTasks -v   # 持久化/恢复
go test -race ./...          # 涉及并发改动时跑这个
```

前端目录（`webapp/`）里：`pnpm build`、`pnpm lint`、`pnpm exec tsc -b`。

## 核心设计

**后端不解析任何 CLI 输出。** 它只做三件事：把 `BaiduPCS-Go` 当后台进程跑起来、把进程的 stdout/stderr **原始字节**通过 WebSocket 透传、提供列目录和任务增删这几个接口。

进度条、`\r` 覆盖刷新、ANSI 颜色全部交给前端的 xterm.js。因此**任务对象里没有 `progress` / `speed` / `eta` 字段**——进度就在终端文本里。任何"帮前端把进度解析出来"的想法都违背这个设计。

唯一的例外是 `ls`：文件列表需要结构化数据（可点击的目录导航），所以 `internal/pcs` 会解析它。为了不让解析结果与 CLI 显示产生偏差，`Entry` 同时返回结构化值和**原样文本**（`sizeText`、`modTimeText`），前端显示用原样文本，且**不重排顺序**（`entries` 的顺序就是 `ls` 的输出顺序）。

## 架构

```
main.go          启动、优雅退出（先 KillAll 再 Shutdown）
assets.go        go:embed all:webapp/dist
internal/config  cleanenv 读 .env + BPCS_* 环境变量
internal/pcs     exec CLI + 解析 ls/meta 文本
internal/session 后台进程、输出环形缓冲、订阅广播、任务持久化
internal/httpapi echo v5 路由 + 终端 WebSocket
webapp/          Vite + React 19 + TS 前端
data/            运行期数据目录（默认，gitignore），一个任务一个 JSON
docs/API.md      接口契约（改接口必须同步改这里）
```

数据流：`Browser.tsx` → `GET /api/browse` → `pcs.Ls()` → `exec BaiduPCS-Go ls` → 解析 → JSON。
下载：`POST /api/tasks` → `session.Manager.StartDownload` → `exec BaiduPCS-Go download` → 输出进环形缓冲 → `GET /api/tasks/{id}/terminal` 的 WebSocket 广播给 xterm。

### `assets.go` 为什么在根包

`go:embed` 只能引用当前包目录或其子目录，所以内嵌声明不能放进 `internal/httpapi`。用 `all:` 前缀是为了把 `.gitkeep` 也嵌进来——否则前端还没构建过时 `go build` 会因为目录为空而失败。

### `internal/session` 的并发约定

- 每个 `Session` 有一把锁保护 `task` 状态、环形缓冲和订阅者表。
- `Subscribe()` 在**同一把锁下**完成"注册订阅者 + 快照已有输出"，所以重放不会重复也不会漏。
- 进程结束时 `wait()` 在锁内关闭并清空所有订阅者通道。`unsubscribe` 和慢订阅者踢除都先检查通道是否还在表里，避免重复 close 导致 panic。
- 写 `s.task` 的字段一律在锁内（`PID` 也是），因为 `Task()` / `List()` 会并发读。
- 慢订阅者（通道满）会被直接断开，前端重连后会拿到完整重放。

### 任务落盘与生命周期

- 所有任务都 `--saveto <BPCS_SAVE_ROOT>`（默认 `~/Downloads`），**平铺**，不建每任务子目录。不同网盘目录下的同名文件会冲突，CLI 会跳过后者。
- **暂停 = kill 进程，继续 = 重跑同一条命令**（`/restart`）。BaiduPCS-Go 支持断点续传，已下载的分片和 `.BaiduPCS-Go-downloading` 元数据会保留。
- 进程用独立进程组启动（`Setpgid` + `Pdeathsig`），kill 时对 `-pid` 发信号，连带子进程一起收掉。
- **终端输出不持久化**，只在内存环形缓冲里（每任务 `BPCS_OUTPUT_BUFFER_BYTES`，默认 4MB）。重启后任务还在但终端是空的。

### 任务持久化（`internal/session/store.go`）

一个任务一个 JSON 文件：`<BPCS_DATA_DIR>/tasks/<任务ID>.json`（默认 `data/tasks/`）。写盘用「先写 `.tmp` 再 rename」，读的时候跳过 `.tmp` 和损坏的文件（坏一个不影响其余）。

启动时 `Manager.restore()` 把任务读回内存，规则是：

- 上次是 `running` 的一律改成 **`killed`**——服务退出时进程确实被杀了（`KillAll` 或 `Pdeathsig`），状态没失真。因此**没有也不需要有 `interrupted` 状态**，前端对 `killed` 已经会显示「重新开始」。
- `FinishedAt` 为空的补上当前时间，否则前端会把它当成还在跑。

**落盘时机与 `done` 通道的先后顺序很关键**：`Session.wait()` 里必须**先调用 `onExit`（落盘）、再 `close(s.done)`**。`Kill` / `Remove` / `Restart` 都靠 `waitDone(sess)` 来判断「这个进程的收尾工作已经做完了」。如果提前关闭 `done`：

- `Remove` 会先删掉 JSON，然后被旧进程的收尾落盘又写回来；
- `Restart` 会被旧进程的收尾覆盖掉新任务的状态。

`Manager.persist` 就是那个 `onExit` 回调，写失败只记日志，不影响下载本身。

## BaiduPCS-Go 的行为（解析逻辑都建立在这些实测结论上）

这些是本项目最容易踩的坑，改 `internal/pcs` 前务必先看：

- **退出码恒为 0，出错也是 0。** 判断成败只能扫描输出文本里的 `遇到错误`。不要写 `if exitCode != 0 { 失败 }`。
- **所有输出走 stdout，stderr 始终为空。**
- 非 TTY 下不输出 ANSI 颜色，但进度条用 `\r` 覆盖刷新。抓日志时要按 `\r` 和 `\n` 一起切行并丢掉进度行，否则缓冲区会被刷爆。
- `ls` 的列按**显示宽度**对齐（中文算两个宽度），所以不能按固定列位置切分，只能用「日期时间」做正则锚点。目录名以 `/` 结尾，目录的大小列是 `-`。
- 文件大小是 **1024 进制**：`145961465` 字节显示为 `139.199700MB`。显示文本是四舍五入过的，`parseSize` 因此用 `math.Round` 而非截断。
- 目标文件已存在时 CLI 会跳过（打印 `文件已存在: ..., 跳过...`），所以重复下载会瞬间「结束」。
- 对**文件**路径调 `ls` 返回空输出（不是错误）。
- 新建任务时会先调一次 `meta` 校验路径存在，把 CLI「路径不存在就静默什么都不做」变成明确报错。

## 测试约定

- `internal/pcs/pcs_test.go` 里的 CLI 输出样本是从真实 v4.0.2 上原样抄下来的（含中文对齐的尾随空格）。改解析逻辑时这些用例必须继续通过，不要为了迁就新代码去改样本。
- 解析逻辑都拆成了纯函数（`parseLs`、`parseMeta`、`parseSize`、`extractError`），就是为了能脱离真实 CLI 做单元测试。新增解析逻辑请沿用这个模式。
- `internal/session` 的测试用 `/bin/sh` 当被测进程，不依赖真实的 BaiduPCS-Go。
- `internal/httpapi` 的测试用 `httptest` + `fstest.MapFS`，不起真实服务、不碰真实网盘。

## 前端约定

- **除 `react` / `react-dom` / `@xterm/xterm` / `@xterm/addon-fit` 外不要新增 npm 依赖**（无 UI 框架、无 Tailwind、无 axios、无路由库）。样式用普通 CSS + CSS 变量，深色主题。
- 终端 WebSocket 收到的是**二进制帧**，必须用流式 `TextDecoder` 解码后再喂给 xterm；且**每条连接都要新建 decoder**——重连时服务端会从头重放，沿用旧 decoder 残留的半截多字节 UTF-8 会把重放开头解坏。
- 服务端在进程结束时以 **code 1000** 主动关闭连接，那是正常结束，**不要重连**；只有异常断开才退避重连。
- 任务列表靠 `GET /api/tasks` **每 2 秒轮询**（后端没有列表推送）。用签名比对跳过内容未变的 setState，避免整列表闪动。
- React Compiler 已启用；StrictMode 会双调用 effect，创建 xterm / WebSocket 的地方要保证 cleanup 干净。

## 安全

服务**不鉴权**，默认监听 `0.0.0.0`，按局域网自用设计。`upgrader.CheckOrigin` 恒返回 true，任何站点都能连上终端。不要把它暴露到公网，也不要在没有明确要求的情况下"顺手加上"鉴权——需要时先问用户。
