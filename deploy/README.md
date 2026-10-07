# Team Deploy 接入准备

application 为 `notes`；HTTP 端口8000，就绪接口 `/api/v1/health/ready`。复用 Go/Web Dockerfile、外部 PostgreSQL和现有HTTPS反向代理。主机默认只绑定127.0.0.1。

流水线由notes仓库自己的YAML定义，不调用私有reusable workflow。只通过checkout获取 `art-shier/deployctl` v1.2.0 对应的审核提交 `2d3a014a7b5015051a871664c1f4c2cfab2a6566`，用于契约校验和生成标准包。使用项目标签作为version。项目测试在Dockerfile中的Node22/Go1.26测试阶段执行，不依赖Runner默认Go/Node版本。

`deployment.yaml` 已由deployctl1.2.0 init生成并通过validate。必需变量为 `DATABASE_URL`、`APP_ORIGIN`、`COOKIE_SECURE`；密码只写在部署服务器私有配置文件中，不进入Git或发布包。沿用ConfigHub的 `shier/prod`、独立 `notes` 数据库和笔记专用账号。

## 尚未启用发布

`release-team-deploy.yml` 暂存在本目录，未放入 `.github/workflows`，避免与原生 `v*` 发布冲突，也避免发布一个无法安全保存图片的服务。

当前平台协议没有持久卷字段，生成的单服务compose也没有volume；笔记图片和导出文件依赖 `/data/attachments`、`/data/exports`。容器重建不能依赖可写层保存文件。需要先扩展平台持久卷契约，或将附件/导出存储外置，才能启用标准发布。不能手改生成的compose规避校验。

deployctl可以保持私有，notes保持公开。启用时在notes的Actions Secrets配置 `PLATFORM_READ_TOKEN`，仅授权读取deployctl仓库内容。Token用于Runner下载打包工具，不是调用私有共享工作流，也不是数据库或ConfigHub Token；未配置时流水线在下载前明确失败，不输出凭据。

草稿使用手动触发，version必须是已有且尚未发布的项目标签。原生流水线也会自动响应v*标签，因此正式切换前需要协调标签/发布入口；不要将已发布v0.1.0用于重复创建标准Release。构建完成才使用真实镜像digest生成标准tar.gz及相邻.sha256，四个包内文件由deployctl生成。

启动配置也需适配：平台仅消费服务器 `/etc/deployctl/notes/prod/config.env`、`secrets.env`，不会自动从ConfigHub拉取。应先拉取并保留专用字段/共享字段回退、URI编码、固定数据库目标、只读 `database-check` 和失败保留原配置，再交给deployctl安装/升级。raw env_file必须为不带shell引号的 `KEY=value`，secrets.env权限600；不能直接复用旧Compose带引号的.env或原生service.env，后者含127.0.0.1监听与宿主机路径。容器监听应为 `0.0.0.0:8000`，APP_ORIGIN使用HTTPS域名，COOKIE_SECURE=true，附件/导出路径与最终持久存储契约一致。

原生与旧Compose安装入口继续可用。这次准备没有推送标签、发布新版本、执行服务器部署，也未连接生产数据库。没有实际Docker构建或SSH验证。
