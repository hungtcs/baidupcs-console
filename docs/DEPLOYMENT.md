# 部署

这个程序要常驻后台，用 **systemd 用户服务**管理。

## 为什么是用户服务，不是系统服务

它 exec 的 `BaiduPCS-Go` 从 `~/.config/BaiduPCS-Go/` 读登录态，下载落在 `~/Downloads`——
这两样都属于**用它的人**，不是属于系统。装成系统服务就是 root 身份、`HOME=/root`，
两处都找不到，得额外配 `User=` 和 `Environment=HOME=` 才能绕回来。

用户服务天然就是你的身份，再配合 `loginctl enable-linger` 一样能做到不登录也开机自启，
`Restart=on-failure` 一样能崩了自动拉起，日志一样进 journal。

## 两种装法

### 一、从包安装（推荐）

见 [README 的安装一节](../README.md#安装)。包会把二进制装到 `/usr/bin/baidupcs-console`，
unit 装到 `/usr/lib/systemd/user/baidupcs-console.service`。

包**不会替你启用服务**，因为安装脚本跑在 root 下，没法替某个普通用户开他的用户服务。
装完的提示里会给出该敲的命令。

### 二、从源码安装

```bash
task deploy
```

它做这些事：

1. 把二进制装到 `~/.local/bin/baidupcs-console`；
2. **探测 `BaiduPCS-Go` 的实际路径写死进 unit**（不依赖 systemd 的 PATH）；
3. 由 `deploy/baidupcs-console.service.in` 生成 unit 到 `~/.config/systemd/user/`；
4. `loginctl enable-linger $USER`；
5. `daemon-reload` + `enable` + `restart`。

任务记录放在 `~/.local/share/baidupcs-console`（XDG 数据目录，不在仓库里）。

> 仓库里有**两份 unit**，别搞混：`deploy/baidupcs-console.service` 是打进包、装到
> `/usr/lib/systemd/user/` 的那份（`ExecStart=/usr/bin/...`）；`deploy/baidupcs-console.service.in`
> 是 `task deploy` 本地用的模板（装到 `~/.local/bin/`）。改一处记得看另一处。

## 日常操作

```bash
systemctl --user status  baidupcs-console       # 状态
systemctl --user restart baidupcs-console       # 重启
systemctl --user stop    baidupcs-console       # 停止
journalctl --user -u baidupcs-console -f        # 跟日志
journalctl --user -u baidupcs-console -n 50     # 看最近 50 行
loginctl show-user "$USER" --property=Linger    # 确认 linger 开着
```

改端口：`BPCS_LISTEN=0.0.0.0:8081 task deploy`（包安装的话改 unit 里的 `Environment=`）。

## 更新

```bash
# 包安装
sudo dpkg -i baidupcs-console_<新版本>_amd64.deb     # 或 rpm -U
systemctl --user restart baidupcs-console

# 源码安装
task deploy
```

当前版本可以从界面上的「概览 → 运行环境」看到，或 `baidupcs-console --version`。

## ⚠️ 重启会打断正在跑的下载

`systemctl --user restart`（以及 `task deploy`）会先杀掉所有还在跑的 `BaiduPCS-Go`，再关服务。
已下载的分片和断点续传元数据都会保留，重启后任务显示为「已停止」，点「继续」就从断点接着下——
但**下载确实会中断一下**，别在下大文件时随手重启。

## 局域网访问放行端口

Fedora/RHEL 默认开着 firewalld：

```bash
sudo firewall-cmd --add-port=8080/tcp --permanent && sudo firewall-cmd --reload
```

## 起不来怎么办

```bash
journalctl --user -u baidupcs-console -n 50
```

撞端口之类的会被 unit 里的 `StartLimitBurst=5` / `StartLimitIntervalSec=60` 拦下
（避免无限重试刷日志），处理完之后要清掉失败计数才能再启动：

```bash
systemctl --user reset-failed baidupcs-console
systemctl --user start baidupcs-console
```

## 卸载

```bash
# 包安装
sudo dpkg -r baidupcs-console      # 或 sudo rpm -e baidupcs-console

# 源码安装
systemctl --user disable --now baidupcs-console
rm -f ~/.config/systemd/user/baidupcs-console.service ~/.local/bin/baidupcs-console
systemctl --user daemon-reload
```

任务记录和已下载的文件都还在，要清就自己删：

```bash
rm -rf ~/.local/share/baidupcs-console     # 任务记录
rm -rf ~/Downloads/<你下的东西>             # 下载的文件
```

## 安全提醒

服务不鉴权并默认监听 `0.0.0.0`。只在本机用就把 `BPCS_LISTEN` 改成 `127.0.0.1:8080`，
远程通过 SSH 隧道访问：

```bash
ssh -L 8080:127.0.0.1:8080 <主机>
```
