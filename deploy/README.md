# Notes Team Deploy

application为notes，主机默认127.0.0.1:8000，就绪接口 `/api/v1/health/ready`。Go/Web镜像、外部PostgreSQL及HTTPS代理。

新版required_config为空，标准包自带pre/post脚本。pre从ConfigHub的shier/prod读取notes专用数据库账号，生成私有DATABASE_URL，采用默认域名notes.shier.art；ctl在pre后重新加载配置并创建新的不可变快照，再启动。post检查管理员状态，可通过ADMIN_EMAIL生成首个邀请。无需clone或提前执行prepare。

服务器预装ConfigHub CLI，pre直接执行 `confighub --server https://config.shier.art export --project shier --env prod --format json`，使用运行安装命令的用户已有CLI登录配置，不默认指定或检查额外Token文件。ConfigHub失败时保留其原始stderr和退出码，错误记录在受保护的hook日志中。可选TOKEN_FILE直接传给ConfigHub，由CLI处理认证；DOMAIN等通过ctl的 `--set` 指定。安装参数仅本次有效，自定义值每次upgrade需重传。密码不进入Git或发布包。

需要待发布的ctl>=1.6.0及新Notes包。两个工作流固定到相同平台源码SHA；已发布v0.2.0和ctl1.5.0不能使用此新流程。真实旧版本信息与新步骤见 [OPERATIONS.md](OPERATIONS.md)。不覆盖旧Release或使用浮动镜像。

`deploy/hooks/pre-install.sh` 由 `python3 deploy/build-hooks.py` 从prepare与共享安全函数生成；CI/release用 `--check` 检查一致性。钩子打包后不依赖源码目录。测试包含临时TLS PostgreSQL、真实ctl/Docker及失败恢复。

图片与导出持久化按用户要求暂缓；升级或重建可能丢失，后续接OSS。HTTPS/DNS在服务器另外配置。原生与旧Compose入口保留。公开平台源码不需要PLATFORM_READ_TOKEN，GHCR镜像需要Public或服务器登录。
