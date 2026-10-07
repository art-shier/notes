# Team Deploy 接入计划

> 使用 executing-plans 在当前会话实施；重大变更交独立审查。

**Goal:** 按用户指定的Team Deploy skill接入notes标准发布与服务器部署，保证ConfigHub凭据隔离和已有部署不被误替换；图片持久化按用户决定暂缓。

**Architecture:** Go/Web复用现有Dockerfile，notes自己定义完整构建发布YAML，仅下载固定SHA的平台工具生成标准包，deployctl按digest/SHA管理单服务；PostgreSQL和HTTPS入口外置。用户已决定暂缓图片持久化，当前图片/导出为临时容器数据，后续可能接OSS；不扩展平台或伪造卷字段。标准流水线启用，等待真实构建发布。

**Tech Stack:** Go1.26、Node22、Docker Compose、deployctl1.2、ConfigHub。

**Source:** https://github.com/art-shier/deployctl/blob/main/skills/team-deploy/SKILL.md 及随附onboarding/operations；已发布平台v1.2.0提交2d3a014a7b5015051a871664c1f4c2cfab2a6566。

## 已确定约束

- application notes；container/host端口8000、127.0.0.1；readiness /api/v1/health/ready。
- Dockerfile notes-server-go/Dockerfile、context根目录；保留现有生产构建、Go/Web与Agent API。
- required_config声明DATABASE_URL、APP_ORIGIN、COOKIE_SECURE，不保存实际生产值。
- ConfigHub沿用shier/prod；notes库与专用账号不重新创建，不连接生产DB做准备验证。
- 不手写标准包四文件，不虚构digest或包链接；不为绕过平台校验编辑生成的Compose。
- 不推送发布标签、不发布新版本、不操作生产服务器；项目接入与实际发布/部署授权分开。
- 用户明确选择notes自有流水线。notes公开、deployctl私有保持不变，通过PLATFORM_READ_TOKEN checkout平台工具，不调用私有reusable workflow，不需要改变仓库可见性。
- v1协议无volumes。用户明确暂缓持久化，图片/导出在容器重建时可能丢失，文档说明边界，后续再接OSS。

## Task 1: 可审查的本地接入准备

- [x] 阅读用户指定skill、项目Dockerfile/启动/健康接口、平台协议及真实发布引用。
- [x] 使用deployctl init dry-run再生成deployment与workflow草稿，validate退出0。
- [x] 最初workflow保存在deploy目录；随后启用项目自有标准workflow，将原生发布改为手动，避免同名Release创建冲突。
- [x] 增加web-test/server-test阶段及test-release.sh，平台不依赖Runner默认Go/Node版本。
- [x] Go8包测试、Web7项、Bash语法及diff检查通过；本机无Docker，未报告镜像构建通过。
- [x] 独立审查准备配置并补充raw env、容器监听/路径与ConfigHub适配缺口；整理到项目主目录。

## Task 2: 完成受支持的发布与部署路径

- [x] 用户已决定暂缓图片持久化；不扩展平台，记录临时容器数据边界。
- [x] 按用户澄清改为项目自有完整流水线，平台工具checkout固定经审查SHA；两仓库可见性保持不变。
- [x] 接入ConfigHub到私有config.env/secrets.env，在控制器执行前完成配置校验与DB只读预检。
- [x] 整理原生与标准workflow的发布策略，避免Release碰撞和原生installer误选不含native资产的版本。
- [ ] 在Linux CI验证镜像和数据库兼容；获得发布/服务器授权后验证真实发布包SHA/digest与Team Deploy安装升级回滚。附件持久性暂缓。
- [x] 更新README、配置准备入口与运维说明；真实标准发布及服务器部署尚待后续授权和配置。

## 准备验证记录

deployctl init/validate（1.2.0）退出0；Go test ./... -count=1全部8包通过；npm ci/npm test退出0，7项通过；test-release.sh Bash -n退出0。读取Github平台Release/tag/CI元数据已确认引用真实存在；未读取生产secret、调用生产ConfigHub/数据库或执行SSH。

用户澄清后已将草稿改为notes自有完整YAML，actionlint1.7.7与deployctl validate退出0；此前仓库公开/私有方案问题不再需要用户选择。仍未配置平台只读Token、触发真实构建或发布。


本次补齐：标准workflow已放到.github/workflows；原生workflow改手动，原生installer筛选对应架构运行包。prepare.sh从ConfigHub生成raw容器配置并用真实digest镜像预检，失败保留配置。新增回归已通过；尚无PLATFORM_READ_TOKEN、实际标准Release或目标服务器部署。
