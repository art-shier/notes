# 部署脚本测试

`python3 tests/client_install_test.py` 使用临时目录安装真实 CLI/Skill，验证带空格路径下的命令帮助、重复安装，以及自定义文件、无关命令、下载源缺失和符号链接保护。CI分别在 Linux 和 Windows 执行，并在 Windows 验证 PowerShell入口；不会配置访问Token或修改真实用户的Skill目录。

`python3 tests/config_hub_start_test.py` 使用真实 jq 和临时目录执行启动脚本，模拟 ConfigHub/容器外部边界：特殊字符URL编码、无关秘密不导出、环境变量覆盖隔离、重复拉取、密码更新、失败保留旧配置、目标锁定及并发/原部署保护。Linux需要jq；Windows开发回归使用work中的官方jq工具，生产不依赖Python。

`python3 tests/deploy_test.py` 使用临时目录和模拟 Git/Docker/curl 验证安装控制流程，不连接真实服务器，不安装系统依赖、不删除真实 Docker 数据。生产部署无需 Python。

测试覆盖首次安装、密码生成、配置保留、邀请/已有账户、域名不匹配、非法参数、构建/TLS失败、非仓库目录保护，以及已有 Docker 缺少 Compose 时从 Ubuntu 或 Docker 官方软件源选择 Compose v2 软件包；软件源不可用时会在生成配置前停止。

GitHub Actions 另执行 `python3 tests/pg_smoke.py`：构建生产 Docker 镜像，以 PostgreSQL 16 验证账户、私有图片、标签、版本冲突、幂等创建、历史、ZIP，以及原生数据库备份恢复。第二个项目只启动app，连接第一个项目的PG服务，验证外部数据库模式及空库/其他业务库/笔记库预检、管理员初始化状态。该测试只允许 `CI=true`，使用随机独立项目，结束时只删除自己的测试卷。真实域名的 HTTPS 证书和生产容量需在目标服务器验收。

`python3 tests/startup_smoke.py` 在 GitHub Actions 中使用真实 Docker Compose 和启用 TLS 的 PostgreSQL16，执行完整 `ops/start.sh`：模拟 CLI 返回配置，由脚本生成权限600的 `.env`，启动 app/caddy，验证数据库连接使用 TLS，并确认后续拉取失败保留原配置及运行容器。`pg_smoke.py` 的外部恢复通过真实 `ops/restore.sh` 执行。这些测试不连接生产 ConfigHub 或数据库，只清理自己随机命名的测试项目。
