# 百度网盘下载控制台 · Web UI

`BaiduPCS-Go` 下载服务的 Web 控制台前端：左侧管理下载任务，右侧用 xterm 实时查看
当前任务的原始终端输出。

后端不解析任何 CLI 输出，只把进程的 stdout/stderr 原始字节通过 WebSocket 透传给浏览器，
所以进度条（CLI 用 `\r` 刷新）由 xterm 天然正确渲染，任务对象里没有 progress / speed / eta 字段。

## 技术栈

- Vite + React 19 + TypeScript（严格模式）
- `@xterm/xterm` + `@xterm/addon-fit`：终端渲染与自适应
- 原生 `fetch`、原生 WebSocket、原生 CSS（深色主题）

## 开发

后端默认监听 `:8080`，dev server 会把 `/api`（含 WebSocket）代理到 `http://localhost:8080`。

```bash
pnpm install
pnpm dev      # http://localhost:5173
```

## 构建与检查

```bash
pnpm build    # tsc -b && vite build
pnpm lint     # oxlint
```

## 目录结构

```
src/
  api.ts                    # 接口封装与类型定义（契约见 ../docs/API.md）
  format.ts                 # 时间格式化
  useTasks.ts               # 任务列表：每 2 秒轮询 + 操作封装
  useTerminal.ts            # xterm 实例 + 终端 WebSocket（重放、重连、自适应）
  App.tsx                   # 左右分栏布局：任务列表 + 终端
  components/
    TaskList.tsx            # 左栏任务列表容器
    TaskRow.tsx             # 单个任务项（名称 + 状态）
    AddTaskDialog.tsx       # 新建下载对话框
    Browser.tsx             # 网盘文件浏览器（面包屑 + 目录浏览）
    StatusBadge.tsx         # 任务状态徽章
    Modal.tsx               # 通用模态框
    Toast.tsx               # 轻提示
```

## 状态说明

| status    | 含义                       | 可用操作           |
| --------- | -------------------------- | ------------------ |
| `running` | 进程正在跑                 | 停止 / 删除        |
| `exited`  | 进程自然退出               | 重新开始 / 删除    |
| `killed`  | 被 /kill 或 /restart 结束  | 重新开始 / 删除    |

`exited` 只表示进程退出了，**不代表下载成功**：CLI 出错时退出码同样是 0，成败要看终端输出。
