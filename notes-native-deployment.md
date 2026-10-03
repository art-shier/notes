# 原生部署：systemd + Go/Web + ConfigHub

Linux + systemd，支持 amd64/arm64。Go 同时提供 API 和编译好的 Web，使用 ConfigHub 中已有的 PostgreSQL；服务器不需要 Go、Node、Python 或 Docker。缺少的 curl/jq/tar/util-linux 会在 Debian/Ubuntu 自动安装，其他发行版先准备这些工具。

将 ConfigHub Token 安全上传到服务器 `/root/shier-prod.token`，设置为 root 所有、权限600。在 ConfigHub 的 `shier / prod` 中使用 `notes_db_username`、`notes_db_password`；主机与端口沿用 `db_address`、`db_port` 即可。默认数据库 `notes` 必须已存在，为独立空库或兼容笔记库；脚本不会创建外部数据库或数据库账号。Windows 本机 CLI 配置不会自动同步到服务器。

替换域名和邮箱，使用 root 或 sudo 执行：

```bash
curl -fsSL https://raw.githubusercontent.com/art-shier/notes/main/install-native.sh | sudo bash -s -- --domain notes.example.com --email you@example.com --token-file /root/shier-prod.token
```

脚本下载最新 Release 的 Go/Web 运行包并验证 SHA-256，安装或复用 ConfigHub CLI，建立专用非登录 OS 账号 `shiji`，拉取配置、预检数据库、生成权限600的配置，执行迁移并启动 systemd 服务，最后输出24小时有效的管理员邀请。重复执行保留数据和账户，拒绝自动改变域名、端口、路径及数据库目标。

**公网入口需要现有 Nginx/Caddy 提供 HTTPS。** Go 默认只监听 `127.0.0.1:8000`。脚本生成 `/etc/shiji/nginx-location.conf` 和 `/etc/shiji/caddy-site.conf`，按所用反向代理加入该域名的站点配置，检查配置后重载。脚本不自动修改现有站点、安装反向代理或签发证书。域名解析到服务器，并通过反向代理开放80/443；完成 HTTPS 后再打开邀请链接。

默认目录：`/opt/shiji` 保存发行版本、CLI 和 `current`；`/etc/shiji` 保存私有配置与首次邀请；`/var/lib/shiji` 保存附件及导出。允许通过 `--install-dir`、`--config-dir`、`--data-dir`、`--service-name`、`--port` 指定互不嵌套的独立位置与非特权端口。目录不支持空格、符号链接；程序、配置、单元目录及所有祖先必须root所有，禁止组/其他用户写入，不支持放在普通用户home或/tmp下。ConfigHub URL、project、env、数据库默认分别为 `https://config.shier.art`、`shier`、`prod`、`notes`，可用对应参数指定；完整参数见 [install-native.sh](install-native.sh)。

```bash
# 状态和日志
sudo systemctl status shiji.service
sudo journalctl -u shiji.service -u shiji-config.service --no-pager -n 100
# 拉取最新配置，预检成功后重启；失败保留正在运行的服务和旧配置
sudo bash /opt/shiji/current/ops/native/start.sh
# 仅重启服务，复用上次配置
sudo systemctl restart shiji.service
# 邀请其他用户
sudo bash /opt/shiji/current/ops/native/admin.sh invite --admin-email you@example.com --email member@example.com
# 验证域名的 HTTPS 与 Web
bash /opt/shiji/current/ops/check.sh https://notes.example.com
```

服务器重启时，`shiji-config.service` 先拉取 ConfigHub 配置并做只读数据库检查，成功后才启动 Go。运行中的业务请求不依赖 ConfigHub 实时在线；开机时拉取失败会阻止启动，修复后执行 `start.sh`。专用数据库账号和 Linux OS 账号分别控制数据库与本机进程权限，两者用途不同。

## 升级与故障处理

升级前完整备份，再重跑相同安装命令，可添加 `--version v0.1.0`。下载、校验和数据库预检在切换 `current` 之前完成；配置拉取失败保留已有服务。若新版本迁移或启动失败，查看 `journalctl -u shiji -u shiji-config`，不要删除持久目录。发布或启动验收失败时恢复旧程序、配置及单元；原服务此前运行且旧程序的数据库兼容检查通过时重新启动。旧发行目录保留，数据库迁移不自动回滚；若兼容检查失败，需要从备份恢复。

强制终止后若出现锁目录，确认没有其他安装/刷新进程后再移除 `/opt/shiji/.install.lock` 或 `/etc/shiji/.prepare.lock`。更新数据库密码后执行 `start.sh`；主机、端口、账号或库名改变会拒绝自动切库。普通 `systemctl restart shiji` 使用最后一次配置。

## 完整备份与恢复

先安装与 PostgreSQL16 匹配的 `pg_dump`/`pg_restore`，使用系统或 PGDG 软件源；安装器不自动安装数据库客户端。确认 `pg_dump --version`。停止该库所有写入实例，再备份。以下默认路径适用单实例，备份目标目录必须不存在，父目录应预先创建：

```bash
sudo systemctl stop shiji.service
sudo bash /opt/shiji/current/ops/native/admin.sh backup-create --output /srv/shiji-backups/新的目录 --app-stopped
sudo bash /opt/shiji/current/ops/native/admin.sh backup-verify --backup /srv/shiji-backups/新的目录
sudo systemctl start shiji.service
```

无论备份成功或失败，确认检查结果后恢复原先运行的实例。数据库、附件和 manifest 都要保存至独立的私有存储，临时 exports 不备份。避免同时运行 Docker 与原生实例写入同一库而只停止一方。

Go 恢复工具要求 PostgreSQL **完全空库，不能已有迁移表**。不要先对恢复目标运行一键安装：它会迁移并创建邀请，随后恢复会被拒绝。原生安装器目前不提供自动恢复参数。

已有原生实例可用独立的暂存配置执行恢复。例如另行创建 `notes_restore` 空库、授权笔记账号，使用当前实例的程序恢复到暂存附件目录；以下以默认路径、相同主机/账号及新的数据库名为例，由 root 在私有终端执行，正式实例保持原配置：

```bash
sudo -i
# 以下暂存目录必须是新建目录；metadata 沿用相同 ConfigHub 项目与账号
mkdir -m 700 /etc/shiji-restore-stage /var/lib/shiji-restore-stage
jq '.database="notes_restore" | .data_dir="/var/lib/shiji-restore-stage"' /etc/shiji/native.json > /etc/shiji-restore-stage/native.json
chmod 600 /etc/shiji-restore-stage/native.json
bash /opt/shiji/current/ops/native/prepare.sh --config-dir /etc/shiji-restore-stage
bash /opt/shiji/current/ops/native/admin.sh --config-dir /etc/shiji-restore-stage backup-restore --backup /private/snapshot --app-stopped
exit
```

此时只恢复新库与暂存附件，未启动新服务。确认结果后，为 `notes_restore` 安装独立原生实例，指定新的服务名、空配置/数据目录、独立端口和 `--database-name notes_restore`。安装器检查已恢复的兼容数据库，不新建管理员。先不要将公网代理指向它，停止新实例，将暂存的 `attachments` 完整复制到新实例附件目录并交给该实例的专用 OS 账号，再启动与验收。恢复保留密码及 Agent Token，清除会话与临时导出；验证登录、图文和 Agent 访问后再切换域名。

从 Docker 切换到原生不会自动搬迁附件或停止已有实例，需要先停止所有写入者，通过完整备份恢复到独立环境。尚未配置备份定时任务。
