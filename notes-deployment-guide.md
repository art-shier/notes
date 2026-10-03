# 拾记：自有服务器部署、备份与恢复

本轮已验证SQLite整库恢复，尚未连接正式服务器。Docker镜像、PostgreSQL、DNS和HTTPS仍需在目标服务器执行下述验收，不能用本地结果代替。

首次安装可直接使用仓库根目录 [一行部署说明](README.md) 的 install.sh：自动下载、配置和启动。以下保留手动部署与备份恢复步骤。

## 1. 服务器准备

建议Linux服务器，安装Docker Engine与Compose v2、curl。域名A/AAAA记录指向服务器，开放80/443；只由Caddy接收公网请求。数据库不映射公网端口。项目目录中保留notes-server-go与notes-web同级结构，Docker构建上下文为它们的父目录。

在notes-server-go复制.env.example为.env，填写DOMAIN和至少64字符的随机十六进制POSTGRES_PASSWORD，然后 `chmod 600 .env`。保留NOTE_HISTORY_LIMIT=200、EXPORT_LIMIT_BYTES=2147483648或根据容量调整。不要将.env、数据库、备份包或私钥放入版本库。首次部署保持邀请注册。

从notes-server-go目录执行：

```sh
docker compose config --quiet
docker compose build app
docker compose up -d
docker compose ps
bash ops/check.sh https://你的域名
docker compose exec app shiji bootstrap --email 你的邮箱
```

bootstrap只为新库生成一次性邀请，不创建默认密码。通过链接自行设置密码；链接不要公开。已有数据库请先备份，不重复初始化。若已有管理员但尚无网页用户，从原邀请注册；后续邀请使用invite命令。

应用启动前自动执行 `shiji migrate`：创建新库或验证当前兼容 schema，未知旧版本先通过原实现升级。database、attachments、exports为独立持久卷；Caddy有证书/配置卷。不要执行 `docker compose down -v`。备份工具在镜像内使用pg_dump/pg_restore；应用镜像固定Debian bookworm及PGDG PostgreSQL 16客户端，与Compose的PostgreSQL 16匹配。实际部署确认 `docker compose exec app pg_dump --version` 与数据库均为16；不要用17客户端恢复到16服务端。

## 2. 正式验收

- HTTPS证书有效，HTTP自动跳转HTTPS；`ops/check.sh`验证HTTPS的存活/就绪JSON与Web入口，未使用跳过证书校验参数。
- 自行创建管理员与第二个邀请用户，账号之间不能读取对方笔记、历史或图片；Cookie写请求有CSRF保护。
- 新建图文笔记，修改标题/正文，刷新后读取一致；历史预览及确认恢复成功，恢复后继续保存。
- 创建只读与读写Agent Token，通过正式Skill读取、写入、历史及图文导出；只读不能写，撤销后立即401。
- 导出图文ZIP并解压离线阅读；重启应用后笔记、历史、图片以及未过期已就绪导出仍可读取。
- 检查 `docker compose logs --tail 100 app caddy` 中没有密码、完整Token或正文。服务器磁盘有数据库、附件、历史和备份的余量。

首次稳定运行后才记录验收通过。当前还没有登录/上传限流、审计、密码重置和附件自动回收，公开注册是另一个阶段，不在本轮开启。

## 3. 完整服务器备份

用户图文ZIP只包含当前用户内容。完整服务器备份还包含全部账号、密码哈希、API Token哈希、邀请和数据库关系。备份必须与数据库一样私有，并保存到另一台机器或加密存储；SHA256用于完整性检查，不是来源认证。

备份需要暂停所有应用实例的写入。Compose脚本会停止该项目的所有app实例，备份结束或失败后只重启原先运行的容器；数据库继续运行。此流程有短暂停机。备份前暂停该项目之外的任何写入者；若有多台应用服务器，也需先停止它们。

```sh
sudo bash ops/backup.sh /srv/shiji-backups/20261003-120000
bash ops/check.sh https://你的域名
```

指定目录必须不存在，父目录提前创建。结果在 `/srv/shiji-backups/20261003-120000/snapshot`，包含database.dump、attachments和manifest.json。文件目录私有，所有当前、历史和未引用图片一起保存。短期exports文件不备份，恢复后可重新生成。

备份工具拒绝缺失/不完整图片、符号链接、未知文件路径或不匹配的数据库迁移版本。失败不会发布有效manifest，不删除已有备份。实际服务器应将首次备份复制到独立环境恢复一次，再依赖该备份流程；本轮没有创建自动定时任务。

直接使用工具时：

```sh
shiji backup-create --output /private/new-snapshot --app-stopped
shiji backup-verify --backup /private/new-snapshot
```

DATABASE_URL和ATTACHMENTS_DIR来自运行环境，密码不会作为pg_dump/pg_restore命令参数。`--app-stopped`是操作者对停写状态的声明，不会自行发现或停止所有远程应用。SQLite额外取得写锁并通过独立连接的 `VACUUM INTO` 合并WAL，输出DELETE journal数据库；不能只复制正在运行的.db文件。

## 4. 独立环境恢复演练

使用与备份Alembic版本一致的应用代码。工具只接受同数据库类型：SQLite→SQLite、PostgreSQL→PostgreSQL；不做SQLite→PostgreSQL迁移。校验清单、大小、SHA256、路径，恢复只允许新SQLite文件或空PostgreSQL数据库、空附件目录，不覆盖已有库。

在独立服务器或独立项目副本配置独立测试域名，避免与生产80/443冲突。复制备份后确保容器用户10001可读取备份；若备份由root接收，针对这一份可信备份执行 `sudo chown -R 10001:10001 /srv/shiji-backups/20261003-120000`，保持目录700、文件600。

```sh
# 在独立项目副本的notes-server-go内；此名称也决定新的持久卷前缀。
export COMPOSE_PROJECT_NAME=shiji-recovery
docker compose build app
sudo env COMPOSE_PROJECT_NAME=shiji-recovery bash ops/restore.sh /srv/shiji-backups/20261003-120000/snapshot
# 恢复后确认.env中的测试域名/端口，再启动。
docker compose up -d app caddy
bash ops/check.sh https://测试域名
```

restore.sh要求明确的COMPOSE_PROJECT_NAME，并拒绝已有app容器；数据库和附件是否为空还由Go工具再次检查。脚本不会自动启动应用或Caddy，不会删除已有数据库。失败的新环境可能留有恢复数据或部分附件，保持停机，检查后丢弃该独立环境或人工修复，不要把失败恢复当作可用生产库。

恢复保留账户、密码、API Token、文件夹、标签、笔记及历史；清除旧网页会话和临时导出任务。重新登录，然后完成第2节的图文、历史、用户隔离和Agent验收，核对manifest数量。备份不可变，恢复后的修改不应影响源库和备份。

本地SQLite恢复示例（先停止目标应用；路径必须是新库/空目录）：

```sh
export DATABASE_URL=sqlite:////srv/shiji-recovery/notes.db
export ATTACHMENTS_DIR=/srv/shiji-recovery/attachments
export APP_ORIGIN=http://127.0.0.1:5173
shiji backup-restore --backup /private/sqlite-snapshot --app-stopped
shiji serve
```

## 5. 升级与回退

先完整备份并校验，记录当前应用代码/镜像版本，再构建和启动升级版本。确认迁移、健康检查、登录/图片/历史均通过。数据库迁移不一定支持安全降级；回退应在独立空环境使用旧代码恢复升级前备份，验证后再切换域名或代理。不要把旧备份覆盖到正在运行的服务器。

## 6. 本轮证据

Go迁移验证记录见 [Go迁移实施计划](notes-go-migration-plan.md)，旧阶段记录保留在 [部署与恢复阶段方案](notes-operations-phase-plan.md)。本地使用真实迁移的隔离库，通过独立Go CLI进程备份/校验/恢复，再启动恢复库API；旧密码、Token、图文与历史均可继续使用。主预览已切换Go，未创建测试账号。

[Linux CI 已通过](https://github.com/art-shier/notes/actions/runs/37112169790)：生产 Docker 镜像构建、真实 PostgreSQL16 的图文/标签/版本/历史/导出，以及原生数据库备份恢复。恢复后的密码、Token和图片保留，旧网页会话失效。自动安装系统依赖的控制流程使用模拟命令回归；真实域名证书、服务器权限/端口和容量仍需在目标服务器验收。
