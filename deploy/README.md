# Notes Team Deploy

application 为 `notes`；HTTP 端口8000，就绪接口 `/api/v1/health/ready`。复用 Go/Web Dockerfile、外部 PostgreSQL和现有HTTPS反向代理。主机默认只绑定127.0.0.1。

流水线由notes仓库自己的YAML定义，不调用私有reusable workflow。只通过checkout获取 `art-shier/deployctl` v1.2.0 对应的审核提交 `2d3a014a7b5015051a871664c1f4c2cfab2a6566`，用于契约校验和生成标准包。使用项目标签作为version。项目测试在Dockerfile中的Node22/Go1.26测试阶段执行，不依赖Runner默认Go/Node版本。

`deployment.yaml` 已由deployctl1.2.0 init生成并通过validate。必需变量为 `DATABASE_URL`、`APP_ORIGIN`、`COOKIE_SECURE`；密码只写在部署服务器私有配置文件中，不进入Git或发布包。沿用ConfigHub的 `shier/prod`、独立 `notes` 数据库和笔记专用账号。

## 发布与服务器步骤

notes自己的 `.github/workflows/release.yml` 已接入，原生构建改为独立的手动工作流。尚未发布新的标准部署包；需要配置平台只读Token并成功运行构建后才能取得真实部署产物。

用户已决定暂缓图片持久化，后续可能接OSS。当前图片/导出在容器可写层，升级或重建可能丢失；不手改标准Compose绕过契约校验。具体发布、ConfigHub准备、安装与账户步骤见 [OPERATIONS.md](OPERATIONS.md)。

deployctl可以保持私有，notes保持公开。启用时在notes的Actions Secrets配置 `PLATFORM_READ_TOKEN`，仅授权读取deployctl仓库内容。Token用于Runner下载打包工具，不是调用私有共享工作流，也不是数据库或ConfigHub Token；未配置时流水线在下载前明确失败，不输出凭据。

标准流水线使用v*标签或手动触发，version必须是已有且尚未发布的项目标签；不要重复发布v0.1.0。构建完成才使用真实镜像digest生成标准tar.gz及相邻.sha256，四个包内文件由deployctl生成。

启动前执行prepare.sh，保留字段回退、URI编码、固定目标、只读预检和失败保留；生成容器适用的raw env，而不是复用旧的带引号.env或原生宿主机配置。

原生与旧Compose安装入口继续可用。这次准备没有推送标签、发布新版本、执行服务器部署，也未连接生产数据库。没有实际Docker构建或SSH验证。
