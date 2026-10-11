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
  "isDir": false,            // 下载的是整个目录（界面据此区分文件夹/文件图标）
  "resumable": true,         // 磁盘上还有未下完的分片，重跑会从断点接着下
  "args": ["download", "--saveto", "/home/hungtcs/Downloads", "--mode", "locate", "/我的资源/..."],
  "commandLine": "BaiduPCS-Go download --saveto /home/hungtcs/Downloads --mode locate /我的资源/...",
  "status": "running",       // running | exited | killed
  "exitCode": 0,             // killed 时为 -1；注意 CLI 出错时退出码也是 0
  "pid": 100742,
  "startedAt": "2026-10-11T21:46:03.935747203+08:00",
  "finishedAt": null,        // RFC3339 或 null
  "progress": { /* 可选，见下 */ }
}
```

### Progress

从 CLI 输出里解析出来的进度。**终端输出仍然是权威的**，这只是给界面用的一份结构化快照；
没有进度行时该字段不存在（`undefined`）。

目录下载时 CLI 逐文件打印，且事先不告诉目录总大小，所以这里以**当前正在下载的那个文件**
为准，另外给出文件计数。单文件任务下 `filesTotal` 就是 1。

```jsonc
{
  "currentName": "adx.gif",   // 当前文件名，来自「准备下载: <路径>」
  "doneText": "1.88MB",       // 原样文本，直接显示才能与终端一致
  "totalText": "139.20MB",
  "doneBytes": 1971322,
  "totalBytes": 145961465,
  "percent": 1.35,            // 0-100；总量未知时为 0
  "speedText": "100.99KB/s",
  "speed": 103414,            // 字节/秒
  "elapsedText": "54s",
  "etaText": "23m12s",        // 原样，未知时是 "-"
  "filesDone": 0,
  "filesTotal": 1,
  "finished": false           // 看到过「下载结束」
}
```

`status` 的三个取值：

| 值 | 含义 |
| --- | --- |
| `running` | 进程正在跑 |
| `exited` | 进程自然退出（**注意：CLI 出错时退出码也是 0，不能靠退出码判断成败，要看终端输出**） |
| `killed` | 进程是被 `/kill` 或 `/restart` 主动结束的，**或者服务重启导致进程被杀** |

任务记录会持久化到 `<BPCS_DATA_DIR>/tasks/<id>.json`，服务重启后自动恢复。上次是 `running`
的任务恢复出来一律是 `killed`（进程确实随服务退出了），点「继续」即可从断点续传。

### `resumable` 与「继续 / 重新开始」

`resumable` 由后端读文件系统判定：CLI 会给没下完的文件留一个
`<文件名>.BaiduPCS-Go-downloading` 标记，下完就删掉。所以

- 有标记（单文件看目标旁边的标记，目录则看树里有没有任意一个）→ `resumable: true` → 界面显示「继续」；
- 没有标记（还没开始下、或已经下完）→ `resumable: false` → 界面显示「重新开始」。

它只在进程结束后算一次（进程结束时、以及服务启动恢复任务时），不在每次列表轮询时现算——
那要读文件系统，不能每 2 秒来一遍。

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

**服务端 → 客户端**：

- **二进制帧**：进程 stdout/stderr 的原始字节，前端应直接丢给 `term.write()`。
- **文本帧**：JSON 控制消息，目前有两种。

  进度：

  ```jsonc
  {
    "type": "progress",
    "progress": { /* 同 Task.progress */ },
    "t": 1728660000000        // 这条采样的 Unix 毫秒时间，用于给速度曲线打点
  }
  ```

  连接建立后立刻推一次，之后每秒读一次任务快照、**有变化才发**；进程结束时再发一次最终进度，
  然后才关闭连接。

  速度历史（**连接建立时发一次**，之后由 progress 的 `t` 往尾巴上追加）：

  ```jsonc
  {
    "type": "speedHistory",
    "times":  [1728659700000, 1728659701000],  // Unix 毫秒
    "speeds": [103414, 0]                      // 字节/秒
  }
  ```

  后端为每个任务保留最近 300 个采样点（进度行约每秒一条，即最近 5 分钟）。没有数据时
  这一帧不会发。**采样只在内存里，不落盘**，所以服务重启后曲线从零开始。

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
