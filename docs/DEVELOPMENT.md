# 开发

## 设计取舍

**后端不解析 CLI 的终端输出。** 它只做三件事：把 `BaiduPCS-Go` 当后台进程跑起来、
把进程的 stdout/stderr **原始字节**通过 WebSocket 透传、提供列目录和任务增删这几个接口。

进度条、`\r` 覆盖刷新、ANSI 颜色全部交给前端的 xterm.js。这样后端不需要跟着 CLI 的输出格式
走，CLI 升级换格式也不会把终端弄坏。

在此之上做了两处**额外**解析，只为了让界面更好用，不取代终端：

- **`ls`** → 可点击的网盘目录浏览；
- **`download` 的进度行** → 终端上方的进度读数与速度曲线。

两处解析都保留 CLI 的**原样文本**（如 `465.96MB`、`100.99KB/s`），前端直接显示，
避免从字节数反推格式带来的偏差。解析是旁路：正则匹配不上就什么都不做，
**绝不因为解析不出进度而丢掉终端字节**。

## 技术栈

| 层 | 选型 |
| --- | --- |
| HTTP | [echo v5](https://github.com/labstack/echo) |
| 配置 | [cleanenv](https://github.com/ilyakaznacheev/cleanenv)（dotenv + 环境变量） |
| WebSocket | [gorilla/websocket](https://github.com/gorilla/websocket) |
| 前端 | Vite + React 19 + TypeScript |
| 终端 | [@xterm/xterm](https://xtermjs.org/) |
| 图标 | [react-icons](https://react-icons.github.io/react-icons/)（Lucide 集） |
| 图表 | [uPlot](https://github.com/leeoniya/uPlot) |
| 打包 | [nfpm](https://nfpm.goreleaser.com/) |
| 任务入口 | [Task](https://taskfile.dev/) + `scripts/` |

前端**不引 UI 框架、不用 Tailwind**，样式是普通 CSS + CSS 变量。

## 构建与运行

```bash
task build      # 前端 + 后端 → build/bpcs-console
task backend    # 只构建后端，复用 webapp/dist 里已有的前端产物
task web        # 只构建前端
task run        # 构建并运行
task dev        # 后端 :8080 + 前端 dev server :5173（/api 已代理，带热更新）
task check      # go vet + go test + gofmt + tsc + oxlint
task test       # 只跑后端单元测试
task clean      # 清理构建产物
```

跑单个测试：

```bash
go test ./internal/pcs/ -run TestParseLsRoot -v
go test ./internal/session/ -run TestRingBuffer -v
go test -race ./...            # 涉及并发改动时跑这个
```

**顺序不能反**：后端是 `go:embed` 把前端产物嵌进二进制的，所以必须先构建前端再编译后端。

## 目录结构

```
main.go              入口、优雅退出（先 KillAll 再 Shutdown）
assets.go            go:embed all:webapp/dist（必须在根包，embed 不能引用上级目录）
internal/config      cleanenv 读 .env + BPCS_* 环境变量
internal/pcs         exec CLI、解析 ls/meta 文本、解析 download 进度行
internal/session     后台进程、输出环形缓冲、订阅广播、任务持久化
internal/httpapi     echo v5 路由、终端 WebSocket
webapp/              Vite + React 前端
deploy/              systemd unit、nfpm 打包配置
scripts/             构建、打包、安装服务的脚本
docs/                本文档与接口说明
```

## 测试约定

- `internal/pcs/pcs_test.go` 里的 CLI 输出样本是从真实 v4.0.2 上抄下来的；
  `internal/pcs/testdata/*.raw` 更是直接 `> file 2>&1` 录下来的原始字节（含 `\r`）。
  **改解析逻辑时这些用例必须继续通过，不要为了迁就新代码去改样本。**
- 解析逻辑都拆成了纯函数（`parseLs`、`parseMeta`、`parseSize`、`extractError`）或纯类型
  （`ProgressParser`），就是为了能脱离真实 CLI 做单元测试。新增解析逻辑请沿用这个模式。
- 要新增真实输出样本，就跑一次 CLI 重定向到 `internal/pcs/testdata/`，
  **别手写**——手写很容易丢掉 `\r` 和尾随空格这些关键细节。
- `internal/session` 的测试用 `/bin/sh` 当被测进程，不依赖真实的 BaiduPCS-Go。
- `internal/httpapi` 的测试用 `httptest` + `fstest.MapFS`，不起真实服务、不碰真实网盘。

> 夹具里的网盘名、文件名、md5、fs_id 已经**替换成等形的合成值**（结构、`\r`、
> 列对齐都保留，只有内容换了），避免把真实数据带进公开仓库。新增样本时请注意同样处理。

## 改接口时

`docs/API.md` 是前后端之间的唯一契约。改接口要同步改它，以及对应的前端 `webapp/src/api.ts`。

## 界面约定

设计规则写在 `webapp/src/index.css` 顶部的注释里，改动前先看。要点：

- **只有一层表面**，靠 1px 发丝线分区，不用卡片、不用阴影、不用渐变。
- **颜色只在有含义的地方出现**：正在传 / 已停止 / 已完成 / 出错。其余走灰阶。
- **数据一律等宽 + 等宽数字**，说明文字才用无衬线。
- 移动端断点 **860px**：侧栏与工作区变成两个互斥的全屏视图。
