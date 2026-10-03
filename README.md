# 拾记 Notes

自托管、多用户私有笔记服务，移动端 Web 优先。首页按文件夹聚合，支持图文、标签、搜索、历史、回收站、离线 ZIP 导出。Agent 通过 Skill + REST API 读写，不需要 MCP。

服务端 Go + Gin + GORM；前端 React + TypeScript + Tiptap；生产 PostgreSQL 16，Caddy 提供 HTTPS。

## 一行部署

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

## 常用运维

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
docker compose up -d --wait --wait-timeout 180
bash ops/check.sh https://notes.example.com
```

备份目标目录必须不存在。恢复只允许独立空环境；详情见 [部署与恢复指南](notes-deployment-guide.md)。

## 本地开发与 Agent

- [Go 服务端运行说明](notes-server-go/README.md)：数据库迁移、CLI、测试、环境变量。
- [Web 前端](notes-web/README.md)：Vite 默认5173，API默认8000。
- [Agent Skill](notes-skill/shiji-notes/SKILL.md)：Python标准库客户端，服务端运行不依赖 Python。
- [技术方案](notes-technical-plan.md)与[Go迁移记录](notes-go-migration-plan.md)。

一键脚本有模拟命令回归；[CI](https://github.com/art-shier/notes/actions) 验证 Go 测试/竞态/vet、Web 测试/构建、安装脚本，以及真实 Docker + PostgreSQL 16 的 API 和完整备份恢复。[首轮验证已通过](https://github.com/art-shier/notes/actions/runs/37112169790)。域名证书、服务器环境和生产容量仍需在目标服务器验收；HTTPS 校验不会跳过 TLS 证书检查。

私有仓库副本可通过API下载脚本并使用临时Token克隆（Token只用于认证，不写入Git远程地址或配置）：

```bash
read -rsp 'GitHub Token: ' GITHUB_TOKEN; echo; export GITHUB_TOKEN; (set -o pipefail; curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" -H 'Accept: application/vnd.github.raw+json' 'https://api.github.com/repos/art-shier/notes/contents/install.sh?ref=main' | bash -s -- --domain notes.example.com --email you@example.com); unset GITHUB_TOKEN
```
