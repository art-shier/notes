# Notes 接入 ctl 管理服务

本页适用于当前Notes标准包与ctl平台配置。ctl服务端/管理台的部署见平台仓库 [服务端指南](https://github.com/art-shier/deployctl/blob/main/docs/control-plane.md)。

在管理台注册项目 `notes`，默认环境 `prod`，镜像仓库默认使用 `ctl.shier.art/notes`。管理API与Registry可以共用 `ctl.shier.art`：HTTPS代理将 `/api/v1/` 与 `/v2/` 请求转发给支持Registry网关的ctl API。创建该项目的publisher凭据给GitHub，创建仅可读取prod的deployer凭据给生产服务器。

GitHub仓库设置：

| 设置 | 位置 | 用途 |
|---|---|---|
| `CTL_SERVER_URL` | Variables，可选 | 管理服务HTTPS origin；未配置或为空时使用`https://ctl.shier.art` |
| `CTL_REGISTRY_HOST` | Variables，可选 | 镜像仓库host，不含协议；未配置或为空时使用`ctl.shier.art` |
| `CTL_PUBLISH_TOKEN` | Secrets，必填 | 该项目publisher凭据，无默认值 |

两个地址变量分别独立回退，显式非空值优先；仅设置其中一个时，另一个仍使用默认值。地址必须与ctl实例的public origin、Registry public host及项目登记的镜像仓库一致。服务连接或鉴权失败时停止发布，不自动切换地址或仓库。缺少publisher凭据时明确报错。

流水线使用Notes自己的业务测试，默认推送到ctl托管仓库。检查不可变索引中的 AMD64/ARM64 架构，按各自 manifest digest 运行最终镜像，兼容 Docker classic image store；通过后生成固定索引 digest 的标准包、调用`ctl publish --channel stable`，再提供GitHub Release下载。现有GHCR版本与原发布包安装方式保持可用，平台源码固定审核SHA。

发布中断重跑会先取回ctl已经登记的同版本原始包/digest，并核对源码commit；有已完成的ctl版本时跳过镜像重建/再次推送，保留原stable指针。GitHub Release资产已存在时校验完全相同的字节，只上传缺失资产；不同内容拒绝覆盖。

prod环境配置完整PostgreSQL `DATABASE_URL`，或提供`DB_HOST`、`DB_USER`与秘密`DB_PASSWORD`。可选`DB_PORT`默认5432、`DB_NAME`默认notes、`DB_SSLMODE`默认require（也支持verify-ca/verify-full）。可以在组环境配置公共主机/端口/TLS，在notes项目环境设置专用账号、密码及库名。Go只在DATABASE_URL缺失时安全拼接并编码DB_*；已有DATABASE_URL优先，改用字段时先核对最终配置中是否仍有该URL。默认`APP_ORIGIN=https://notes.shier.art`、`COOKIE_SECURE=true`来自镜像，自定义域名设置`APP_ORIGIN`，端口使用`--port`。

安装参数可设置`ADMIN_EMAIL`：post仅在数据库没有账户/邀请时创建管理员邀请。邀请内容保存在ctl的私有hook日志。已有账户/邀请不会重复初始化。

```bash
sudo ctl login --server https://ctl.shier.art
sudo ctl install notes --prod
sudo ctl upgrade notes --prod
sudo ctl status notes --prod
```

pre仅消费ctl私有最终快照effective.env，用已拉取的固定digest镜像执行只读database-check；不生成或改写配置，refresh_config关闭。Notes不再读取ConfigHub，服务器无需其CLI或Token。pre/post辅助容器使用`--pull never`。字段缺失/无效、数据库不存在或不可连接时阻止替换服务，生产不回退SQLite；检查错误不输出秘密值。

管理台配置优先于CLI同名变量/参数。保存不立即重启，下一次安装/升级生效；失败恢复实际旧版本、配置和绑定。数据库/邀请副作用及图片/导出持久化保持此前边界，后续OSS接入单独实现。
