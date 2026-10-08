# 验证入口

Go模块运行go test ./...、go vet ./...；config测试覆盖DATABASE_URL优先、DB_*默认/覆盖、IPv6/凭据编码、生产禁止SQLite与不泄露值的字段错误。

Linux root下运行以下脚本，配置安全用ext4临时目录，Docker/特权边界采用fixture：
- tests/team_config_test.py：隔离运行真实打包pre，确认最终effective.env原样传给已拉取固定digest镜像（--pull never）、没有ConfigHub或配置重写、数据库失败不暴露秘密，拒绝公开/链接文件。
- tests/team_hook_test.py：以上协议、可重现生成及旧prepare入口停用。
- tests/native_config_test.py：私有--env-file、DB字段、已有配置复用、环境隔离、预检失败保留原配置；旧metadata中的CLI/Token不访问。
- tests/config_hub_start_test.py：保留历史文件名，验证旧私有.env本地预检/启动、旧ConfigHub metadata忽略、失败与并发/权限保护。
- tests/deploy_test.py、tests/native_install_test.py：安装边界、原数据/配置保留、SHA/archive/path检查及旧ConfigHub参数拒绝。
- tests/team_package_test.py 平台源码目录：真实标准包回读，required_config为空、refresh_config关闭、pre/post保持不可变。
- tests/release_delivery_test.py 平台源码目录：同版本发布中断恢复。
- tests/client_install_test.py：CLI/Skill安装兼容。

GitHub CI另执行Docker/systemd集成：team_smoke使用真实ctl、标准hooks、DB_*及TLS PostgreSQL16，覆盖override/回滚/预检/post失败与邀请幂等；startup_smoke验证外部TLS数据库故障不替换运行容器，并用真实默认Compose图验证首次内置库启动、停止db/app后的恢复及.env保留；native_smoke使用新私有env原生包、非root服务及TLS数据库，覆盖安装/重复安装/权限/失败升级/启动检查。pg_smoke保持API及真实备份恢复测试。

这些测试不访问生产ConfigHub或数据库，只清理各自随机命名fixture。
