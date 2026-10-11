# BaiduPCS-Console API 契约

后端默认监听 `:8080`。开发模式下 Vite dev server（`:5173`）把 `/api` 代理到后端。

- 所有请求/响应均为 JSON（终端 WebSocket 除外）。
- 错误统一为 `{"error": "<message>"}`，并带合适的 HTTP 状态码（400/404/500）。
- **无鉴权**：按局域网自用设计，服务直接监听 `0.0.0.0`。请勿暴露到公网。

## 设计要点

后端**不解析任何 CLI 输出**。它只做三件事：把 `BaiduPCS-Go` 当后台进程跑起来、把原始
stdout/stderr 字节透传给浏览器、提供列目录和任务增删这几个接口。进度条怎么显示、`\r`
怎么覆盖、ANSI 颜色怎么解析，全部由前端的 xterm 处理。

因此**任务对象里没有 `progress` / `speed` / `eta` 这类字段**，界面上的进度直接来自终端输出。

## 类型

### Task

```jsonc
{
  "id": "t_e48d3982",
  "name": "595274 ARM Cortex-M3与Cortex-M4权威指南.pdf",  // 展示名
  "remotePath": "/我的资源/595274 ARM Cortex-M3与Cortex-M4权威指南.pdf",
  "saveDir": "/home/hungtcs/Downloads",   // 落盘目录（即 --saveto 的值）
  "args": ["download", "--saveto", "/home/hungtcs/Downloads", "--mode", "locate", "/我的资源/..."],
  "commandLine": "BaiduPCS-Go download --saveto /home/hungtcs/Downloads --mode locate /我的资源/...",
  "status": "running",       // running | exited | killed
  "exitCode": 0,             // killed 时为 -1；注意 CLI 出错时退出码也是 0
  "pid": 100742,
  "startedAt": "2026-10-11T21:46:03.935747203+08:00",
  "finishedAt": null         // RFC3339 或 null
}
```

`status` 的三个取值：

| 值 | 含义 |
| --- | --- |
| `running` | 进程正在跑 |
| `exited` | 进程自然退出（**注意：CLI 出错时退出码也是 0，不能靠退出码判断成败，要看终端输出**） |
| `killed` | 进程是被 `/kill` 或 `/restart` 主动结束的，**或者服务重启导致进程被杀** |

任务记录会持久化到 `<BPCS_DATA_DIR>/tasks/<id>.json`，服务重启后自动恢复。上次是 `running`
的任务恢复出来一律是 `killed`（进程确实随服务退出了），点「重新开始」即可从断点续传。

> **终端输出不持久化。** 恢复出来的任务没有终端历史，连上 WebSocket 会立刻收到关闭帧
> （code 1000）。

## 网盘浏览

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/browse?path=/` | 列出目录。`path` 省略时默认为 `/`。 |

返回：

```jsonc
{
  "path": "/我的资源",
  "parent": "/",
  "entries": [
    {
      "name": "Document",
      "path": "/我的资源/Document",   // 完整路径，可直接用于创建任务
      "isDir": true,

      "size": 0,                      // 解析出的字节数，目录为 0
      "sizeText": "-",                // CLI 大小列的原样文本，目录为 "-"

      "modTime": "2015-12-12T18:40:20+08:00",  // RFC3339
      "modTimeText": "2015-12-12 18:40:20"     // CLI 日期列的原样文本
    }
  ]
}
```

**`entries` 的顺序就是 `BaiduPCS-Go ls` 的原始顺序**，后端不排序，前端也不应重排。

`sizeText` / `modTimeText` 存在的意义是让前端**原样显示**，与 CLI 输出逐字一致。不要拿
`size` 自己去格式化——CLI 的大小列有自己的精度（例如 `465.96MB`），从字节数反推不保证能还原。

路径不存在时返回 `400 {"error":"列出 /nope-xyz 失败: 遇到错误, ... 消息: 文件不存在"}`。

## 任务

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/tasks` | `{"tasks":[Task]}`，最新的在前。 |
| `POST` | `/api/tasks` | body `{"remotePath":"/我的资源/a.pdf"}` → `201 Task`。会先校验路径存在，再启动后台进程。 |
| `GET` | `/api/tasks/{id}` | `Task` |
| `POST` | `/api/tasks/{id}/kill` | 结束进程，**保留**任务记录与已下载的分片 → `Task` |
| `POST` | `/api/tasks/{id}/restart` | 用同样的命令重新起进程，任务 ID 不变。BaiduPCS-Go 会从断点续传 → `Task` |
| `DELETE` | `/api/tasks/{id}` | 结束并移除任务，磁盘上已下载的文件不动 → `204` |

## 其它

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/health` | `{"ok":true,"cliPath":"BaiduPCS-Go","saveRoot":"/home/hungtcs/Downloads","mode":"locate"}` |

## 终端 WebSocket

`GET /api/tasks/{id}/terminal`

**服务端 → 客户端**：全部是**二进制帧**，内容是进程 stdout/stderr 的原始字节，
前端应直接丢给 `term.write()`。

连接建立时会先把该任务已积累的输出**整体重放一遍**（内存里最多保留 4MB，超出后丢弃最旧的部分），
所以刷新页面、或任务跑了一半才连上，都能看到完整历史。进程结束时服务端会主动关闭连接，
关闭码 `1000`，reason 为 `进程已结束`。

**客户端 → 服务端**：
- **二进制帧** → 写进进程 stdin。
- **文本帧** → 预留的 JSON 控制消息，当前后端会忽略。

### 前端解码注意

UTF-8 多字节字符可能被拆在两个帧之间，**不要**用 `String.fromCharCode` 逐字节转，
要用流式的 `TextDecoder`：

```ts
const decoder = new TextDecoder('utf-8')
ws.onmessage = (e) => {
  const bytes = new Uint8Array(e.data)
  term.write(decoder.decode(bytes, { stream: true }))
}
```

（`term.write()` 本身也接受 `Uint8Array`，但自己用流式 TextDecoder 更稳妥。）
