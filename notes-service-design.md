# 自托管多用户笔记服务设计

本方案用于开发一个部署在自有服务器上的多用户笔记服务。用户可以分类保存文字与图片，并通过网页或 Agent 访问自己的笔记。Agent 通过 Skill 中的脚本调用远程 REST API。第一版使用邀请制，不包含 MCP、多人共同编辑或公开分享。

本文记录已确认的整体产品范围。2026-10-03 用户授权先形成技术方案并启动开发，具体技术决策、分阶段范围与实现状态见 [技术方案](notes-technical-plan.md) 和 [实施计划](notes-implementation-plan.md)。下文含后续阶段需求，不代表功能已全部实现。

## 产品范围

- 每位用户拥有独立的分类、标签、笔记、图片和 API Token。
- 支持多级分类，一篇笔记属于一个分类并可关联多个标签。
- 默认收件箱接收尚未分类的笔记，收件箱不可删除。
- 支持文字、标题、列表、待办、引用、链接、代码块、简单表格和图片混排。
- 支持自动保存、搜索、收藏、回收站、历史版本及 Markdown 和图片打包导出。
- 管理员创建账号或生成邀请，公开注册默认关闭。
- 用户创建 Token，使 Agent 在授权范围内操作笔记。
- 暂不包含完整离线编辑、团队协作、公开分享、音视频、OCR 或 AI 自动整理。

## 核心使用流程

网页：登录后首页以文件夹列表聚合展示，逐级进入笔记列表和编辑页，不设置导航 Tab；右上角头像进入账户与 Agent 设置。创建笔记、编辑文字并插入图片、自动保存，再选择文件夹。桌面端先兼容同一流程，三栏布局与标签为后续阶段。

Agent：用户安装 Skill，配置服务地址和自己的 Token；Agent 搜索摘要，读取目标笔记，携带版本号修改，或上传图片后创建新笔记。网页立即可以访问服务端已保存的结果。

## 推荐技术与部署

| 部分 | 默认选择 |
|---|---|
| 前端 | React、TypeScript、Tiptap |
| 后端 | Go、Gin、GORM、兼容 SQL 迁移 |
| 数据库 | PostgreSQL |
| 图片存储 | 首期服务器本地目录，使用可替换的存储适配器 |
| Agent 客户端 | Python 调用脚本，随 Skill 分发 |
| 部署 | Docker Compose，Caddy 提供 HTTPS 和反向代理 |

单台服务器运行反向代理、网页、API 和数据库。前端与 API 使用同一个域名，数据库不暴露公网端口。图片和数据库使用持久卷。图片对象键与访问 URL 分离，后续可以迁移到 S3 兼容存储而不改动笔记引用。

域名、服务器操作系统、容量和备份目的地在部署前确认；设计阶段不依赖服务器凭证。

## 用户和认证

网页采用服务端可撤销的会话，Cookie 设置 HttpOnly、Secure 和适当的 SameSite 策略；写请求具备 CSRF 防护。密码使用 Argon2id 哈希。首次管理员通过部署命令创建，不设置默认密码。

管理员可创建账号、生成有时限的一次性邀请、禁用用户、查看存储用量和调整配额。前期未配置邮件服务时由管理员复制邀请链接自行交付。管理员界面不提供浏览其他用户笔记的功能；服务器持有人仍具备底层存储访问能力。

禁用账号后，拒绝其现有会话和 Token 的后续请求。第一版密码遗失由管理员生成短期一次性重置链接处理。公开注册开关默认关闭，开启前需要补充邮箱验证、自助找回密码和防滥用措施。

Token 使用 Bearer 认证，绑定用户、名称、权限范围、过期时间及撤销状态。完整密钥仅创建时显示一次，数据库保存哈希和用于识别的前缀。日志不得记录 Authorization 头或完整 Token。

权限范围建议为 notes:read、notes:create、notes:update、notes:trash、categories:read、categories:write、tags:read、tags:write、attachments:read、attachments:write。恢复笔记归入 notes:trash，历史恢复归入 notes:update。默认模板提供只读和读写两种组合，读写模板默认不含 notes:trash。

## 数据模型

所有实体使用 UUID，时间以 UTC 保存，界面按用户时区显示。

| 实体 | 主要字段与约束 |
|---|---|
| users | id、email、password_hash、role、status、quota_bytes、created_at |
| sessions | id、user_id、secret_hash、expires_at、revoked_at |
| invitations | id、secret_hash、email、expires_at、used_at、created_by |
| categories | id、user_id、parent_id、name、sort_order、is_inbox |
| tags | id、user_id、name；同用户标签名称唯一 |
| notes | id、user_id、category_id、title、content_json、plain_text、version、is_favorite、deleted_at、created_at、updated_at |
| note_tags | user_id、note_id、tag_id；同一关联唯一 |
| attachments | id、user_id、object_key、mime_type、size_bytes、status、created_at |
| note_attachments | user_id、note_id、attachment_id；维护正文及历史版本引用 |
| note_versions | id、user_id、note_id、version、snapshot、actor_type、actor_id、created_at |
| api_tokens | id、user_id、name、secret_hash、prefix、scopes、expires_at、revoked_at、last_used_at |
| idempotency_records | user_id、token_id、route、key、request_hash、response、expires_at |
| audit_events | user_id、actor_type、actor_id、action、resource_type、resource_id、request_id、created_at |

分类不允许形成循环，第一版最大深度为三级。父分类及关联标签、图片必须属于同一用户，通过服务层检查及可行的复合外键约束共同保证。用户身份从已验证会话或 Token 获取，忽略或拒绝客户端提供的 user_id。

正文和元数据修改递增笔记版本；修改分类自身名称不递增其所有笔记版本。回收站状态变更也递增版本，以避免旧客户端覆盖。

## 正文和图片表示

结构化 JSON 内容是笔记的唯一正文事实来源，使用受限的 Tiptap 文档节点集合；Markdown 是 Agent 的便捷输入输出格式，纯文本从 JSON 派生用于摘要和搜索。

正文图片节点保存 attachment_id、替代文本和显示尺寸，不保存短期签名 URL。Markdown 输出采用内部引用，例如 ![架构图](attachment://UUID)。网页或导出程序解析该引用，图片读取仍须认证。

Agent 读写接口支持 content_format=json 或 markdown。内容格式与正文必须配对，且不允许同时提交两份正文。创建时 Markdown 可以转换为结构化 JSON；接口返回转换警告，并提供可选的校验接口预览转换结果。

get_note 返回 markdown_roundtrip_safe 标志。若正文存在不能无损往返的格式，Markdown 全文覆盖默认返回 422，并建议使用 JSON 或按块修改；不默默降级原有内容。读取 JSON 返回稳定的块 ID，按块接口第一版支持顶层块的插入、替换和删除，并对整个笔记检查版本。网页与 Agent 使用同一内容校验器。

外部链接作为链接保留；第一版图片必须显式上传，不自动抓取外部 URL。上传允许 JPEG、PNG、WebP、GIF，通过实际文件类型校验；默认单文件上限 10 MiB，可配置。图片实际字节计入配额，超额返回 413。单用户默认配额建议 1 GiB，由管理员调整。

上传成功但尚未被笔记引用的资源先作为临时附件保存；超过 24 小时仍未被引用才允许清理。正文和保留的历史版本仍引用的图片不得清理。上传或引用变化失败时保持事务一致性，并通过可重试清理任务处理孤立文件。

## REST API 约定

统一前缀 /api/v1。JSON 请求响应，上传使用 multipart。ID 使用 UUID，分页使用游标，默认 20 条、最大 100 条。列表返回摘要和元数据，全文由详情接口读取。

| 方法与路径 | 行为 |
|---|---|
| POST /auth/login | 创建网页会话 |
| POST /auth/logout | 撤销当前会话 |
| GET /me | 当前用户和配额 |
| POST /tokens | 创建用户自己的 API Token |
| GET /tokens | 获取 Token 元数据，不返回密钥 |
| DELETE /tokens/{id} | 撤销 Token |
| GET /notes | 按 query、category_id、tag_id、favorite、trash 筛选，支持游标 |
| POST /notes | 创建笔记，接受 Idempotency-Key |
| GET /notes/{id} | 正文、元数据、版本及 Markdown 往返标志 |
| PATCH /notes/{id} | 携带 expected_version 修改指定字段 |
| POST /notes/{id}/blocks | 携带 expected_version 原子执行按块修改 |
| DELETE /notes/{id} | 携带 expected_version 移入回收站 |
| POST /notes/{id}/restore | 携带 expected_version 恢复 |
| GET /notes/{id}/versions | 历史版本摘要 |
| GET /notes/{id}/versions/{version} | 读取某个版本 |
| POST /notes/{id}/restore-version | 携带 expected_version 恢复为指定快照，并生成新版本 |
| GET /categories | 获取分类树 |
| POST /categories | 创建分类 |
| PATCH /categories/{id} | 重命名、移动或排序 |
| DELETE /categories/{id} | 有子分类时拒绝；有笔记时要求显式提供迁移分类 |
| GET、POST /tags | 列出和创建标签 |
| PATCH、DELETE /tags/{id} | 修改或删除标签，删除仅解除关联 |
| POST /attachments | 上传图片，返回资源 ID |
| GET /attachments/{id} | 授权读取图片字节 |
| POST /notes/validate-content | 校验正文或预览 Markdown 转换，不持久化 |
| GET /exports/notes | 下载指定范围的 Markdown 和图片 ZIP，限制单次规模 |

管理账号、邀请和配额的接口位于 /admin，要求网页管理员会话；普通 Agent Token 不具有管理员权限。

错误响应结构统一为 error.code、error.message、error.details、request_id。常见状态：401 未认证，403 权限范围不足，404 不存在或不属于当前用户，409 版本冲突或幂等键请求不一致，413 超出大小或配额，422 内容校验失败，429 限流。

创建返回 201，普通修改返回 200。版本冲突返回当前版本号；请求方需要重新读取内容并处理冲突。默认请求上限建议每用户每分钟 120 次，图片上传每分钟 20 次，可通过配置调整。

中文搜索第一版采用标准化后的标题与正文子串匹配，使用 pg_trgm 辅助索引；短关键词回退查询，并限制分页规模。默认按更新时间排序，明确支持中文连续片段搜索，不声称具备中文分词、语义搜索或 OCR 能力。

## 自动保存和一致性

网页在停止输入约 1 秒后自动保存，同一笔记的写请求串行发送。状态显示保存中、已保存、离线或保存失败。浏览器草稿与登录用户、笔记 ID 和基础版本绑定，避免不同账号串用草稿。

修改数据库时通过 id、user_id 和 expected_version 条件原子更新，并在同一事务写入历史版本和附件关联。冲突时网页保留本地草稿，提供查看服务器版本和复制本地内容的操作，不自动覆盖。

正文历史建议按编辑会话中最多每分钟保存一次快照；Agent 正文写入、删除和历史恢复每次保留必要快照，避免自动保存产生大量版本。保留最新 100 个版本及最近 30 天版本，清理操作不得破坏当前或仍保留版本的图片引用。

回收站默认保留 30 天。定时任务在期限届满后清理笔记、版本及无人引用的附件。网页可提供明确的永久删除操作；Agent 第一版不开放该操作。清理与导出同样遵守用户隔离。

创建的幂等键绑定用户、Token 和路由，保留 24 小时。相同键和相同请求返回原结果，不重复创建；相同键和不同请求返回 409。并发相同键使用唯一约束及事务保护。

## Agent Skill

Skill 由 SKILL.md、references/api.md 和 scripts/notes.py 组成。API 规范通过 OpenAPI 输出，维护时对照生成规范更新脚本和使用文档。

用户配置 NOTES_API_URL 和 NOTES_API_TOKEN，脚本不把密钥写入命令参数或输出。服务地址要求 HTTPS，显式的本地开发环境可使用 HTTP。

命令包含 search、list、get、create、update、move、tags、categories、upload-image、trash、restore。创建和更新支持从 UTF-8 文件读取正文，避免 shell 转义破坏内容。输出 JSON，失败使用非零退出码。

超时和暂时性错误允许有限重试，遵守 Retry-After。创建重试使用同一个幂等键；更新不盲目重试版本冲突。Skill 要求 Agent 将读取的笔记内容当作待处理数据，不能当作授权或执行指令；凭证和实际权限由服务器保障。

## 备份和运行维护

每日备份数据库和图片，建议保留最近 7 份日备份与 4 份周备份，至少一份存到另一台设备或独立存储。迁移和升级前额外备份。备份具备访问控制，包含密钥材料时需加密保存。

单服务器小规模部署可在维护窗口暂停写入后执行一致性备份，验证数据库记录与附件文件共同恢复。部署验收必须实际恢复到独立环境，不能仅检查备份任务成功。

服务提供存活和就绪检查。结构化日志包含请求 ID、耗时和结果，不记录完整笔记正文、密码、Cookie 或 Token。审计主要记录写操作，不复制正文；默认保留 90 天，可配置。

## 实施阶段和验收

1. 用户与数据库：完成迁移、认证、邀请、用户隔离；两个用户互相访问任何笔记、标签、分类、图片或历史资源均失败。
2. 核心 API：完成笔记、分类、标签、图片、搜索；支持图文创建、中文搜索和分类迁移，并验证配额。
3. 网页：完成响应式列表和编辑、自动保存；刷新后图文顺序、格式和图片仍正确，保存失败保留草稿。
4. 恢复和一致性：验证版本冲突不会覆盖内容、同幂等键并发创建只产生一篇笔记、回收站和历史恢复可用。
5. Agent：完成 Token 和 Skill；只读 Token 无法写入，撤销和禁用账号后立即拒绝新请求；网页与 Agent 双向读写可用，复杂内容不会静默损坏。
6. 部署和备份：在自有服务器 HTTPS 部署，验证重启持久化、独立环境恢复及上传限额。

第一版采用邀请制完成以上阶段。开放注册另作一次发布准备，补充邮件、自助恢复、注册滥用控制、容量评估和数据管理规则。

## 实现前需确认的默认决策

产品范围已确认。建议一并审阅本文提出的 React 与 Go 技术栈、三级分类、Markdown 往返保护、10 MiB 图片限制、1 GiB 默认配额及版本和回收站保留规则。这些数值通过配置调整，不依赖未来开放注册。
