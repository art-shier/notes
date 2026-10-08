# Notes 接入 ctl 管理服务

本页适用于 Notes [v0.3.2](https://github.com/art-shier/notes/releases/tag/v0.3.2) 与 ctl>=1.7.0。ctl服务端/管理台的部署见平台仓库 [服务端指南](https://github.com/art-shier/deployctl/blob/main/docs/control-plane.md)。

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
