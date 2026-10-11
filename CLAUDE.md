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

**终端输出是权威的，结构化数据只是附赠的便利。** 后端把 `BaiduPCS-Go` 当后台进程跑起来，把进程的 stdout/stderr **原始字节**通过 WebSocket 透传；进度条、`\r` 覆盖刷新、ANSI 颜色全部交给前端的 xterm.js。

在此之上有两处**额外**的解析，都是为了让界面更好用，而不是取代终端：

- **`ls`** → `pcs.Entry`（可点击的目录导航）。为避免解析结果与 CLI 显示产生偏差，`Entry` 同时返回结构化值和**原样文本**（`sizeText`、`modTimeText`），前端显示用原样文本，且**不重排顺序**（`entries` 的顺序就是 `ls` 的输出顺序）。
- **`download` 的进度行** → `pcs.Progress`（终端上方的进度条）。同样带上 `doneText` / `totalText` / `speedText` / `etaText` 这些原样文本。

两条原则：

1. **解析失败不能影响终端。** 终端字节照常透传，解析只是旁路。正则匹配不上就什么都不做，绝不因为解析不出进度就丢字节或报错。
2. **要与 CLI 显示一致时，透传原始文本而不是反推。** CLI 的大小列有自己的精度（`465.96MB`）和四舍五入（`139.199700MB`），从字节数格式化回去不保证还原。

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

### 进度解析（`internal/pcs/progress.go`）

进度行原样长这样（实测 v4.0.2，**圆点固定 12 个，不随终端宽度变化**）：

```
[1] ↓ 128.00KB/139.20MB 213.21KB/s in 1s, left 11m7s ............
[1] ↓ 1.88MB/139.20MB 0B/s in 55s, left - ............
```

两个容易踩的点：

- **输出是按块到达的，一块可能在半行处断开**，`ProgressParser` 因此会缓存未完成的那一段。逐字节喂入与一次性喂入的结果必须完全一致，有专门用例守这一点。
- **最新那条进度行往往还没以 `\r` 收尾**（进度行是先写内容、下一条行首才补 `\r`）。只处理已终结的行会永远慢一拍，所以 `Snapshot()` 会额外对尾部缓冲试一次——但**只按进度行正则**，因为那个正则要求尾部完整（`in 1s, left 11m7s`），半截的行匹配不上；而「准备下载: <路径>」被截断会得到错误的文件名。

**目录下载**：CLI 逐文件打印，索引 `[N]` 递增，且事先不告诉目录总大小。所以 `Progress` 以**当前文件**为准，另外给出 `filesDone` / `filesTotal`（单文件任务下 `filesTotal` 为 1）。不要试图把多个文件的字节数聚合成一个总百分比——分母是逐渐长出来的，百分比会来回跳。

进度通过**终端 WebSocket 的文本帧**推给前端（二进制帧仍是终端字节），`writeTerminal` 里每秒读一次任务快照、有变化才发。改这个协议要同步改 `useTerminal.ts` 的 `handleControlMessage`。

### 任务落盘与生命周期

- 所有任务都 `--saveto <BPCS_SAVE_ROOT>`（默认 `~/Downloads`），**平铺**，不建每任务子目录。不同网盘目录下的同名文件会冲突，CLI 会跳过后者。
- **暂停 = kill 进程，继续 = 重跑同一条命令**（`/restart`）。BaiduPCS-Go 支持断点续传，已下载的分片和 `.BaiduPCS-Go-downloading` 元数据会保留。
- `Task.Resumable` 决定界面按钮显示「继续」还是「重新开始」，由 `Manager.IsResumable` 读文件系统判定：CLI 给没下完的文件留 `<文件名>.BaiduPCS-Go-downloading` 标记，**下完会删掉**，所以「有标记 = 可续传」。这个判断**只在进程结束后和启动恢复时各算一次**（`onProcessExit` / `restore`），不能放进 `Task()`/`List()`——列表每 2 秒轮询一次，每次去走目录会拖垮它。
- 进程用独立进程组启动（`Setpgid` + `Pdeathsig`），kill 时对 `-pid` 发信号，连带子进程一起收掉。
- **终端输出不持久化**，只在内存环形缓冲里（每任务 `BPCS_OUTPUT_BUFFER_BYTES`，默认 4MB）。
  重启后任务还在，但终端是空的，会显示 `.terminal-empty` 那块空状态。

  **这是刻意的，不要「顺手」补上。** 2026-10-11 已跟用户确认过：加日志文件（追加写
  `data/logs/<id>.log`、重启时读回环形缓冲、重跑时截断）这个方案讨论过，用户明确表示
  「先这样，目前够用了」所以暂缓。要做的话务必先确认，并定下「重跑时旧日志截断还是保留」。

  另外两点也评估过，结论是不做：**速度曲线历史**（重启后从零开始，曲线本来就是看当下的）、
  以及**持久化的 `progress` 只在进程结束时写一次**（SIGKILL 时会停在创建时的值；不值得为它
  每秒重写 JSON，而且「下到哪了」本来就能从磁盘文件大小读出来）。

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

- `internal/pcs/pcs_test.go` 里的 CLI 输出样本是从真实 v4.0.2 上原样抄下来的（含中文对齐的尾随空格）；`internal/pcs/testdata/*.raw` 更是直接 `> file 2>&1` 录下来的原始字节（含 `\r`）。改解析逻辑时这些用例必须继续通过，不要为了迁就新代码去改样本。
- 解析逻辑都拆成了纯函数（`parseLs`、`parseMeta`、`parseSize`、`extractError`）或纯类型（`ProgressParser`），就是为了能脱离真实 CLI 做单元测试。新增解析逻辑请沿用这个模式。
- 要新增真实输出样本，就跑一次 CLI 重定向到 `internal/pcs/testdata/`，别手写——手写很容易丢掉 `\r` 和尾随空格这些关键细节。
- `internal/session` 的测试用 `/bin/sh` 当被测进程，不依赖真实的 BaiduPCS-Go。
- `internal/httpapi` 的测试用 `httptest` + `fstest.MapFS`，不起真实服务、不碰真实网盘。

## 前端约定

- **可用的 npm 依赖只有这几个：`react`、`react-dom`、`@xterm/xterm` + `@xterm/addon-fit`、`react-icons`、`uplot`。** 不要引入 UI 框架、Tailwind、axios、路由库或状态管理库。样式用普通 CSS + CSS 变量，深色主题。
- 图标一律用 `react-icons/lu`（Lucide）。**新增图标前先确认名字存在**：`grep "declare const LuXxx:" node_modules/react-icons/lu/index.d.ts`。
- 文件类型图标走 `src/fileIcon.ts` 的 `renderFileIcon(name, isDir, className)`，按扩展名映射。它用 `createElement` 而不是 `const Icon = ...` + `<Icon />`，因为后者会触发 oxlint 的 `react(static-components)` 规则。新增映射请沿用这个写法。
- 终端 WebSocket 收到的**二进制帧**是终端字节，必须用流式 `TextDecoder` 解码后再喂给 xterm；且**每条连接都要新建 decoder**——重连时服务端会从头重放，沿用旧 decoder 残留的半截多字节 UTF-8 会把重放开头解坏。
- 终端 WebSocket 收到的**文本帧**是 JSON 控制消息（目前只有 `{"type":"progress",...}`），走 `handleControlMessage`。解析失败要静默忽略，不要抛出去打断终端。
- 服务端在进程结束时以 **code 1000** 主动关闭连接，那是正常结束，**不要重连**；只有异常断开才退避重连。
- 因此终端不能只依赖 taskId：**点「继续」时任务 ID 不变，但后端会替换掉整个会话**（新进程、新输出缓冲），而旧连接早已按 1000 正常关闭。只看 taskId 的话 effect 不会重跑，前端会一直挂在死连接上，必须刷新页面才有内容。`useTerminal(taskId, runId)` 的第二个参数传 `startedAt` 当「这一轮运行」的标识，重启瞬间就会自动重连。改这块务必连同 `useTasks` 的签名一起想——`startedAt` 必须参与签名，否则列表状态不更新，runId 也就永远不变。
- 任务列表靠 `GET /api/tasks` **每 2 秒轮询**（后端没有列表推送）。`useTasks` 用签名比对跳过内容未变的 setState；**签名里必须带上 progress 和 resumable**，否则列表上的进度条不会动。
- 选中任务的进度有两条来源：列表轮询（2 秒）和终端 WebSocket（1 秒）。`App` 用后者覆盖 `listTasks` 里对应的那一条，让正在看的任务跟手一些。
- React Compiler 已启用；StrictMode 会双调用 effect，创建 xterm / WebSocket 的地方要保证 cleanup 干净。

### 速度曲线（`uplot`）

后端为每个任务在内存里保留最近 300 个速度采样（`speedHistory`，进度行约每秒一条，即最近 5 分钟），
**不落盘**——曲线是「看当下」的东西，而任务 JSON 要被每 2 秒轮询，塞进去只会让它变胖。

- WS 连接时下发一次 `{"type":"speedHistory","times":[...],"speeds":[...]}`；之后每条 `progress`
  消息带一个 `t`（Unix 毫秒），前端据此往尾巴上追加。**时间轴统一用后端的时钟**，否则两条来源会错位。
- `SpeedChart` 是 uPlot 的薄包装：实例只建一次，数据变了 `setData`，容器变了 `setSize`。
  注意 uPlot 的 x 轴单位是**秒**，传进去要 `/1000`。
- 合计速度曲线不一样：后端没有「所有任务总和」这种东西，它是 `useTasks` 在**轮询回调里**采样的
  （约 2 秒一个点），刷新页面就没了。采样放轮询回调而不是 effect 里，否则会触发
  oxlint 的 `react(set-state-in-effect)`——顺带也确实是「由引起变化的事件来更新」这个更正确的做法。

## 设计系统

定位是一台**传输仪表台**：内容全是终端输出和传输数字，用的人是隔一会儿瞥一眼。整套视觉围绕
「数据要一眼可读、装饰要几乎没有」来做。规则写在 `webapp/src/index.css` 顶部的注释里，改动前先看。

- **一层表面，发丝线分区。** 全站只有 `--ink-1` 一层底，分区一律靠 `1px solid var(--line)`。
  **不要用卡片**（圆角 + 边框 + 阴影的组合）、不要渐变。唯一会浮起来的是模态和轻提示。
- **颜色只在有含义的地方出现**：`--live` 正在传、`--hold` 已停止、`--done` 已完成、`--bad` 出错。
  其余全部走灰阶。不要把强调色用在纯装饰上。
- **数据一律等宽 + 等宽数字**：百分比、大小、速度、时间、文件名、路径都用 `--mono` 和
  `font-variant-numeric: tabular-nums`；说明文字才用无衬线。
- **只有两个圆角**：`--r-sm`（4px，内联小件）、`--r`（8px，模态）。
- **图标标注动作和数据类型，不装饰标题**。按钮、状态徽章、文件类型、空状态、导航项用图标；
  页面标题、小节标题、统计项不放图标。
- **全站只有一个动效**：`@keyframes spin`（运行中的图标）。已按 `prefers-reduced-motion` 降级。
- **不引外部字体**：这是内嵌进单个二进制的局域网工具，不该为了字体多一次网络请求。

界面里唯一「用力」的地方是 `ProgressBar`：百分比用 30px 等宽数字立在最前，下面是贯穿整幅宽度的
进度尺，再往下才是细节。别的地方保持安静。

移动端断点 **860px**（`@media (max-width: 860px)`）：侧栏和工作区变成两个互斥的全屏视图，
靠 `.app` 上的 `data-pane="list|detail"` 切换，`.workspace` 从右侧滑入。返回按钮 `.back-btn`
只在窄屏出现。另外做了：触摸目标不小于 44px、模态变贴底抽屉、`100dvh` 而不是 `100vh`、
`safe-area-inset` 留白、`viewport-fit=cover`。

改完样式后用 `task check` 验证；改类名时注意**动态拼出来的变体**（`badge-${status}`、
`term-${state}`、`toast-${kind}`、`is-dir`、`is-running`...），脚本扫不出来，要手工核对。

## 界面状态

- 没选中任务时右侧显示**概览页**（`Overview.tsx`）：统计条 + 正在下载列表 + 合计速度曲线 + 运行环境。
  侧栏列表上方有一个常驻的「概览」入口（`.nav-item`），否则选中任务后就回不去了。
- **任务详情页的主体是任务信息**（`TaskInfo.tsx`），不是终端。自上而下：工具栏（任务名 + 操作）、
  进度读数、速度曲线、可滚动的任务信息，最后是**浮在底部的终端浮层**（`.terminal-sheet`）。
- 终端浮层收起时只在底部露出一条悬浮拉手，展开时整块浮上来（`transform: translateY`，不是改高度）。
  **整条把手都是点击目标**（它是一个 `<button>`），所以提示只能是 `<span className="terminal-sheet-hint">`
  ——按钮里不能再套按钮。展开高度是 `--dock-height`（桌面 `min(70dvh, 680px)`，窄屏 `min(80dvh, 760px)`）。
  两个要点：
  1. **用 transform 而不是高度**，浮层因此始终拥有真实尺寸（只是被推到视口外），FitAddon 任何
     状态下都能量到正确行列数，不用处理 0 尺寸。别改成 `display: none` 或 `height: 0`。
  2. 左右始终留出 `--dock-gap`，**宽度不变**，所以动画期间不会引发 ResizeObserver 抖动。
- 浮层展开时同时做两件事：压一层 `.terminal-sheet-backdrop` 遮罩（点它收起，用 `<button>` 写以便
  自带点击语义和键盘可达性，`tabIndex={-1}` 免得进 Tab 序列），并给 `.detail-body` 加
  `overflow: hidden` **锁住背后滚动**——遮罩已经压上去了，背后还能滚会让人分不清哪层在上。
- `SpeedChart` 拆成了外层（判空）+ 内层 `Plot`（只在确定有数据时挂载）。**这个拆分不能合并**：
  uPlot 实例是在 effect 里用 ref 建的，如果同一个组件有时渲染占位 div、有时渲染图表容器，
  effect 首次运行就取不到 ref，而依赖数组里只有 height，之后再也不会重跑——图永远出不来。
  加曲线那天就是这么错的，任务详情和概览两处都不显示。
- 任务信息里有两点是刻意的：**不显示退出码**（CLI 出错时也是 0，列出来只会误导），
  **复制按钮平时透明、悬停/聚焦才出现**（触摸设备上常显）。
- **终端容器始终挂载**（`ref` 必须一直在），没输出时只是加 `.is-hidden` 设成 `visibility: hidden`，
  而不是不渲染。用 `visibility` 而不是 `display` 是因为 FitAddon 要能量到尺寸，`display: none` 会量到 0。
  「有没有输出」由 `useTerminal` 的 `hasOutput` 判断（收到过非空二进制帧）。停止且从未有输出的任务
  （典型是服务重启后恢复出来的）显示空状态而不是一块黑窗口。

## 打包、部署与发版

```bash
VERSION=0.1.0 task package    # 交叉编译 amd64+arm64，打成 deb / rpm（需要 nfpm）
task deploy                    # 装成 systemd 用户服务，本地开发用
```

- **版本号**由构建时注入：`-ldflags "-X main.version=..."`，只改 `main.go` 里那个 `var version`。
  没注入时是 `dev`。`--version` 打印后退出；运行中的实例把它显示在概览页「运行环境」，
  也写进 `GET /api/health` 的 `version`。
- **打包配置** `deploy/nfpm.yaml` 里 `${ARCH}` `${VERSION}` 是占位符，由 `scripts/package.sh`
  用 sed 渲染。nfpm 会自动把 `amd64`/`arm64` 映射成 rpm 的 `x86_64`/`aarch64`，所以一份架构名
  对两种包都成立，不用分别渲染。
- **有两条 unit，别搞混**：`deploy/baidupcs-console.service` 是打进包里、装到
  `/usr/lib/systemd/user/` 的那份（`ExecStart=/usr/bin/...`）；`deploy/baidupcs-console.service.in`
  是 `task deploy` 本地用的模板（装到 `~/.local/bin/`）。改一处记得看另一处，两份文件头部都写了这条。
- 包**不自动启用服务**：用户 unit 没法由安装脚本替某个用户开启，而且这程序要读那个用户的
  `~/.config/BaiduPCS-Go/` 登录态、写他的 `~/Downloads`，默认跑成谁都不对。postinstall 只打印该敲的命令。
- **预发布版本**（tag 里带 `-`，如 `v0.1.0-test.1`）必须显式 `--prerelease`：GitHub **不会**
  只看 tag 名字自动判断，不标的话它会显示成仓库的 Latest release，看起来像能直接用的正式版。
- **别被 Release 附件的文件名骗了**：nfpm 产出的包名是 `0.1.0~test.1`（`~` 让预发布排在正式版
  之前，这是对的），但 GitHub 上传附件时会把 `~` 换成 `.`，附件名变成 `0.1.0.test.1`。
  包**内部**的版本号不受影响（`dpkg-deb -f x.deb Version` 查得到），升级顺序是对的。
- **发版**：`.github/workflows/release.yml`，推 `v*` tag 触发。顺序是「先构建前端再交叉编译」——
  后端是 `go:embed` 把前端嵌进去的，反了打出来的包前端会是空的。也支持 `workflow_dispatch`
  手动跑一遍验证流程（手动跑不创建 Release）。

## 安全

服务**不鉴权**，默认监听 `0.0.0.0`，按局域网自用设计。`upgrader.CheckOrigin` 恒返回 true，任何站点都能连上终端。不要把它暴露到公网，也不要在没有明确要求的情况下"顺手加上"鉴权——需要时先问用户。
