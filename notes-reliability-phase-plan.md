# 拾记第三阶段：历史版本与完整图文导出

> 执行方式：本轮在当前工作区直接实施，先验证失败测试再实现，最后独立审查。沿用现有 outputs 项目与文档目录；没有 Git 仓库，不初始化或创建无关分支。

目标：用户可以找回已保存的标题和正文，并导出包含图片、可离线阅读页面及原始数据的 ZIP。用户已批准实施这两项，服务器部署和整库恢复演练仍是下一阶段。

## 合约

1. 每次成功创建或修改笔记时，在同一事务保存不可变快照，覆盖 Web、Agent、块操作、移动/收藏/标签关联、回收/恢复以及删除标签引起的版本变化。失败、冲突和幂等重放不增加快照。旧笔记迁移仅建立当前版本基线，不能重建过去。默认每篇保留最近200个已保存版本（NOTE_HISTORY_LIMIT 可配置2–2000）。不会删除历史图片。
2. GET /notes/{id}/history（limit20，最大100，before_version分页）与 GET /notes/{id}/history/{version} 要求 notes:read；列表只返回摘要、时间、来源和动作，详情含快照正文及私有图片地址。快照保存当时的标题、正文、块ID、文件夹、标签、收藏和回收状态。
3. POST /notes/{id}/history/{version}/restore 要求 notes:update 和 expected_version。恢复标题及JSON正文/块ID，文件夹、标签、收藏、回收状态及原始创建来源沿用当前设置；回收站内先通过原有恢复接口恢复笔记。生成新版本，恢复前内容仍在历史中。旧版本图片必须仍存在且属于当前用户，失败全部回滚。
4. POST /exports（include_trash、include_history 默认true）要求 notes:read、folders:read、tags:read、attachments:read。同一数据库读取快照产生 ZIP：library.json（当前笔记/分类/标签）、notes/{UUID}.html、index.html、history/{noteUUID}/{version}.json、attachments/{UUID}.{类型}及manifest.json（文件清单、大小、SHA256、选项、数量）。JSON为无损数据；HTML可离线查看图文，不执行正文HTML或脚本；所有标题/文本转义。没有账号密码、会话、邀请或Token。
5. 导出只包含当前用户及所选笔记当前/历史引用的图片，图片去重；包含历史时保留历史独占图片。缺图或跨用户引用导致整个任务失败，不能生成缺图的包。PostgreSQL用REPEATABLE READ、SQLite显式BEGIN获得一致读取。导出默认未压缩总内容上限2GiB（EXPORT_LIMIT_BYTES可配置），临时文件不计入图片配额，不当作整库备份。
6. 导出先生成临时私有文件再原子标记就绪，返回id、下载地址、数量及大小；GET /exports列出当前用户未过期任务以便重试下载；GET /exports/{id}/download重新校验身份、四项权限和有效期，使用浏览器原生下载。默认有效15分钟，每用户最多2个未过期任务；失败清理文件/记录，过期任务在启动、生成或下载时清理。重启能继续下载未过期已就绪文件；构建中任务失败或过期清理（最长1小时）。禁止直接静态公开临时目录。
7. 编辑页更多菜单进入历史，预览只读，恢复前明确影响并确认。进入前保存队列必须成功；恢复冲突保留预览，允许显式刷新当前版本后重新决定。设置中的导出页有选项、准备中和错误状态，不新增导航。Skill新增 history、history-get、history-restore、export命令，恢复仍需明确expected-version；大包分块下载、失败删除不完整文件且不覆盖已有文件。

## 审查重点

- 并发修改与快照同事务，不把旧正文标成新版本；删除标签也不漏快照。
- 恢复不删除当前内容、不扩大回收站权限、不复活已删除标签。
- 导出过程并发修改保持一致，历史引用图片不会遗漏或越权；缺图明确失败。
- HTML标题、文本、链接与图片路径安全，ZIP路径只使用UUID/固定名称。
- 导出失败、过期、重启与下载期间清理不遗留无限临时文件或误删正在下载的文件。

## 实施任务

### 1. 历史 API 与迁移

文件：models.py、history.py、notes.py、blocks.py、tags.py、config.py、main.py、Alembic新迁移；tests/test_history.py、test_migration.py。

- [x] 写失败测试并执行：创建/连续编辑/块修改/删除标签快照、失败与幂等不增加、双用户与只读、恢复保留当前设置和旧图、并发恢复仅一个成功、分页/200版本保留边界。
- [x] 实现 record_revision(db,note,actor,action,limit,restored_from=None) 与三个 history 路由；atomic_update 同事务加入快照，迁移为旧笔记建立基线。
- [x] 执行 pytest tests/test_history.py tests/test_migration.py -q 通过，检查升级/降级及 schema check。

### 2. ZIP 导出与下载

文件：models.py、exports.py、archive.py、config.py、main.py、新迁移；tests/test_exports.py。

- [x] 写失败测试并执行：空库/完整图文与离线HTML、历史独占图片、回收选项、用户和权限、缺图/越权/上限原子失败、快照一致性、过期清理和重开下载。
- [x] 实现一致读取归档、文件清单校验、私有临时任务与原生下载；限制活跃任务，下载通过已打开文件描述符避免过期清理影响已经开始的传输。
- [x] 执行 pytest tests/test_exports.py -q 通过并用zipfile核验所有manifest哈希和图片字节。

### 3. Web 与 Skill

文件：HistoryPanel.tsx、editorExtensions.ts、NoteEditor.tsx、Settings.tsx、App.tsx、api.ts、styles.css；notes-skill/shiji-notes/SKILL.md、references/api.md、scripts/notes.py；tests/test_skill_cli.py；隔离浏览器脚本。

- [x] 补CLI失败测试：历史请求参数、版本冲突不重试、导出超12MiB分块下载、拒绝覆盖和失败文件清理；实现并通过。
- [x] 接入历史列表/预览/确认恢复与导出选项/原生下载，复用现有保存、恢复和样式约定。
- [x] 实际浏览器验证 Web/Agent历史往返、图片恢复、冲突/只读、导出解压离线查看、320/390/430布局及旧草稿逻辑。

### 4. 交付验证

- [x] 全部后端/CLI与前端队列测试、生产构建、Skill校验、主库备份迁移重启，独立审查并修复重要发现。
- [x] 更新README/技术状态/开发记录；清理隔离测试原始凭证，保持主库不引入测试账号。生产部署、导入和整库恢复演练仍明确未验收。


## 2026-10-03 验证与开发记录

- 后端与CLI全套：61项通过；前端保存/草稿：7项通过；生产构建通过；Skill结构校验通过。
- SQLite旧笔记迁移建立当前快照基线的回归、升级/降级和Alembic schema check通过。主库迁移前通过SQLite backup API保存副本，迁移并重启后健康检查通过；主库仍无测试账户。
- 实际隔离浏览器：含图旧版预览只读；外部修改后恢复返回409，显式刷新后恢复成功；恢复后的下一次编辑使用新版本保存。Web和Skill历史往返、只读拒绝写入、跨用户404和撤销Token立即失效通过。
- 原生浏览器生成与再次下载ZIP、Skill复用现有包下载、历史/回收选项、历史独占图片通过。两个导出包所有文件大小/SHA256与manifest逐一一致，CLI和浏览器下载字节一致。
- 解压后的index.html与图文页面在file://下打开，图片正常加载，文本中的script字符仅作为文本显示，无脚本节点；主界面、版本预览、导出和离线页面在320/390/430宽度无横向溢出。截图已目视检查。
- 独立审查复现了缺失实体图片仍恢复、删除标签与新增关联交错漏历史两处问题。新增回归先失败再通过；恢复增加路径/文件/字节数校验，标签操作采用共同事务锁。最终复查未发现新的重要问题。
- 主API载入最终修复并保持运行。隔离API实际停止/重启后，版本/历史仍在，未过期ZIP再次下载字节完全一致；完成后停止隔离服务并清理原始测试凭证。主Web5173/API8000保持可用。
- 导出准备中的保留期限明确为最长1小时：失败立即释放，异常终止由过期清理回收。多worker重启不直接删除其他worker正在生成的文件；实例租约/即时废弃任务回收尚未实现。
- Docker配置补充私有exports持久卷与目录权限；仅做配置检查，本机无Docker且无服务器凭证，PostgreSQL/容器/HTTPS生产验收未执行。导入、附件/回收站清理、管理员网页、限流、审计及整库恢复演练未包含在本阶段。
