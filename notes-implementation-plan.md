> 历史实施记录：当前服务端已转为 Go，执行入口与迁移范围见 notes-server-go/README.md 和 notes-go-migration-plan.md。以下 Python 路径保留用于追溯旧版。

# 拾记基础服务实施计划

> For agentic workers: use superpowers:executing-plans to implement each task and verify the interfaces.

**Goal:** 建立可运行的多用户图文笔记闭环，现有移动 Web 使用真实 REST API。

**Architecture:** FastAPI 模块化单体，SQLAlchemy/Alembic，私有本地附件；React 沿用现有布局并接入同域代理。JSON 正文为事实来源，服务端强制用户隔离与版本检查。

**Tech Stack:** Python 3.12、FastAPI、SQLAlchemy 2、Alembic、Argon2id、Pillow、PostgreSQL 16 / 开发 SQLite、React 19、TypeScript、Tiptap。

**Spec:** [技术方案](notes-technical-plan.md)，[整体产品方案](notes-service-design.md)。本轮范围以技术方案第 2 节为准。

## Global Constraints

- 不引入 MCP，不增加导航 Tab，不做共同编辑，不开启公开注册。
- 密码至少 12 字符；图片最多 10 MiB；文件夹最多三级；权限由服务端鉴权；正文仅允许受限 JSON。
- 保留旧浏览器原型数据；生产采用 PostgreSQL 和 Secure Cookie。

## Review Focus

- 并发消费同一邀请应只有一次成功（任务 1）。
- 不合法正文、跨用户图片、伪装图片不能落库（任务 2）。
- Cookie 写请求缺失 CSRF、只读/撤销 Token 不得写入（任务 1、2）。
- 同版本并发修改不可双成功，失败时不丢草稿（任务 2、3）。
- 快速输入、页面切换、登录切换不可覆盖较新内容或串用草稿（任务 3）。

## Task 1: 账户、数据库和会话

Files: notes-server/app/{config,db,models,security,auth,cli,main}.py、requirements.txt、migrations、tests/test_auth.py。

Interfaces: create_app(settings) → FastAPI；get_db → transaction Session；认证上下文含 user、session/token、scopes；/me 返回 id、email、display_name、csrf_token。

- [x] 先写邀请注册、登录、CSRF、退出/禁用、非管理员 CLI 防护等 API 测试；确认缺实现失败。
- [x] 实现 SQLAlchemy 模型、迁移、CLI 初始邀请、密码与会话、统一错误响应。
- [x] 运行账户测试，确认失效凭证和邀请不可重用。

## Task 2: 用户隔离的笔记 API

Files: notes-server/app/{content,folders,notes,attachments,tokens}.py、tests/test_notes.py。

Consumes: Task 1 身份和事务。Produces: 方案第 6 节 API，notes 元数据与 JSON 详情，图片认证 URL，expected_version 更新。

- [x] 先写双用户隔离、三级/重名、JSON 节点、版本竞争、搜索分页、回收站、图片真实类型/配额/归属、Token 权限撤销测试；确认失败。
- [x] 实现各模块与原子版本更新；会话只管理自己的 Token。
- [x] 运行全部 API 测试，验证数据库重开后数据仍在。

## Task 3: Web 接入与自动保存

Files: notes-web/src/{api,AuthPage,useRemoteLibrary}.ts(x)、App.tsx、NoteEditor.tsx、Settings.tsx、FolderBrowser.tsx、data.ts、vite.config.ts、styles.css。

Consumes: Task 2 API。Produces: 邀请注册、登录、真实列表与编辑、串行保存/草稿恢复、附件上传、Token 管理与退出。

- [x] 浏览器验收覆盖注册、两个用户隔离、刷新、持续输入、上传、冲突和 Token 实际 API 调用。
- [x] 实现前端接口映射和保存队列，保留现有视觉及旧 localStorage。
- [x] 执行 npm run build、后端 pytest、浏览器验收；检查 320/390/430 像素的登录与主要页面。

## Task 4: 运行交付

Files: notes-server/{Dockerfile,compose.yaml,Caddyfile,.env.example,README.md}、根方案状态。

- [x] 提供迁移、邀请、开发和生产启动方法，不设置默认密码。
- [x] 运行 SQLite 迁移及本机联调，记录未执行的 Docker/PostgreSQL/HTTPS 验证。
- [x] 进行一次独立代码审查，修复重要发现并运行覆盖测试；只在证据支持时标记已完成。


## 2026-10-03 验证记录

- 后端 pytest：16 项通过，包含两个用户隔离、邀请并发、Token、图片、版本竞争和数据库重开。测试客户端有一条第三方 httpx 兼容性弃用警告，不影响当前通过结果。
- 前端保存/草稿单元测试：7 项通过；npm run build 通过。
- 实际浏览器：注册、JSON/图片保存、刷新、持续输入、409 冲突与重载、离线草稿恢复/放弃后编辑、重新读取后的版本同步、立即回收、Token 实际权限与撤销、两个用户和 320/390/430 像素页面通过。
- SQLite Alembic 首次迁移及 schema check 通过。独立审查的 5 个行为问题已修复；回收站旧版本问题按用户操作影响作为必须修复处理。
- Docker/PostgreSQL/HTTPS 配置已提供，当前机器无 Docker，尚未执行这些部署验证；这不标记为生产验收通过。
- 第一阶段当时尚未实现 tags/history/Markdown/Skill/幂等/清理/生产运维；标签、Markdown、Skill、幂等已在 [第二阶段](notes-agent-phase-plan.md) 交付，其余仍待后续。旧 localStorage 原型数据保留，无自动导入和永久删除。
