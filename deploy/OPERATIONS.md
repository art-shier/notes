# Notes：ctl部署与运维

推荐[ctl平台模式](CONTROL-PLANE.md)。项目notes，默认环境prod，绑定127.0.0.1:8000，就绪接口 `/api/v1/health/ready`。PostgreSQL及HTTPS代理外置。

## 数据库配置

在ctl配置中选择一种形式：

| 字段 | 用途 |
|---|---|
| DATABASE_URL | 完整PostgreSQL URL；存在时优先于全部DB_* |
| DB_HOST | 实例主机名、IPv4或IPv6 |
| DB_PORT | 默认5432 |
| DB_USER | Notes专用账号，必填 |
| DB_PASSWORD | 专用密码，秘密字段，必填 |
| DB_NAME | 默认notes |
| DB_SSLMODE | 默认require，也支持verify-ca、verify-full |

可以在ctl组环境中共享DB_HOST/DB_PORT/DB_SSLMODE，在notes项目环境中覆盖DB_USER/DB_PASSWORD/DB_NAME。Go读取ctl最终快照，只有DATABASE_URL缺失时才拼接并URI编码。缺少字段或字段无效时只报告字段名，生产不回退SQLite。独立数据库及账号必须预先存在；安装不会创建数据库或自动迁移其他部署的数据。

旧私有config.env/secrets.env不会被hook改写。已有DATABASE_URL继续有效；改用DB_*时应先核对最终配置中是否仍有DATABASE_URL。数据库切换需另行备份及迁移，不能将应用回滚视为数据库回滚。

## 安装与升级

```bash
sudo ctl login --server https://ctl.shier.art
sudo ctl install notes --prod
sudo ctl upgrade notes --prod
sudo ctl status notes --prod
```

自定义端口使用--port；域名在ctl设置APP_ORIGIN=https://你的域名。镜像默认APP_ORIGIN=https://notes.shier.art、COOKIE_SECURE=true。首次邀请可加--set ADMIN_EMAIL=you@example.com。post仅在账户状态empty时生成邀请；已有邀请或账户保持现状。邀请记录在ctl私有hook日志。

执行顺序：ctl获取配置和不可变镜像 → pre用最终effective.env执行只读database-check → Go迁移/启动/就绪 → post账户检查 → 提交成功。pre/post辅助容器使用已拉取digest及--pull never，数据库检查失败阻止替换旧服务。pre不生成文件，refresh_config关闭；服务器无需ConfigHub CLI或Token。

启动/就绪/post失败恢复旧成功版本与快照；restart复用成功快照，upgrade重新获取ctl配置。回滚不撤销数据库迁移或邀请。

## 发布与旧入口

Notes自己的release.yml测试并构建AMD64/ARM64镜像，发布到ctl托管Registry并登记stable。GitHub变量CTL_SERVER_URL/CTL_REGISTRY_HOST可覆盖默认地址，必需秘密CTL_PUBLISH_TOKEN；版本包和SHA256仍提供GitHub Release下载。发布中断恢复原始同版本包，不覆盖不同资产。

已发布的v0.2.0–v0.3.3包内容保持原样。当前源码移除了ConfigHub回退；deploy/prepare.sh只给出迁移到ctl配置的提示，不写文件。旧ConfigHub安装参数不再接受。独立Compose的既有私有.env和原生service.env仍可本地预检/启动，不读取旧ConfigHub metadata中的CLI或Token。

## 当前图片边界

用户决定暂缓持久卷/OSS。托管包图片与导出暂存容器可写层，升级/重建/回滚可能丢失；数据库记录不含原图片文件。保留重要图片的原部署备份，OSS接入单独实施。
