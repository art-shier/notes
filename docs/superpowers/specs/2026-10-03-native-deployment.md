# 原生部署设计

用户希望生产服务直接在 Linux 服务器运行，并保留启动前通过 ConfigHub CLI 拉取配置的一键部署体验。

- 提供独立 install-native.sh，不改变已有 Docker 安装入口。原生模式使用现有外部 PostgreSQL，不创建生产数据库或账号。
- GitHub Release 提供 Linux amd64/arm64 的 Go + Web + 运维脚本运行包及 SHA-256。服务器不编译源码，不需要 Go/Node/Python/Docker。
- 原生脚本需要 root、systemd、curl、jq、tar、runuser；自动依赖安装仅支持 Debian/Ubuntu。复用或安装官方 ConfigHub CLI。
- 默认复用已有 HTTPS 反向代理，输出 Nginx/Caddy 配置片段，不覆盖其他站点。Go 只监听 127.0.0.1，域名必填，Cookie secure。
- 默认路径 /opt/shiji（不可变发行版本和 current）、/etc/shiji（仅 root 可读配置与元数据）、/var/lib/shiji（专用 OS 账号拥有的附件/导出）。支持经过严格验证的独立自定义路径和服务名。
- root oneshot 配置单元在首次 systemd 启动及开机时拉取配置；Go 服务以专用非登录 OS 账号运行，读取由 systemd 注入的环境并迁移。标准刷新入口先拉取和只读预检，失败不停止已有 Go 服务，再重启应用。
- 配置只选择数据库字段，支持 notes_db_* 优先、db_address/db_port 回退，require TLS，数据库目标固定。env 权限600，不 source/eval 远端数据，不输出凭据。
- 默认 systemctl restart 复用已生成配置；通过 start.sh 刷新；服务重启策略 Restart=on-failure。备份通过原生 admin.sh 运行现有 Go 命令，操作者先停止全部实例，另准备 PostgreSQL16客户端。
- 下载安装、元数据、当前发行、域名、监听端口、路径、数据库目标均保护原部署；失败不删除持久数据，禁止把 Docker 数据卷当作原生数据目录。
- 测试只使用临时目录、随机 systemd 服务/账号与临时 PG，禁止访问实际生产数据库或更改本地预览。
