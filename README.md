# 拾记 Notes

自托管、多用户私有笔记服务，移动端 Web 优先。首页按文件夹聚合，支持图文、标签、搜索、历史、回收站、离线 ZIP 导出。Agent 通过 Skill + REST API 读写，不需要 MCP。

服务端 Go + Gin + GORM；前端 React + TypeScript + Tiptap；生产 PostgreSQL 16，Nginx/Caddy 提供 HTTPS。

## deployctl 标准部署

notes自己的发布流水线负责测试、Docker镜像构建与标准包生成，服务器使用deployctl安装/升级。deployctl仓库现已公开，流水线读取固定审核提交的打包工具，无需配置 `PLATFORM_READ_TOKEN`；当前尚未发布新的标准部署包。启动前通过 `deploy/prepare.sh` 从ConfigHub的shier/prod生成私有raw配置并做只读数据库预检，库名默认为notes。

图片持久化按当前决定暂缓，升级或重建可能丢失本地图片/导出，后续再接OSS。发布、配置、安装与当前验证边界见 [deployctl部署指南](deploy/OPERATIONS.md)。

## 原生一行部署（无需 Docker）

适用于 Linux + systemd，支持 amd64/arm64。下载编译好的 Go/Web，通过 ConfigHub 连接已有 PostgreSQL；服务器不需要 Go、Node、Python 或 Docker。

将 ConfigHub Token 安全上传至服务器 `/root/shier-prod.token`，设为 root 所有、权限600；默认读取 `https://config.shier.art` 的 `shier/prod`，数据库为 `notes`。主机/端口沿用 `db_address`、`db_port`，使用笔记专用 `notes_db_username`、`notes_db_password`。Windows 的 CLI 配置不会自动同步到服务器。

```bash
curl -fsSL https://raw.githubusercontent.com/art-shier/notes/main/install-native.sh | sudo bash -s -- --domain notes.example.com --email you@example.com --token-file /root/shier-prod.token
```

替换域名和邮箱。脚本验证 Release SHA-256，安装或复用 ConfigHub CLI，创建专用 OS 账号，拉取及预检数据库配置，生成私有配置，迁移并启动 systemd 服务，最后输出管理员邀请。默认程序 `/opt/shiji`、配置 `/etc/shiji`、附件与导出 `/var/lib/shiji`；已有数据和账户保留。Debian/Ubuntu 自动安装缺少的 curl/jq/tar/util-linux。

**HTTPS 由现有 Nginx/Caddy 提供。** Go 只监听 `127.0.0.1:8000`；脚本生成 `/etc/shiji/nginx-location.conf`、`/etc/shiji/caddy-site.conf`，加入该域名的反向代理配置并重载后再打开邀请链接。脚本不自动修改其他站点或签发证书。

```bash
sudo systemctl status shiji.service
sudo journalctl -u shiji.service -u shiji-config.service -f
# 拉取配置并预检，成功后重启；失败保留正在运行的服务
sudo bash /opt/shiji/current/ops/native/start.sh
# 仅重启，使用最后一次配置
sudo systemctl restart shiji.service
```

开机先拉取配置，成功后启动 Go；运行中的业务请求不依赖 ConfigHub 实时在线。升级前先备份，重跑同一安装命令，可添加 `--version v0.1.0`。已发布 [v0.1.0 Linux 运行包](https://github.com/art-shier/notes/releases/tag/v0.1.0)，[完整CI](https://github.com/art-shier/notes/actions/runs/37130540349) 包含真实 systemd 与 TLS PostgreSQL 验证。原生备份另需 PostgreSQL16客户端。更多参数、域名入口、升级与备份恢复见 [原生部署指南](notes-native-deployment.md)。

## Docker 一行部署（可选）

适用于 Linux 服务器；自动安装缺失的 Docker/Git 支持 Ubuntu/Debian。其他发行版先安装 Docker Engine、Compose v2、Git、curl。先将域名 A/AAAA 解析到服务器，并开放 TCP 80/443；确保端口未被其他服务占用。建议至少 2核/4GB，并预留附件和备份磁盘空间。

替换域名和邮箱后执行：

```bash
curl -fsSL https://raw.githubusercontent.com/art-shier/notes/main/install.sh | bash -s -- --domain notes.example.com --email you@example.com
```

脚本下载项目到当前用户的 `~/notes`，生成独立随机数据库密码，构建并启动 PostgreSQL、Go/Web、Caddy，等待数据库和 HTTPS 就绪，最后输出 **24小时有效的管理员邀请链接**。打开链接自行设置密码，没有默认密码或公开注册。系统依赖安装及 Docker 权限可能要求 sudo。

配置位于 `~/notes/notes-server-go/.env`，权限600；首次邀请备份在同目录 `.bootstrap-invite`，不要公开。注册完成后重跑脚本会删除本机邀请备份。数据保存在 Docker 命名卷；项目名默认 `shiji`，保存在 `.env` 的 COMPOSE_PROJECT_NAME 中。

可在上述命令末尾的 `--email ...` 后追加 `--dir /srv/notes --project shiji`，指定安装位置和独立项目名：

```bash
curl -fsSL https://raw.githubusercontent.com/art-shier/notes/main/install.sh | bash -s -- --domain notes.example.com --email you@example.com --dir /srv/notes --project shiji
```

重复执行会保留已有配置、密码和数据，也不会自动更新已有 Git 代码。输入域名与已有配置不同会拒绝；已有非本项目目录不会覆盖；失败不会自动删除持久卷。若安装进程被强制终止，确认没有其他安装运行后再移除 `notes-server-go/.install.lock` 目录。

## ConfigHub + 已有 PostgreSQL 部署

启动流程为 **ConfigHub CLI 拉取 JSON → 校验数据库字段 → 构建及只读连接检查 → 原子生成私有 .env → 启动 Go/Web 与 Caddy**。该模式不启动本地 PostgreSQL。

先在已有 PostgreSQL16 中准备独立的 `notes` 数据库和专用账号。启动脚本要求目标库为空库或已经迁移的笔记库，不创建外部数据库，也不会把已有独立部署的数据自动搬过去。推荐在 ConfigHub 配置 `notes_db_address`、`notes_db_port`、`notes_db_username`、`notes_db_password`；缺少这些专用字段时使用 `db_address`、`db_port`、`db_username`、`db_password`。数据库名通过 `--database-name` 指定，默认 `notes`。目前 `shier/prod` 已配置笔记专用账号 `notes_app`，主机和端口复用共享字段。

在**实际部署服务器**配置 CLI 的 Token，或准备一个只有运行用户可读的 Token 文件（Linux权限600）。本机 Windows 的 CLI 配置不会自动同步到服务器。然后替换域名和邮箱执行：

```bash
curl -fsSL https://raw.githubusercontent.com/art-shier/notes/main/install.sh | bash -s -- --domain notes.example.com --email you@example.com --config-hub --config-hub-project shier --config-hub-env prod --database-name notes --token-file /private/shier-prod.token
```

安装器复用已安装的 ConfigHub CLI；缺少时从其官方 Release 安装并校验 SHA-256，放在项目 `.tools` 目录。服务器已有全局 CLI Token 时可省略 `--token-file`。ConfigHub URL 默认 `https://config.shier.art`，可通过 `--config-hub-url` 指定其他 HTTPS 服务。

以后从相同部署用户的终端启动或刷新配置：

```bash
cd ~/notes/notes-server-go
bash ops/start.sh
```

项目/环境、域名等启动参数保存在 `.config-hub.json`；数据库目标记录在 `.database-target`；凭据只进入权限600的 `.env`，不会输出到日志。拉取失败、字段错误或只读数据库预检失败时，保留上次配置和运行容器。密码更新经检查后可生效；主机、端口、库名、账号或 Compose 项目改变会拒绝自动切换。`notes_db_sslmode` 可设为 `require`（默认）、`verify-ca` 或 `verify-full`；后两项要求容器具备对应 CA 信任配置。

`docker compose restart` 和服务器重启后的 Docker 自动恢复会使用最后一次生成的配置；需要重新拉取时执行 `ops/start.sh`。ConfigHub 只用于启动前拉取，业务请求不依赖它实时在线。直接运行 `docker compose config` 会显示数据库连接信息，请使用 `--quiet` 做检查。

## Docker 常用运维

```bash
cd ~/notes/notes-server-go
docker compose ps
docker compose logs --tail 100 app caddy
docker compose exec app shiji invite --admin-email you@example.com --email member@example.com
docker compose restart app
```

若当前用户不能访问 Docker，给上述命令加 sudo。不要执行 `docker compose down -v`，这会删除数据卷。

升级前完整备份，再更新与构建：

```bash
cd ~/notes/notes-server-go
sudo bash ops/backup.sh /srv/shiji-backups/新的备份目录
git -C .. pull --ff-only
docker compose build --pull app
bash ops/start.sh
bash ops/check.sh https://notes.example.com
```

备份目标目录必须不存在。恢复只允许独立空环境；详情见 [部署与恢复指南](notes-deployment-guide.md)。

## 本地开发与 Agent

### 一键安装笔记 CLI + Skill

安装在 Agent 所在机器，需要 Python 3.10+。CLI 不依赖 Docker、Git或第三方 Python 库。

Linux/macOS：

```bash
curl -fsSL https://raw.githubusercontent.com/art-shier/notes/main/install-client.sh | bash
```

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/art-shier/notes/main/install-client.ps1 | iex
```

默认将 Skill 安装到 `~/.agents/skills/shiji-notes`，命令安装到 `~/.local/bin`，使用 `shiji-notes --help` 验证。Windows脚本会将命令目录加入用户和当前终端 PATH；其他已打开的终端需要重开。Linux/macOS 若当前 PATH 没有该目录，执行 `export PATH="$HOME/.local/bin:$PATH"`，并加入所用 shell 的启动文件。重启 Agent 以加载新 Skill。

其他 Agent 可指定自己的 Skill 目录：Linux/macOS 在上述命令最后的 `bash` 后追加 `-s -- --skills-dir /你的/skills目录`；Windows下载脚本后使用 `./install-client.ps1 -SkillsDir 'C:\你的\skills目录'`。`--bin-dir` / `-BinDir` 可以自定义命令目录。已有源码也可直接运行 `python install-client.py --source-dir .`。

下载时先解析仓库提交，再从同一个提交下载三个 Skill/CLI 文件。重复安装可更新本安装器管理的文件，发现本地修改或同名非本项目命令时会停止。脚本不配置连接凭据，在 Agent 运行环境中另行设置：

```text
NOTES_API_URL=https://你的笔记域名/api/v1
NOTES_API_TOKEN=从笔记网页创建的个人访问Token
```

该 Token 属于笔记账户，与数据库密码及 ConfigHub Token 用途不同。连接验证命令：`shiji-notes folders list`。安装不会连接数据库或创建笔记账号。

- [Go 服务端运行说明](notes-server-go/README.md)：数据库迁移、CLI、测试、环境变量。
- [Web 前端](notes-web/README.md)：Vite 默认5173，API默认8000。
- [Agent Skill](notes-skill/shiji-notes/SKILL.md)：Python标准库客户端，服务端运行不依赖 Python。
- [技术方案](notes-technical-plan.md)与[Go迁移记录](notes-go-migration-plan.md)。

一键脚本有模拟命令回归；[CI](https://github.com/art-shier/notes/actions) 验证 Go 测试/竞态/vet、Web 测试/构建、安装脚本，以及真实 systemd 原生 Go/Web、TLS PostgreSQL16、Docker API 和完整备份恢复。[首轮验证已通过](https://github.com/art-shier/notes/actions/runs/37112169790)。域名证书、服务器环境和生产容量仍需在目标服务器验收；HTTPS 校验不会跳过 TLS 证书检查。

私有仓库副本可通过API下载脚本并使用临时Token克隆（Token只用于认证，不写入Git远程地址或配置）：

```bash
read -rsp 'GitHub Token: ' GITHUB_TOKEN; echo; export GITHUB_TOKEN; (set -o pipefail; curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" -H 'Accept: application/vnd.github.raw+json' 'https://api.github.com/repos/art-shier/notes/contents/install.sh?ref=main' | bash -s -- --domain notes.example.com --email you@example.com); unset GITHUB_TOKEN
```
