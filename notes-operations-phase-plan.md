> 历史实施记录：当前服务端已转为 Go，执行入口与迁移范围见 notes-server-go/README.md 和 notes-go-migration-plan.md。以下 Python 路径保留用于追溯旧版。

# 拾记部署与整库恢复实施计划

> 执行方式：使用 superpowers:executing-plans 在当前工作区实施，测试先行，最后独立审查。没有 Git 仓库，不创建无关仓库或分支。

目标：提供自有服务器的部署检查、完整备份和恢复操作，并在隔离环境证明账户、笔记历史、附件及 Agent Token 可以恢复。保留既定移动端样式与 Skill + REST API 架构。

## 设计合约

- 备份是管理员离线操作，不新增 Web/Agent 管理接口。操作前停止所有应用实例；数据库服务保留运行。SQLite 额外取得写锁覆盖数据库快照与图片复制，PostgreSQL 使用 pg_dump custom 格式。JSON/ZIP 用户导出不能代替该备份。
- 备份目录包含数据库、完整附件目录、版本/数量元数据及逐文件大小/SHA256清单；包含未引用附件与历史图片。会话、Token和密码哈希在整库中，备份目录权限必须私有；不打印数据库密码或密钥。不保存 .env、私钥或短期导出文件。
- 新备份目录必须不存在，使用私有临时目录；独占创建目标目录，完整清单最后发布作为就绪标记。文件缺失、大小不一致、符号链接或路径逃逸都失败。备份只支持当前Alembic head，记录数据库类型。
- 校验检查清单覆盖、大小、哈希与安全路径，拒绝多余或缺失文件。恢复只能到新SQLite文件或空PostgreSQL数据库及空附件目录，不覆盖现有数据，不跨数据库类型转换。失败不自动删除用户数据库。
- 恢复清除旧网页会话与临时导出任务，保留账户、密码哈希、Token、笔记、标签、版本和图片。需重新登录。恢复后检查数据库完整性、附件及数量，再启动应用；先在独立目录/Compose项目演练。
- 首期手动备份，不创建未经请求的定时任务；提供异机保存和升级前备份方法。正式服务器信息未提供前，远程部署/HTTPS验收不能标为完成。

## 任务

### 1. 整库备份与恢复工具

文件：notes-server/app/backup.py、backup_files.py、tests/test_backup.py。

接口：create_backup(settings, output, app_stopped=False)、verify_backup(path)、restore_backup(settings, backup, app_stopped=False)；CLI `python -m app.backup create|verify|restore`，数据库与附件路径来自环境配置。

- [x] 失败测试：拒绝在线声明缺失/覆盖；真实含图历史库备份和恢复；哈希损坏/路径逃逸/缺图拒绝；恢复后旧会话失效、Token保留、导出任务清空。另有实际双用户恢复演练；符号链接用例因本机权限跳过，需Linux执行。
- [x] 实现私有整库备份与新环境恢复，PostgreSQL进程参数不含密码、事务恢复；缺失工具时明确失败。实际PG执行仍待目标环境。
- [x] 最终完整后端套件和配置检查。

### 2. 部署操作与验收

文件：notes-server/ops/backup.sh、restore.sh、check.sh、Dockerfile、README.md；notes-deployment-guide.md。

- [x] 提供Compose停写备份/恢复脚本、HTTPS/健康就绪检查与重启持久化操作；恢复必须使用独立空环境。
- [x] Bash语法检查、实际本地恢复演练通过登录、图文/历史、双用户隔离、Agent读取与写入；PostgreSQL/Docker/HTTPS未实际执行。
- [x] 独立审查修复客户端版本兼容性；最终状态/主库检查待收尾。

## 当前边界

本机无Docker、pg_dump/pg_restore，无正式服务器连接信息。先完成可执行工具、配置和SQLite恢复演练；远程验收依赖用户提供SSH/域名与部署环境。服务器已有数据不得在缺乏证据时直接替换。


## 执行记录与验证（2026-10-03）

- 完整后端/CLI套件72项通过、2项跳过；备份专项11项通过、2项跳过。跳过项是实体符号链接源附件/备份文件拒绝，本Windows账号没有创建symlink权限，已明确要求在Linux正式环境执行；不能声称本机验证了这两项。已有Starlette/httpx与Alembic配置弃用警告不影响当前通过结果。
- 先运行失败测试确认备份模块缺失；实现后通过真实图文历史恢复、会话失效/Token保留、导出清空、缺图失败、拒绝覆盖和清单损坏/缺失/多余/路径逃逸。密码在URI用户信息和query两种形式均经过失败到通过的命令参数保护测试。
- 新WAL回归实际保持未checkpoint写入：源.db哈希未变、WAL含更新，完整备份仍包含新数据，输出不依赖-wal/-shm文件。SQLite还检查integrity_check与foreign_key_check。
- 实际迁移的双用户源库通过3个独立CLI进程create、verify、restore；新目录启动隔离API8002/Web5174，真实浏览器验证重新登录、图片、标签、收藏、历史预览/恢复、双用户404、原Token读取并继续写入。恢复库再次停机/启动后版本4及图片保留，源库仍为版本2，备份逐文件哈希仍一致。恢复截图已目视检查。
- 独立审查确认默认Debian trixie PG17客户端会发送PG16不支持的transaction_timeout，造成恢复失败。Dockerfile改为bookworm+官方PGDG postgresql-client-16，与Compose PG16匹配；官方key URL可访问，配置检查通过。真实Docker构建与PG恢复仍未执行。
- Bash三个脚本语法通过；Compose YAML、客户端/服务端大版本匹配、私有持久卷配置通过；Alembic schema check无新增操作。未修改前端代码，不重复运行无关前端构建。
- 已停止本轮隔离API/Web，删除原始账号/Token凭证JSON。主Web5173/API8000健康检查仍为ready，主库无合成测试账号。

### 实施决策

- 不安装Docker或改动现有服务端，不猜测服务器地址。用户连接信息尚未提供，正式服务器/HTTPS验收保持未完成；此轮交付本地已验证工具和可执行操作指南。
- 文件校验、复制与清单发布放在backup_files.py；备份目标独占创建，manifest最后发布作为完成标记；SQLite数据库独占硬链接发布，避免目标在准备期间出现时被覆盖。要求支持本地硬链接的SQLite文件系统；失败会明确返回，不覆盖目标。
- 恢复失败可以留下独立新环境数据，按指南保持停机并检查；不自动DROP数据库或删除用户原库。恢复会话失效，Token保留。
- 备份需要操作者声明全部实例停写。Compose脚本只管理当前项目本机容器，多机或项目外写入者须另外停止。不创建定时备份，不增加管理API，不开启公开注册。

## 仍需目标服务器完成

- [ ] 用户提供系统/Compose状态、真实域名及SSH主机、端口、用户名、已有密钥路径或配置别名。
- [ ] Docker构建、PostgreSQL16真实备份/独立恢复和Linux符号链接回归。
- [ ] 域名DNS、HTTPS、重启持久化、真实两账户/Agent验收与异机备份保存。
