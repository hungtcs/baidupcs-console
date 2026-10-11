#!/bin/sh
# 包安装后打印的提示。
#
# 这里刻意**不去**自动 enable 服务：这是个 systemd 用户 unit，安装脚本跑在 root 下，
# 没法替某个普通用户启用他的用户服务。而且这个程序要读那个用户的
# ~/.config/BaiduPCS-Go/ 登录态、写他的 ~/Downloads，默认跑成谁都不对——
# 必须由用它的那个人自己决定。
set -e

cat <<'EOF'

baidupcs-console 已安装到 /usr/bin/baidupcs-console。

它按局域网自用设计：不对请求做任何鉴权，并监听 0.0.0.0:8080。不要暴露到公网。

用它的人自己启用服务（换成你自己的用户名下的终端）：

    systemctl --user daemon-reload
    systemctl --user enable --now baidupcs-console
    loginctl enable-linger "$USER"        # 不登录也开机自启

然后浏览器打开 http://<本机IP>:8080

如果系统开了 firewalld，局域网里的其他设备还连不上，需要放行端口：

    sudo firewall-cmd --add-port=8080/tcp --permanent && sudo firewall-cmd --reload

前提：BaiduPCS-Go 已安装并登录（BaiduPCS-Go login）。它不属于这个包，
装好后如果 systemd 找不到它，用下面的命令补上绝对路径：

    systemctl --user edit baidupcs-console
    # 在打开的编辑器里写：
    # [Service]
    # Environment=BPCS_CLI_PATH=/usr/local/bin/BaiduPCS-Go

日志：journalctl --user -u baidupcs-console -f
EOF
