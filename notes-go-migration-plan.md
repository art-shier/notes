# 拾记服务端 Go 迁移实施计划

用户已明确要求将后端改为Go。本轮直接实施，保留已确认前端样式、REST API与Skill客户端；不重新询问选型批准。

## 合约与边界

- Go + Gin 服务端，GORM配合PostgreSQL/纯Go SQLite驱动；部署服务端不依赖Python。Skill的Python脚本作为独立API客户端保留。
- 同一 `/api/v1` API、snake_case字段、错误信封、Cookie/CSRF、Token权限、邀请制、用户隔离、expected_version、幂等创建保持兼容。
- 支持原有标题/JSON/Markdown、块ID、标签/分类、收藏/回收站、私有图片、历史与ZIP导出，以及完整离线备份/新环境恢复CLI。
- 保留当前数据库表和字段，兼容已升级至8942cd035480的Python版本数据库、Argon2id密码哈希和Token哈希；不改写旧正文或凭证。不支持未经升级的旧schema直接启动，明确拒绝并提供先升级方法。
- Go提供新库迁移命令与OpenAPI；数据库schema标记继续使用当前兼容版本，后续新增迁移才引入新版本。
- Python实现先保留作为兼容测试与回退参考，不作为最终生产运行依赖；主库切换前备份，先隔离数据验收，主库不添加测试账户。
- 多用户请求由Go并发处理；使用受控连接池、Argon2并发限额、导出全局限额，长下载不持有数据库连接。SQLite写事务串行化；PostgreSQL每用户写入锁保障版本/标签/配额/幂等一致性。
- 真实PostgreSQL/Docker/HTTPS仍依赖服务器环境；不能用SQLite回归或小规模并发测试宣称生产容量。

## 模块与执行

- [x] 配置、数据库/兼容schema、HTTP基础、错误与安全响应、账户/Token、附件、Go CLI入口。
- [x] 独立document包：正文白名单、纯文本/图片hydration、CommonMark转换/安全往返、块操作与稳定ID。
- [x] 笔记、文件夹、标签、版本/历史API：统一事务、鉴权、版本检查和幂等创建。
- [x] 独立归档/备份包：一致读取ZIP、SHA256/离线HTML、私有任务/下载、离线备份校验与恢复。PG16实现尚待真实环境验收。
- [x] 兼容旧库与旧哈希、Web/Skill联调、并发写入/幂等/导出限额、全套Go测试/vet、无CGO Linux构建。
- [x] Docker/Compose/脚本改用Go，文档更新；独立审查重要发现修复；备份后主预览切换Go。

按并行任务技能仅委派独立文件域；根代理负责基础接口、集成及交付。所有文件位于outputs/notes-server-go，临时数据与脚本位于work；不建立无关Git仓库。

## 审查重点

1. Python库读取与时间/JSON/Argon2兼容，不丢用户、Token、附件、历史。
2. 并发CAS/幂等和删除标签快照原子性，不因语言迁移放松权限/CSRF。
3. 文档严格白名单、Markdown往返保护与旧块ID保留。
4. 导出保持同一读取快照、图片完整/归属/安全路径；恢复不覆盖已有库。
5. 请求/连接/CPU任务限额以及超限清理，不持连接长时间传输，不猜测并发容量。

## 本轮验收记录（2026-10-03）

- `go test ./... -count=1`：47个通过节点（含子用例）、1个符号链接用例跳过。Windows账号没有创建符号链接权限；相关路径仍有逐级Lstat检查，需Linux补跑。
- `go vet ./...`通过；`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath ./cmd/shiji`交叉构建成功。本机缺少gcc，未执行race detector。
- 通过远程HTTP适配器对真实Go进程运行原接口/客户端测试：52 passed、8 skipped。跳过的是依赖Python内部hook/动态配置的测试，其对应事务回滚、标签并发、保留上限、导出限制/快照/过期等由原生Go回归覆盖；旧测试仅作为兼容测试来源，不参与生产运行。
- 实际Edge/Playwright：Web图文、标签、草稿恢复、Agent Markdown/JSON/图片/块编辑、幂等、权限撤销、双用户隔离，以及320/390/430px布局通过。
- 原Python当前schema库迁入Go：旧Argon2密码、会话、Token、块ID和含HTML字符的幂等请求重放通过；Go写入历史后，经Go CLI备份/恢复，密码/Token/图片保留，旧会话失效。另成功恢复既有Python双用户v1备份。
- 独立审查发现并修复：大中文JSON传输上限不兼容、并发图片解码内存风险、健康脚本遗留Python依赖。新增大正文与并发阻塞读取回归验证修复。
- 主库停机备份至本地work/go-main-before-switch，并校验后恢复至notes-server-go/data；原Python数据保留。主Web5173/API8000已使用Go，主库用户数仍0。隔离8001/5174测试服务已停止，临时明文测试账号文件已删除。
- 初始本地迁移阶段尚未执行真实 PostgreSQL16、Docker镜像/Compose、DNS/HTTPS及生产容量压测；后续 CI 验证见下方。没有公开注册、定时备份或正式上线。

## 一键部署与 Linux CI 补充验收（2026-10-03）

- Git 仓库已初始化并推送到公开的 https://github.com/art-shier/notes；运行数据、密码配置及旧 Python 后端不进入版本库。
- [Linux CI](https://github.com/art-shier/notes/actions/runs/37112169790) 已通过 Go 全套测试和 race/vet、无 CGO 构建、Web 7 项测试与构建、安装脚本语法与回归。
- 生产 Docker 镜像构建成功；Compose 中以真实 PostgreSQL16 完成账户、图片、标签、CAS 冲突、幂等、历史恢复和 ZIP 完整性验证。原生 pg_dump/pg_restore 恢复后密码、Token、图片仍可用，旧会话失效。
- install.sh 支持公开下载、Linux 依赖安装、随机数据库密码、HTTPS 就绪检查和首次管理员邀请；重复执行保留配置和数据。审查发现的 Ubuntu 已有 Docker 缺 Compose 软件包选择问题已补回归并修复。
- 实际域名的 Caddy 证书、服务器环境、生产容量和异机备份仍需在目标服务器验收。
