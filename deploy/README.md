# Notes 标准部署包

当前包仅消费ctl提供的最终运行配置，不读取ConfigHub，也不生成数据库连接文件。建议使用[ctl平台模式](CONTROL-PLANE.md)安装。

配置完整PostgreSQL `DATABASE_URL`，或配置 `DB_HOST`、`DB_USER`、秘密 `DB_PASSWORD`；可选 `DB_PORT`（5432）、`DB_NAME`（notes）、`DB_SSLMODE`（require）。Go只在DATABASE_URL缺失时拼接字段，安全编码用户名/密码和IPv6地址；生产不回退SQLite。

required_config为空，支持两种配置形式。pre使用ctl的私有effective.env与已拉取的固定digest镜像执行只读database-check，使用--pull never；不改写config.env/secrets.env，无需refresh_config。post保留账户检查与首次管理员邀请行为。

`prepare.sh`和旧ConfigHub参数已停用；已有私有数据库配置及数据保留。发布包生成、发布恢复及图片/OSS边界见[运维指南](OPERATIONS.md)。
