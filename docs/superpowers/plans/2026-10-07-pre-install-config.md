# Pre-install 自动配置实施计划

> **For agentic workers:** Use executing-plans for inline implementation; one fresh whole-change review before delivery.

**Goal:** 用户无需提前填写DATABASE_URL，由Notes pre-install读取ConfigHub并生成配置，一次安装使用生成的数据库连接。

**Architecture:** Notes不声明DATABASE_URL必填，pre脚本自包含并生成受保护的服务器配置。ctl的当前v1.5.0在pre之前冻结配置，因此需要一个显式 `hooks.pre_install.refresh_config` 布尔选项：pre成功后重新读取服务器config/env、保留override顺序、创建新的不可变快照，再启动。此选项最低ctl1.6.0；1.6.0尚未发布，不冒充已有规范或产物。

**Tech Stack:** Python>=3.10、Linux Bash/jq/ConfigHub CLI、Docker、Go。

**Spec:** 当前用户明确要求DATABASE_URL不做安装前必填，在pre-install读取生成。默认域名notes.shier.art，ConfigHub config.shier.art/shier/prod、数据库notes、专用账号字段回退规则沿用现有实现。

## Global Constraints

- 平台修改在独立源码克隆的feat/pre-install-refresh-config分支；Notes复用现有feat/team-deploy worktree。
- 不修改旧配置快照、旧Release或Compose模板；refresh只允许pre显式开启，普通hooks/v1包行为保持。
- 原始服务器文件是hook副作用；rollback恢复成功快照，不能承诺回滚数据库或服务器源文件。
- Token仅从服务器私有文件/既有CLI凭据读取，不进入命令值、Git、发布包、日志或应用环境。
- 无生产ConfigHub、数据库或SSH调用。只用临时fixture验证；版本发布仍需具体授权。

## Review Focus

- 首次空配置：pre生成的DATABASE_URL须进入实际启动快照，不能落入SQLite默认。
- pre/配置校验失败：旧服务不替换，旧状态/快照可重启。
- post/就绪失败：恢复旧版本、绑定及真实旧快照。
- 配置覆盖与秘密：secrets/持久override/本次unset与env-var顺序不变，无配置值出现在错误。
- 缺失Token、目标变化、部分写入：不泄露凭据、不切换数据库、保留原配置。

## Task 1: ctl配置回读契约与事务

**Files:** deployctl/{contract,release,runtime}.py、schemas/{deployment,release}.schema.json、tests/test_pre_install_refresh.py、版本及运行时hook文档。

**Interfaces:** 使用现有Manager.deploy和create_snapshot；可选pre `refresh_config: true`，上下文增加已校验的DEPLOYCTL_IMAGE。v2包有新字段最低1.6.0，普通hooks仍1.5.0、无hooks仍1.0.0。

- [x] 写并观察失败：flag契约、空配置生成、覆盖顺序、失败恢复、不修改初始快照、未开启行为及真实Bash。
- [x] 实现opt-in回读：pre后读取/合并/校验，必要时生成独立快照并更新candidate事务引用。
- [x] 跑完整平台测试与schema/分发校验，更新规范；提交可审查源码，不自动发布CLI。

## Task 2: Notes hooks与启动保护

**Files:** deploy/hooks/{pre-install,post-install}.sh、deploy/deployment.yaml、构建脚本/工作流、internal/config/{config,config_test}.go、Dockerfile、tests/{team_hook_test,team_config_test}.py。

**Interfaces:** 自包含Bash钩子使用ctl上下文/--set参数。pre默认TOKEN_FILE=/root/shier-prod.token、DOMAIN=notes.shier.art，读取指定CLI并写config.env/secrets.env与目标锁；post使用成功快照做账户状态检查，可按ADMIN_EMAIL仅在empty时引导。镜像要求真实DATABASE_URL，开发模式不变。

- [x] 写并观察失败：打包后无源码依赖、真实ConfigHub字段编码、失败保留、默认/参数与新声明；Go生产缺失DB拒绝。
- [x] 实现自包含hook并从required_config移除DATABASE_URL；更新构建工具到受检的新平台提交，或明确待发布状态。
- [x] 运行Bash回归、Go测试、真实v2测试包往返验证。

## Task 3: Linux集成、审查与交付

- [ ] Linux CI执行真实hook/ctl及临时TLS PostgreSQL安装和失败恢复，不能只验证mock或新字段语法。
- [ ] 独立审查两个仓库的契约、秘密和事务边界，修复Important/Critical。
- [ ] README以新命令为主，明确旧v0.2.0及尚未发布的新CLI/Notes包边界。
- [ ] 提供具体发布版本、变更和测试证据，取得新CLI/Notes版本发布授权后才生成真实产物；不操作生产服务器。

## Evidence / progress

- Task1: new contract/refresh tests RED before implementation, then GREEN. Local full platform suite: 150 tests, 12 Linux-only skips on Windows; schema, bundled zipapp build and installer source check passed. Real Docker refresh added to platform CI.
- Task2: Go production missing DB test RED then GREEN; local full Go tests/vet passed. Packaged pre missing RED then real isolated Bash regression GREEN. Standalone prepare regression GREEN. Real packager fixture round-trip v2/min1.6/no DATABASE_URL in defaults passed. Hooks have no checkout dependency.
- Task3: Linux TLS PG16 + exact packaged hooks + ctl integration added; not run locally (Docker unavailable). Workflow platform SHA will use real reviewed feature commit; no fake version/digest is published. Independent review dispatched.
- Ruling: published ctl1.5 does not reread config; implemented opt-in platform support on separate feature branch, requiring unpublished1.6. This avoids mutating immutable snapshots or shipping a hook that cannot affect startup. Cost if not accepted upstream: new Notes package must wait for supported platform release.

- Independent whole-change review: platform transaction accepted; Important interrupted-publication restoration and ambiguous post-failure assertion are being fixed. No Critical findings.
- Final: minor (deferred): free_port may rarely reuse a released fixture port; this can fail CI binds, not production port allocation.
- Review exclusions resolved: workflow placeholder replaced by actual platform commit9e62828; Docker success remains pending CI. DNS/HTTPS/production capacity require target-server access, outside this configuration change. DB migrations/invites/source changes are deliberate external side effects, not snapshot rollback.

- Task1 complete: platform feature commit9e62828, draft PR https://github.com/art-shier/deployctl/pull/1, CI37638958818 all4jobs passed including real Docker refresh/rollback. No merge/tag/release.

- Final fix: interrupted publication regression initially did not expose changed config because fixed generated contents were identical; test now stages a changed history limit. Observed RED (zero-status cleanup discarded backups), then GREEN after restoring whenever publishing=true and explicit TERM/HUP/INT handling. Actual TERM case also GREEN.
- Final fix: post recovery test now requires post_install in error plus last-failure.json phase/version, preventing earlier failures from falsely satisfying the scenario. Linux integration must supply execution evidence.
- Ref pin: Notes CI/release both use platform9e62828e4f2a31f1b06d9bb5a500a16baafb8609. Bundle/build consistency, Go suite/vet, Bash syntax, v2 package roundtrip and actionlint passed after fixes.
