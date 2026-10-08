# 原生部署：systemd + Go/Web + PostgreSQL

推荐的生产入口是[ctl标准部署](deploy/CONTROL-PLANE.md)。原生入口用于Linux + systemd，无需Docker；当前源码不再安装或调用ConfigHub。

首次安装前准备root所有、权限600的私有raw key=value文件，例如/root/notes.env，包含完整PostgreSQL DATABASE_URL。用户名/密码特殊字符必须URL编码。也支持DB_HOST/DB_USER/DB_PASSWORD以及DB_PORT/DB_NAME/DB_SSLMODE，默认5432/notes/require；包含空白或引号的特殊凭据推荐使用编码后的DATABASE_URL，避免systemd EnvironmentFile的引号/空白规则影响原始密码。外部库及Notes专用账号预先创建，不自动迁移其他库。

```bash
curl -fsSL https://raw.githubusercontent.com/art-shier/notes/main/install-native.sh | sudo bash -s -- --domain notes.example.com --email you@example.com --env-file /root/notes.env
```

先确认所选原生Release包含新入口；旧v0.1.0运行包保持原样，升级脚本不会重写历史资产。安装校验SHA256，创建专用非登录OS账号，只读预检后发布私有service.env，迁移并启动systemd，最后输出首次管理员邀请。已有部署可省略--env-file，保留原service.env；旧native.json的ConfigHub字段被忽略，不需要原CLI/Token。重复指定--env-file是显式更新配置，数据库目标改变需先另行备份及迁移。

默认程序/配置/数据目录为/opt/shiji、/etc/shiji、/var/lib/shiji，监听127.0.0.1:8000。程序、配置和单元路径及祖先必须root所有，禁止组/其他用户写入，禁止符号链接；不支持/tmp或普通用户home。原生数据库配置文件必须root所有且私有，输入只允许已知字段，始终禁止生产SQLite回退。

HTTPS由现有Nginx/Caddy提供；安装器生成私有代理片段，不修改其他站点或签发证书。完成域名解析、证书与代理后再打开邀请链接。

```bash
sudo systemctl status shiji.service
sudo journalctl -u shiji.service -u shiji-config.service --no-pager -n 100
# 检查已有本地配置后重启；检查失败保留运行中的服务
sudo bash /opt/shiji/current/ops/native/start.sh
sudo systemctl restart shiji.service
sudo bash /opt/shiji/current/ops/native/admin.sh invite --admin-email you@example.com --email member@example.com
```

开机时shiji-config.service仅检查私有本地配置与数据库，不获取远程配置。升级前完整备份，再选择含原生运行包的版本重跑安装命令。新程序启动失败时恢复旧程序、配置及单元；数据库迁移不自动回滚。锁目录只能在确认没有安装/预检运行后移除。

## 备份与恢复

安装匹配PostgreSQL16的pg_dump/pg_restore，停止该库所有写入者后操作：

```bash
sudo systemctl stop shiji.service
sudo bash /opt/shiji/current/ops/native/admin.sh backup-create --output /srv/shiji-backups/新目录 --app-stopped
sudo bash /opt/shiji/current/ops/native/admin.sh backup-verify --backup /srv/shiji-backups/新目录
sudo systemctl start shiji.service
```

恢复要求独立完全空库及空附件目录，不能先运行会迁移并创建邀请的安装器。使用独立私有暂存配置明确指向新数据库和新附件/导出目录，再执行backup-restore --backup /private/snapshot --app-stopped。验收前不要切换正式连接或公网代理；恢复保留密码及Agent Token，清除会话/临时导出。不自动复制附件、停止其他实例或执行数据库迁移。
