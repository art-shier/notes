# Notes 接入 ctl 管理服务

本分支接入待发布的ctl1.7.0；现有公开Release不变。ctl服务端/管理台的部署见平台仓库 `docs/control-plane.md`。

在管理台注册项目 `notes`，默认环境 `prod`，镜像仓库填托管路径（例如`registry.shier.art/notes`）。创建该项目的publisher凭据给GitHub，创建仅可读取prod的deployer凭据给生产服务器。

GitHub仓库设置：

| 设置 | 位置 | 用途 |
|---|---|---|
| `CTL_SERVER_URL` | Variables | 管理服务HTTPS origin，例如`https://ctl.shier.art` |
| `CTL_REGISTRY_HOST` | Variables | 镜像仓库host，不含协议，例如`registry.shier.art` |
| `CTL_PUBLISH_TOKEN` | Secrets | 该项目publisher凭据 |

两个变量必须同时设置。流水线使用Notes自己的业务测试，推送固定digest镜像、生成标准包，调用`ctl publish --channel stable`后继续提供GitHub Release下载。未设置变量时保持原GHCR方式。平台源码引用固定实际SHA；新工具尚未发布，版本发布前需核对最终审核SHA。

prod环境业务变量至少配置有效的秘密`DATABASE_URL`，形式为包含用户名、密码、地址、端口、数据库名的完整PostgreSQL URL。密码特殊字符进行URL编码。默认`APP_ORIGIN=https://notes.shier.art`、`COOKIE_SECURE=true`来自Notes镜像，需要更改域名时在管理台设置`APP_ORIGIN`。端口在部署默认值或安装命令的`--port`中指定。

安装参数可设置`ADMIN_EMAIL`：post仅在数据库没有账户/邀请时创建管理员邀请。邀请内容保存在ctl的私有hook日志。已有账户/邀请不会重复初始化。

```bash
sudo ctl login --server https://ctl.shier.art
sudo ctl install notes --prod
sudo ctl upgrade notes --prod
sudo ctl status notes --prod
```

有有效`DATABASE_URL`时pre直接使用最终快照做只读数据库检查，不要求ConfigHub CLI或Token，也不改写本地生成文件。没有连接时保留原ConfigHub生成和回读路径。ctl已经拉取受检镜像，pre/post辅助容器使用`--pull never`，不接触私有Registry凭据。数据库不存在或不可连接时阻止替换服务，生产不回退SQLite。

管理台配置优先于CLI同名变量/参数。保存不立即重启，下一次安装/升级生效；失败恢复实际旧版本、配置和绑定。数据库/邀请副作用及图片/导出持久化保持此前边界，后续OSS接入单独实现。
