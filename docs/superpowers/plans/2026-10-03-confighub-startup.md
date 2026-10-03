# ConfigHub 启动配置实施计划

> **For agentic workers:** Use superpowers:executing-plans inline. Review the whole change after verification.

**Goal:** 启动前使用 ConfigHub CLI 读取指定项目/环境，生成私有配置并启动使用外部 PostgreSQL 的笔记服务。

**Architecture:** 新增外部数据库 Compose 文件和 ops/start.sh。启动脚本只接收 ConfigHub 连接与项目参数，从 CLI 的 JSON 输出选择数据库字段，使用 jq 编码 URL，不执行远端配置文本。先用临时配置构建及只读检查数据库，再原子替换 .env；之后执行 Compose 启动。安装脚本调用同一启动流程，保留原独立数据库方式。

**Tech Stack:** Bash、ConfigHub CLI、jq、Docker Compose、Go/GORM、PostgreSQL16。

## Global constraints / design

- ConfigHub URL 必须 HTTPS，项目/环境显式指定；本次目标 shier/prod。
- 支持 notes_db_address/port/username/password 字段，缺省使用已存在的 db_address/port/username/password。数据库名通过 --database-name 指定，默认 notes；不连接 postgres/template 系统库。
- 全局 CLI 配置或 --token-file 提供 Token；不写入仓库、不放在命令行、不输出数据库凭据。
- 外部数据库必须已存在，且为空库或已迁移的笔记库。启动脚本不创建数据库、不改其他业务库、不自动迁移旧独立部署的数据。
- .env 与临时配置权限600；ConfigHub 拉取/字段校验/数据库检查失败时保留已有 .env 和运行容器。
- 同一目录记住项目/环境等非秘密启动参数；后续 bash ops/start.sh 重新拉取。主机/端口/库名/用户名改变时拒绝自动切换，密码更新可通过只读预检后生效。
- 外部模式启动 app/caddy，不启动 db；备份仍用 app 中的 PG16客户端，恢复检查空目标库而非启动本地 db。

## Review focus

1. 特殊字符、换行与 shell 注入不会执行或进入日志。
2. CLI失败、字段缺失、数据库不可达，不覆盖上次配置、不停现有服务。
3. 已有本地数据库部署或外部目标变化，不自动切库。
4. 并发启动、符号链接和配置文件权限，不破坏配置或暴露秘密。
5. 外部 PG 健康/账户初始化/备份恢复，不依赖不存在的 db 容器。

## Task 1: Go CLI 数据库预检和初始化状态

Files: cmd/shiji/main.go, deployment.go, deployment_test.go。

- [x] 先编写 run(database-check) 对空SQLite/笔记库通过、其他表拒绝；bootstrap-status 覆盖空、有效/过期邀请与已有用户。
- [x] 运行测试观察失败；实现 deploymentCheck(*gorm.DB) error 和 bootstrapState(*gorm.DB) (string,error)，CLI 输出仅状态或一般错误。
- [x] go test ./... 与 vet。

## Task 2: CLI拉取与外部启动

Files: ops/start.sh, compose.external.yaml, tests/config_hub_start_test.py。

- [x] 用临时项目、真实 jq、模拟外部 CLI/Docker 编写行为测试：编码、重复拉取、密码刷新、失败保留、切库/注入/符号链接/并发拒绝。
- [x] 观察失败；实现私有临时文件、元数据、目标指纹、只读预检、原子配置发布及启动。
- [x] 运行完整脚本回归和 Bash 语法检查。

## Task 3: 安装、运维、文档与 CI

Files: install.sh, ops/restore.sh, README.md, notes-deployment-guide.md, tests/deploy_test.py, tests/pg_smoke.py, .github/workflows/ci.yml, .gitignore。

- [x] 先补安装 ConfigHub 参数转发回归；实现缺少 jq/CLI 时安装官方公开CLI，已安装则复用。
- [x] 初始化状态改用 app CLI；恢复支持无本地 db。文档给出含 token-file 的一行部署、后续启动、独立数据库与账号准备要求。
- [x] CI用独立PG16容器验证外部模式、只读预检、账户状态、API与备份恢复；不访问真实生产配置或数据库。
- [ ] 最终独立审查、修复重要问题，提交推送并等待最终 CI。

## Evidence / rulings

- 用户已明确要求启动时通过CLI拉取配置后生成配置再启动；保持连续执行，无需重复询问实现方式。
- 本机对 shier/prod 的只读验证已成功：PostgreSQL16.14、TLS、数据库连接可用。当前该实例无 notes 数据库，因此部署前需要准备独立目标库；本次不向其他业务库写入。
