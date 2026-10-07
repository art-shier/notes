# Team Deploy 接入计划

> 使用 executing-plans 在当前会话实施；重大变更交独立审查。

**Goal:** 按用户指定的Team Deploy skill接入notes标准发布与服务器部署，同时保证图片持久化、ConfigHub凭据隔离和已有部署不被误替换。

**Architecture:** Go/Web复用现有Dockerfile，发布包由平台workflow生成，deployctl按digest/SHA管理单服务；PostgreSQL和HTTPS入口外置。持久附件与导出必须先有平台持久卷契约或外部存储，不能依赖容器可写层。当前配置先作为草稿，等待存储/平台访问策略确定。

**Tech Stack:** Go1.26、Node22、Docker Compose、deployctl1.2、ConfigHub。

**Source:** https://github.com/art-shier/deployctl/blob/main/skills/team-deploy/SKILL.md 及随附onboarding/operations；已发布平台v1.2.0提交2d3a014a7b5015051a871664c1f4c2cfab2a6566。

## 已确定约束

- application notes；container/host端口8000、127.0.0.1；readiness /api/v1/health/ready。
- Dockerfile notes-server-go/Dockerfile、context根目录；保留现有生产构建、Go/Web与Agent API。
- required_config声明DATABASE_URL、APP_ORIGIN、COOKIE_SECURE，不保存实际生产值。
- ConfigHub沿用shier/prod；notes库与专用账号不重新创建，不连接生产DB做准备验证。
- 不手写标准包四文件，不虚构digest或包链接；不为绕过平台校验编辑生成的Compose。
- 不推送发布标签、不发布新版本、不操作生产服务器；项目接入与实际发布/部署授权分开。
- notes公开；平台私有且workflow access=none。GitHub官方支持表规定公开caller只能调用公开workflow，PLATFORM_READ_TOKEN不能改变此限制。
- v1协议无volumes。现有附件/导出目录不能直接适配；已询问持久卷扩展、对象存储或仅保留准备配置的选择。

## Task 1: 可审查的本地接入准备

- [x] 阅读用户指定skill、项目Dockerfile/启动/健康接口、平台协议及真实发布引用。
- [x] 使用deployctl init dry-run再生成deployment与workflow草稿，validate退出0。
- [x] workflow保存在deploy目录，不激活与现有v*发布并行的同名Release创建流程。
- [x] 增加web-test/server-test阶段及test-release.sh，平台不依赖Runner默认Go/Node版本。
- [x] Go8包测试、Web7项、Bash语法及diff检查通过；本机无Docker，未报告镜像构建通过。
- [x] 独立审查准备配置并补充raw env、容器监听/路径与ConfigHub适配缺口；整理到项目主目录。

## Task 2: 完成受支持的发布与部署路径

- [ ] 等待用户确定附件持久化策略；若扩展平台，先设计/验证平台schema、包渲染、runtime与升级回滚持久性，不擅自新增未知volume字段。
- [ ] 等待用户确定仓库可见性/可调用的平台；同一经审查SHA固定workflow调用与platform-ref。
- [ ] 接入ConfigHub到私有config.env/secrets.env，在控制器执行前完成配置校验与DB只读预检。
- [ ] 整理原生与标准workflow的发布策略，避免Release碰撞和原生installer误选不含native资产的版本。
- [ ] 在Linux验证镜像、发布包SHA/digest、Team Deploy安装升级回滚、附件持久性与数据库兼容。
- [ ] 更新README与一键入口；根据明确发布/部署授权继续实际交付。没有目标主机时报告只完成项目接入。

## 准备验证记录

deployctl init/validate（1.2.0）退出0；Go test ./... -count=1全部8包通过；npm ci/npm test退出0，7项通过；test-release.sh Bash -n退出0。读取Github平台Release/tag/CI元数据已确认引用真实存在；未读取生产secret、调用生产ConfigHub/数据库或执行SSH。
