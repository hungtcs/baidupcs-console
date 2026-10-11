# BaiduPCS-Console

给 [BaiduPCS-Go](https://github.com/qjfoidnh/BaiduPCS-Go) 套一个网页界面：在局域网里用浏览器（包括手机）浏览网盘、发起下载、看实时进度。

下载和浏览都由本机已登录的 BaiduPCS-Go 完成，这个程序只是把它跑起来并把结果呈现出来。

## 界面

左边是任务列表，右边是概览页或任务详情页，终端浮在底部。

- **概览**：任务统计、正在下载的进度、合计速度曲线、运行环境。
- **任务详情**：以任务信息为主体（网盘路径、保存位置、耗时、能否续传、完整命令），
  上面是进度读数和速度曲线，底部是可上滑展开的终端——CLI 的原始输出，一字节不改。
- **终端**：进度条、`\r` 刷新、ANSI 颜色全部原样呈现。后端不解析它、也不改写它。

## 依赖

- Linux（x86_64 或 arm64）
- **BaiduPCS-Go 已安装并登录**（`BaiduPCS-Go login`）——它不随本程序分发，需要你自己装
- systemd（用于后台常驻）

## 安装

### 从 deb / rpm

到 [Releases](https://github.com/hungtcs/baidupcs-console/releases) 下载对应架构的包：

```bash
# Debian / Ubuntu
sudo dpkg -i baidupcs-console_<版本>_amd64.deb

# Fedora / RHEL
sudo rpm -i baidupcs-console-<版本>-1.x86_64.rpm
```

装完**不会自动启用服务**——这是个 systemd **用户**服务，因为它要读你自己的
`~/.config/BaiduPCS-Go/` 登录态、写你自己的 `~/Downloads`，跑成别的身份都不对。
所以由使用它的那个人自己启用：

```bash
systemctl --user daemon-reload
systemctl --user enable --now baidupcs-console
loginctl enable-linger "$USER"      # 不登录也开机自启
```

然后浏览器打开 `http://<本机IP>:8080`。

### 从源码

需要 Go 1.25+、Node 20+、pnpm、[Task](https://taskfile.dev/)：

```bash
task build      # 构建前端 + 后端 → build/bpcs-console
./build/bpcs-console
```

想把源码装成服务：`task deploy`。详见 [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)。

## 装完之后

**局域网里其他设备连不上** —— Fedora/RHEL 默认开着 firewalld，放行一下：

```bash
sudo firewall-cmd --add-port=8080/tcp --permanent && sudo firewall-cmd --reload
```

**服务起不来** —— 先看日志：`journalctl --user -u baidupcs-console -n 50`。
撞端口之类的问题会被 `StartLimitBurst` 拦下，处理完之后要
`systemctl --user reset-failed baidupcs-console` 才能再启动。

**提示找不到 BaiduPCS-Go** —— unit 里没写死它的路径（它不属于这个包，装在哪由你决定）。
如果 systemd 的 PATH 找不到它，给服务加个 override：

```bash
systemctl --user edit baidupcs-console
# [Service]
# Environment=BPCS_CLI_PATH=/usr/local/bin/BaiduPCS-Go
```

## 配置

全部走环境变量，可选地写进 `.env`（参考 [.env.example](.env.example)）。
优先级：进程环境变量 > `.env` > 内置默认值。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BPCS_LISTEN` | `0.0.0.0:8080` | HTTP 监听地址 |
| `BPCS_CLI_PATH` | `BaiduPCS-Go` | BaiduPCS-Go 可执行文件路径 |
| `BPCS_SAVE_ROOT` | `~/Downloads` | 下载落盘根目录 |
| `BPCS_DATA_DIR` | `data` | 任务记录目录，一个任务一个 JSON |
| `BPCS_MODE` | `locate` | 下载模式：`locate` / `pcs` / `stream` |
| `BPCS_MAX_THREADS` | `0` | CLI 的 `-p` 参数，0 用 CLI 默认 |
| `BPCS_OUTPUT_BUFFER_BYTES` | `4194304` | 每个任务在内存里保留的输出上限 |

改配置后重启服务生效：

```bash
systemctl --user restart baidupcs-console
```

## ⚠️ 安全

**这个服务不鉴权，并且默认监听 `0.0.0.0`。** 任何能访问到这个端口的人都可以浏览你的网盘内容、发起下载。**不要暴露到公网。** 需要收紧的话把 `BPCS_LISTEN` 改成 `127.0.0.1:8080`，再通过 SSH 隧道访问。

## 已知行为

- **CLI 出错时退出码也是 0**，所以不能靠退出码判断成败。任务列表里的「已结束」只表示进程退出了，不代表下载成功——成败要看终端输出。
- 目标文件已存在时 CLI 会跳过（打印 `文件已存在: ..., 跳过...`），所以重复下载会瞬间「结束」。
- 「继续 / 重新开始」由磁盘上有没有未下完的分片决定：有分片就是「继续」（走 CLI 的断点续传），没有就是「重新开始」。
- **终端输出不持久化**：只放在内存里（每任务上限 4MB）。重启后任务还在，但点开终端是空的，得「继续」才有新输出。
- 任务记录持久化在 `<BPCS_DATA_DIR>/tasks/<id>.json`，重启后自动恢复。上次在跑的任务恢复成「已停止」。
- **重启服务会打断正在跑的下载**（分片和续传信息都保留，点「继续」接着下）。
- 所有任务平铺下载到 `BPCS_SAVE_ROOT`，不建每任务子目录；不同网盘目录下的同名文件会冲突，后者被跳过。

## 文档

| 文档 | 内容 |
| --- | --- |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | 部署详解：两种装法、开机自启、日志、更新、卸载 |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | 开发：设计取舍、构建、测试、目录结构 |
| [docs/RELEASE.md](docs/RELEASE.md) | 打包与发版 |
| [docs/API.md](docs/API.md) | HTTP 接口与 WebSocket 协议 |

## 许可

[MIT](LICENSE)
