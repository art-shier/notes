# 部署脚本测试

`python3 tests/deploy_test.py` 使用临时目录和模拟 Git/Docker/curl 验证安装控制流程，不连接真实服务器，不安装系统依赖、不删除真实 Docker 数据。生产部署无需 Python。

测试覆盖首次安装、密码生成、配置保留、邀请/已有账户、域名不匹配、非法参数、构建/TLS失败、非仓库目录保护，以及已有 Docker 缺少 Compose 时从 Ubuntu 或 Docker 官方软件源选择 Compose v2 软件包；软件源不可用时会在生成配置前停止。

GitHub Actions 另执行 `python3 tests/pg_smoke.py`：构建生产 Docker 镜像，以 PostgreSQL 16 验证账户、私有图片、标签、版本冲突、幂等创建、历史、ZIP，以及原生数据库备份恢复。该测试只允许 `CI=true`，使用随机独立项目，结束时只删除自己的测试卷。真实域名的 HTTPS 证书和生产容量需在目标服务器验收。
