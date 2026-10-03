# 部署脚本测试

`python3 tests/deploy_test.py` 使用临时目录和模拟 Git/Docker/curl 验证安装控制流程，不连接真实服务器，不安装系统依赖、不删除真实 Docker 数据。生产部署无需 Python。

测试覆盖首次安装、密码生成、配置保留、邀请/已有账户、域名不匹配、非法参数、构建/TLS失败和非仓库目录保护。HTTPS证书、Docker运行及PostgreSQL需另做实际验收。
