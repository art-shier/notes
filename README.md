# 拾记 Notes

自托管、多用户私有笔记服务，移动端 Web 优先。首页按文件夹聚合，支持图文、标签、搜索、历史、回收站、离线 ZIP 导出。Agent 通过 Skill + REST API 读写，不需要 MCP。

服务端 Go + Gin + GORM；前端 React + TypeScript + Tiptap；生产 PostgreSQL 16，Nginx/Caddy 提供 HTTPS。

## deployctl 标准部署

Notes标准发布提供 AMD64/ARM64 镜像与 SHA256 校验的标准部署包。自己的 GitHub 流水线完成测试和构建，将镜像推送到 `ctl.shier.art/notes`，登记 ctl 版本并推进 stable。使用 ctl>=1.7.0，在管理台注册notes、配置prod的DATABASE_URL或DB_HOST/DB_USER/秘密DB_PASSWORD，服务器使用该项目/prod 的 deployer 凭据登录后安装：

```bash
sudo ctl login
sudo ctl install notes --prod
```

已有安装使用 `sudo ctl upgrade notes --prod`。自定义宿主机端口添加 `--port 8085`；首次管理员邀请可添加 `--set ADMIN_EMAIL=you@example.com`。构建使用项目 publisher 凭据，保存到 GitHub Secret `CTL_PUBLISH_TOKEN`。端到端步骤和环境变量优先级见 [ctl 平台模式](deploy/CONTROL-PLANE.md)。

图片持久化按当前决定暂缓，升级或重建可能丢失本地图片/导出，后续再接OSS。发布、配置、安装与当前验证边界见 [deployctl部署指南](deploy/OPERATIONS.md)。

部署入口默认域名为 `notes.shier.art`，管理模式下自定义域名在 prod 配置设置 `APP_ORIGIN=https://你的域名`。DNS需指向部署服务器，HTTPS反向代理需在服务器另行配置。

pre-install只检查ctl最终快照，使用已拉取的固定镜像执行只读database-check，不读取ConfigHub或改写配置。DATABASE_URL优先；缺失时Go从DB_HOST/DB_USER/DB_PASSWORD安全组装，端口/库名/TLS默认5432/notes/require。组环境可提供公共地址，项目环境提供专用凭据。镜像默认APP_ORIGIN=https://notes.shier.art、COOKIE_SECURE=true，生产不回退SQLite。

## 原生部署（无需Docker）

Linux + systemd原生入口支持amd64/arm64，使用预先准备的root私有配置文件，不再安装或调用ConfigHub。首次安装使用支持新入口的原生Release及--env-file /root/notes.env；已有部署可复用service.env。旧原生Release保持原样，不能将新安装器与旧ConfigHub运行包混用。安装校验SHA256，配置预检后迁移并启动，数据和账户保留。具体配置、版本选择和备份见[原生部署指南](notes-native-deployment.md)。

## Docker 一行部署（可选）

适用于 Linux 服务器；自动安装缺失的 Docker/Git 支持 Ubuntu/Debian。其他发行版先安装 Docker Engine、Compose v2、Git、curl。先将域名 A/AAAA 解析到服务器，并开放 TCP 80/443；确保端口未被其他服务占用。建议至少 2核/4GB，并预留附件和备份磁盘空间。

替换域名和邮箱后执行：

```bash
curl -fsSL https://raw.githubusercontent.com/art-shier/notes/main/install.sh | bash -s -- --domain notes.shier.art --email you@example.com
```

脚本下载项目到当前用户的 `~/notes`，生成独立随机数据库密码，构建并启动 PostgreSQL、Go/Web、Caddy，等待数据库和 HTTPS 就绪，最后输出 **24小时有效的管理员邀请链接**。打开链接自行设置密码，没有默认密码或公开注册。系统依赖安装及 Docker 权限可能要求 sudo。

配置位于 `~/notes/notes-server-go/.env`，权限600；首次邀请备份在同目录 `.bootstrap-invite`，不要公开。注册完成后重跑脚本会删除本机邀请备份。数据保存在 Docker 命名卷；项目名默认 `shiji`，保存在 `.env` 的 COMPOSE_PROJECT_NAME 中。

可在上述命令末尾的 `--email ...` 后追加 `--dir /srv/notes --project shiji`，指定安装位置和独立项目名：

```bash
curl -fsSL https://raw.githubusercontent.com/art-shier/notes/main/install.sh | bash -s -- --domain notes.shier.art --email you@example.com --dir /srv/notes --project shiji
```

重复执行会保留已有配置、密码和数据，也不会自动更新已有 Git 代码。输入域名与已有配置不同会拒绝；已有非本项目目录不会覆盖；失败不会自动删除持久卷。若安装进程被强制终止，确认没有其他安装运行后再移除 `notes-server-go/.install.lock` 目录。

## 已有外部PostgreSQL部署

新的生产安装在ctl配置中提供专用数据库连接，见[ctl平台模式](deploy/CONTROL-PLANE.md)。Notes不创建外部数据库/账号，不自动迁移其他部署的数据。

旧Compose实例的私有.env和持久卷保持有效，ops/start.sh在本地配置上只读预检后启动；旧.config-hub.json不再触发远程读取。ConfigHub相关安装参数、Token选项及deploy/prepare.sh配置生成已停用。需要更改数据库目标时先备份并另行迁移；不要把应用回滚当成数据库回滚。

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
bash ops/check.sh https://notes.shier.art
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

下载时先解析仓库提交，再从同一个提交下载完整 Skill/CLI 文件。重复安装可更新本安装器管理的文件，发现本地修改或同名非本项目命令时会停止。安装后连接：

```text
shiji-notes login
shiji-notes whoami
shiji-notes folders list
```

`login` 默认打开 `https://notes.shier.art` 的授权页面。用户核对授权码、账户、权限和有效期并亲自确认，CLI 自动保存连接到当前用户的 `~/.shiji-notes/client.json`，后续直接使用。Agent 使用 `login --no-browser`，将输出的授权链接在新标签页打开后等待用户确认，不能代替用户批准。`logout` 撤销并清理连接；原来的环境变量和手动 Token 仍兼容。该 Token 属于 Notes 账户，与 ctl 部署凭据、数据库密码不同。

Notes v0.4.0 起在“空间设置 → Agent 接入”提供可完整复制给 Agent 的安装和授权说明，同时公开这些资源（需先升级 Notes 服务）：

- [在线 Skill 文本](https://notes.shier.art/agent/SKILL.md)
- [完整 Skill ZIP](https://notes.shier.art/agent/shiji-notes.zip)
- [Python 安装器](https://notes.shier.art/agent/install-client.py)

自部署实例将以上域名换成自己的服务地址。下载安装器后运行 `python install-client.py --server https://你的域名`，会下载同版完整包并核对 `/api/v1/agent-access` 提供的 SHA256；支持自定义 skills 目录，不覆盖本地修改。没有部署新版实例时，也可从 [GitHub Skill 文本](https://raw.githubusercontent.com/art-shier/notes/main/notes-skill/shiji-notes/SKILL.md) 阅读安装说明。安装不会连接数据库或创建笔记账号。

- [Go 服务端运行说明](notes-server-go/README.md)：数据库迁移、CLI、测试、环境变量。
- [Web 前端](notes-web/README.md)：Vite 默认5173，API默认8000。
- [Agent Skill](notes-skill/shiji-notes/SKILL.md)：Python标准库客户端，服务端运行不依赖 Python。
- [技术方案](notes-technical-plan.md)与[Go迁移记录](notes-go-migration-plan.md)。

一键脚本有模拟命令回归；[CI](https://github.com/art-shier/notes/actions) 验证 Go 测试/竞态/vet、Web 测试/构建、安装脚本，以及真实 systemd 原生 Go/Web、TLS PostgreSQL16、Docker API 和完整备份恢复。[首轮验证已通过](https://github.com/art-shier/notes/actions/runs/37112169790)。域名证书、服务器环境和生产容量仍需在目标服务器验收；HTTPS 校验不会跳过 TLS 证书检查。

私有仓库副本可通过API下载脚本并使用临时Token克隆（Token只用于认证，不写入Git远程地址或配置）：

```bash
read -rsp 'GitHub Token: ' GITHUB_TOKEN; echo; export GITHUB_TOKEN; (set -o pipefail; curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" -H 'Accept: application/vnd.github.raw+json' 'https://api.github.com/repos/art-shier/notes/contents/install.sh?ref=main' | bash -s -- --domain notes.shier.art --email you@example.com); unset GITHUB_TOKEN
```
