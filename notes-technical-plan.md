# 拾记技术方案

日期：2026-10-03。产品约束：移动端 Web 优先，首页按文件夹聚合，不使用导航 Tab；多用户私有空间，不做共同编辑；自有服务器部署；Agent 通过 Skill 和 REST API 访问，不引入 MCP。

当前状态：服务端已迁移至 `notes-server-go`。账户、笔记、标签、Markdown、块操作、历史、图文 ZIP 与离线备份恢复均已有 Go 实现；本地预览使用 Go API。以下第一阶段边界保留为开发过程记录，当前验收范围及部署差距见 Go 迁移计划和部署指南。

## 1. 架构与技术

采用模块化单体，避免首期引入独立鉴权服务、消息队列和微服务。React + TypeScript + Tiptap 沿用现有前端；Go + Gin 提供版本化 REST API 和 OpenAPI；GORM 访问数据库，Go CLI 管理兼容 schema。生产数据库使用 PostgreSQL 16，开发和自动测试可用 SQLite。SQLite 不作为多人正式部署的默认数据库。

浏览器 → 同域 Caddy → 静态网页 / Go API → PostgreSQL + 私有附件目录。

Agent → 用户配置的 HTTPS API 地址 → 同一 Go API 服务，Bearer Token 认证。

Vite 开发服务器把 /api 代理到本机 API，浏览器不持有服务端密钥。生产同域部署，不开放任意来源 CORS。Cookie、附件读取、CSRF 均使用同域边界。

## 2. 本轮开发边界

先交付真实可运行的基础闭环：邀请注册、登录/退出、用户隔离、三级文件夹、笔记 JSON 正文、搜索、收藏、回收站、图片上传与读取、版本冲突保护、API Token 权限与撤销、前端服务端保存。通过两个账户和浏览器刷新验证。

现有本地原型数据仍保留在浏览器原键中，不自动导入任意登录账号；新用户服务端只创建收件箱，不伪造示例笔记。旧原型素材和截图保留。后续提供明确选择账号的导入入口。

第一阶段不声称实现历史版本、标签、Markdown 无损转换、按块操作、幂等创建、存储自动清理、备份恢复验证、完整管理员网页或公开注册。这些保持在整体方案中，并按后续阶段实现。Skill 正式包在 API 稳定后实现，本阶段 Token 可以直接调用已公开的 REST API。

## 3. 模块和数据

后端按 config、db/models、security/auth、content、folders、notes、attachments、tokens、cli 分文件，路由和事务保持在对应功能模块。前端新增 api、认证页和持久化协调器，现有文件夹页与编辑器只负责呈现和输入。

主要实体使用 UUID、UTC 时间：users（email、显示名、密码哈希、禁用状态、角色、配额）；invitations（哈希、邮箱、角色、有效期、使用时间）；sessions（密钥哈希、用户、过期时间）；folders（用户、父级、名称、收件箱标记）；notes（用户、文件夹、标题、content_json、plain_text、收藏、删除标记、version）；attachments（用户、私有对象键、MIME、字节数）；api_tokens（用户、名称、密钥哈希、前缀、权限、有效期、撤销时间）。

所属用户只从认证上下文取得，不能通过客户端 user_id 切换。读取未知或其他用户的 UUID 一律 404。文件夹、图片和笔记引用在写入前校验归属，并使用数据库约束保证基础关联。

文件夹最多三级，创建时禁止同级重名。收件箱不可删除。文件夹移动、删除和批量整理为后续接口，避免先开放难以恢复的动作。

## 4. 身份与权限

密码使用 Argon2id，最少 12 字符，无默认账号和默认密码。首个管理员由 CLI 生成一次性邀请，注册时用户自己设置密码；后续邀请由管理员命令创建。邀请和注册消费在一个事务中完成，过期、已用或邮箱不匹配不能注册。

网页使用可撤销的服务端 Session，HttpOnly Cookie，SameSite=Lax；生产开启 Secure。每次写请求同时校验允许的 Origin 与会话 CSRF Token。CSRF Token 由登录/当前用户接口返回，网页仅保存在内存；退出撤销会话。禁用用户使已有 Session 和 Token 立即失效。

第二阶段在原权限基础上新增 tags:read、tags:write；读写模板可选 notes:trash，旧 Token 不自动升级。

API Token 使用高熵随机密钥，完整值仅创建时返回一次，数据库只保存 SHA-256 哈希和前缀。权限是 notes:read、notes:create、notes:update、notes:trash、folders:read、folders:write、attachments:read、attachments:write。只读模板不能写；读写模板默认不含删除权限。Token 不能管理其他 Token、邀请或账号。

首次本机预览在 127.0.0.1 运行；生产启动必须显式提供 HTTPS Origin，并开启安全 Cookie。未完成部署验证前不宣称适合直接开放公网。

## 5. 正文、图片与配额

Tiptap JSON 是唯一正文事实来源。服务端使用节点/属性/marks 白名单校验并派生纯文本，拒绝未知节点、任意 HTML、脚本 URL、外部图片和 data: 图片。第一阶段支持段落、标题、列表、引用、代码、基础行内格式、链接、图片、水平线。

图片 JSON 存 attachment_id；读取响应补充认证图片地址供网页展示。附件目录不由静态服务器公开。通过 /api/v1/attachments/{id} 读取时重新鉴权。允许 JPEG、PNG、WebP、GIF，Pillow 校验真实文件；默认上限 10 MiB，单用户默认配额 1 GiB，限制像素数量。配额通过用户行锁/原子更新维护，客户端 MIME 不可信。文件写入失败回滚数据库并清理临时文件；下载防止 MIME 嗅探。

第一阶段保留上传后未引用附件，不自动删除，避免误删。后续加入当前正文与历史引用关系和可重试清理任务。

## 6. API 合约

统一 /api/v1；JSON 使用 snake_case。分页 limit 默认 20、最多 100，cursor 为下一页位置，列表仅返回元数据和摘要。详情返回 content_json。错误格式为 error.code、error.message、error.details、request_id。

| 路径 | 行为 |
|---|---|
| POST /auth/register | 消费邀请、建立账户和收件箱 |
| POST /auth/login；POST /auth/logout；GET /me | 登录、退出、当前身份和 CSRF Token |
| GET /folders；POST /folders | 列出、创建所属用户的文件夹 |
| GET /notes；POST /notes；GET /notes/{id} | 查询摘要、创建、读取详情 |
| PATCH /notes/{id} | expected_version 原子修改、移动、收藏 |
| DELETE /notes/{id}；POST /notes/{id}/restore | 携带 expected_version 移入回收站、恢复 |
| POST /attachments；GET /attachments/{id} | 上传、授权读取图片 |
| GET /tokens；POST /tokens；DELETE /tokens/{id} | 网页用户管理自己 Token |
| GET /health/live；GET /health/ready | 存活、数据库就绪 |

notes 查询支持 query、folder_id、favorite、trash；默认按 updated_at/id 倒序。中文先采用连续片段匹配，索引和分词优化另做性能阶段。身份隔离在搜索与分页中同样生效。

写入使用 WHERE id=:id AND user_id=:user AND version=:expected 原子条件更新，成功递增 version，409 返回服务器当前版本。回收站与恢复也递增版本，避免旧客户端覆盖状态。

## 7. 网页保存

停止输入约 800 ms 自动保存；同一笔记请求串行，保存期间继续输入合并为下一次写入，不能把较旧响应覆盖到最新内容。状态为已保存、待保存、保存中、失败/冲突。

未保存草稿键包含用户 ID、笔记 ID 和基础版本。保存成功清理匹配草稿；失败保留。切换页面先尝试保存，冲突阻止覆盖并提供复制本地内容和重新载入服务器内容。退出需先保存成功，或显式确认放弃页面内编辑。浏览器重开时提示恢复同账号草稿。

附件先上传得到 ID，再插入正文；失败不插入坏链接。初次登录、加载失败、空文件夹和无搜索结果都有明确状态。主题继续是本地偏好。

## 8. 部署与后续阶段

提供 Dockerfile、Compose、Caddy 配置与 .env.example。PostgreSQL 和附件分别挂持久卷，不映射数据库公网端口。迁移使用 Alembic upgrade head，在应用启动前执行。健康检查只验证自身进程和数据库，不暴露用户数据。日志不记录密码、Cookie、完整 Token 或笔记正文。

开发本机暂时没有 Docker，先验证 SQLite API 与前端；容器构建、PostgreSQL 集成、HTTPS、进程重启和独立环境恢复必须在有 Docker 的环境执行，不用本地测试代替。

后续阶段：① 标签、Markdown/块 API、幂等请求和正式 Skill；② 历史版本、导入/导出、回收站清理与附件回收；③ 登录/上传限流、管理网页、审计、备份和恢复演练；④ 服务器 HTTPS 验收。公开注册需单独补邮箱验证、找回密码、滥用控制，不提前开启。

## 9. 验收重点

两个用户互相读取/改写笔记、文件夹和图片均失败；只读 Token 写入失败，撤销后马上拒绝；伪装图片、超限上传和跨用户图片引用失败；两个客户端同版本修改只有一个成功；保存过程中持续输入不丢最后一次内容；刷新后图文仍一致；服务重启后数据库内容仍在；旧 localStorage 原型数据未被删除。

## 10. 第二阶段已实现（2026-10-03）

见 [Agent 阶段方案](notes-agent-phase-plan.md) 和 [Skill](notes-skill/shiji-notes/SKILL.md)。标签与笔记关联按用户隔离；Markdown 转换检查结构往返，无法无损转换的正文不允许全文覆盖；块 ID 单独保存，修改批次原子并检查版本；幂等创建绑定用户、Token/会话和路由，并与笔记创建同事务。Web 编辑页接入标签且不新增导航。旧 SQLite 的升级、列表结构规范化和块 ID 回填有迁移回归测试。

第二阶段当时尚未交付历史与图文导出，现已按下节实现。生产 PostgreSQL/Docker/HTTPS 验收以及导入、清理、限流与运维仍未交付，不因本地通过而标为生产通过。

## 11. 第三阶段：历史与图文导出（2026-10-03）

见 [历史与导出方案](notes-reliability-phase-plan.md)。新增 note_revisions 与 export_bundles；每次笔记成功写入同事务记录不可变快照，默认保留最近200版。恢复仅标题与正文/块ID，沿用当前分类、标签和收藏，生成新版本；缺失图片或版本冲突全部回滚。迁移为旧笔记保存当前基线。

图文 ZIP 从同一数据库读取快照生成，包含当前/可选历史、引用图片、离线 HTML、原始 JSON 与 SHA256 清单。默认未压缩上限2GiB，每用户最多2份短期导出，准备好后15分钟有效；构建占位最长1小时。下载重新鉴权、原生传输，Skill分块保存并支持重试已有包。Compose新增私有exports卷，生产容器实际运行仍未验证。

开发 SQLite 的全后端/CLI测试61项、前端保存/草稿7项、生产构建、Skill校验、迁移schema检查通过。真实隔离浏览器验证版本预览/409刷新/恢复后继续保存、Web与Agent历史往返、原生下载及再次下载、选项过滤、历史独占图片、离线HTML图片和320/390/430布局；两份ZIP逐文件哈希核验通过。阶段记录还包含重启验证与独立审查。

## 12. 部署准备与整库恢复（2026-10-03）

新增管理员离线备份/校验/新环境恢复CLI，数据库与全部附件以SHA256清单校验；SQLite包含尚未checkpoint的WAL，PostgreSQL使用custom dump及单事务恢复。恢复不覆盖已有库，旧网页会话和临时导出清空，账号、密码、Token、分类、标签和历史保留。Compose提供停写备份、独立项目恢复及HTTPS健康检查脚本；镜像固定PGDG16客户端，与PG16服务端匹配。

实际本地用双用户隔离库经独立CLI进程备份与恢复，重新启动恢复库API/Web后验证登录、图片、历史恢复、用户隔离与原Agent Token继续读写。正式服务器连接信息尚未提供，容器/PostgreSQL/HTTPS验收未完成。详情见 [部署指南](notes-deployment-guide.md) 和 [实施与验证记录](notes-operations-phase-plan.md)。没有新增定时任务或公开注册。
