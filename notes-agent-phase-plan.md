# 拾记第二阶段：Agent 接入与标签

日期：2026-10-03；用户已要求继续开发。沿用 [技术方案](notes-technical-plan.md) 的下一阶段，不引入 MCP，不开启公开注册，不更改导航。

## 本轮合约

1. JSON 仍是正文事实来源。创建/更新兼容旧 content_json，新增 content_format=json|markdown 与 content；不能同时提交两份正文。Markdown 图像只接受 attachment://UUID，不抓取外部图片。返回转换 warnings；GET 详情提供 markdown_roundtrip_safe。不能无损往返的原正文禁止 Markdown 全文覆盖，返回 422，改用 JSON 或按块接口。
2. 顶层块 ID 存在笔记的 block_ids 元数据，不塞进 Tiptap 不认识的属性。已有笔记迁移补齐 ID；未改变的块保持 ID，按块替换保留目标 ID。网页重新分段时按内容顺序匹配并补新 ID。GET 返回 blocks；POST /notes/{id}/blocks 用 expected_version 原子执行 insert/replace/delete；任一步失败全部回滚。insert 支持 after_id（null 为开头），其他操作要求现有 block_id。按块接口只接受受限 JSON 节点。
3. 标签属于用户，名称同用户唯一；GET/POST/PATCH/DELETE /tags。笔记创建/修改接受 tag_ids，响应包含 tags，列表支持 tag_id。引用其他用户标签返回 404。删除标签只解除关联，不删除笔记；相关笔记版本递增。编辑页支持选择、移除和创建标签，不新增导航页。
4. POST /notes 接受 Idempotency-Key（8–128 个 ASCII 字母/数字/_.:-）。键绑定用户、Token（网页为会话）、路由，保留 24 小时；相同规范化请求重放原 201 响应，不同请求 409。唯一约束和同一事务保证并发不重复创建；失败不留下空记录。默认网页也发送键，CLI 重试使用相同键。
5. 新增 tags:read、tags:write 权限。现有 Token 不自动扩大权限；新只读/读写模板包含相应标签权限。读写默认仍不包含 notes:trash，可显式勾选回收站权限。
6. Skill 包位于 outputs/notes-skill/shiji-notes，包含 SKILL.md、references/api.md、scripts/notes.py。使用环境变量 NOTES_API_URL、NOTES_API_TOKEN；HTTPS 默认，显式 NOTES_ALLOW_LOCAL_HTTP=1 才允许 localhost 开发 HTTP。拒绝重定向，防止携带密钥访问新地址。密钥不进入命令参数和输出。CLI 接受 UTF-8 文件、返回 JSON/非零错误码。GET 与带键创建可有限重试；409 不盲目重试，其他写入结果不确定时先读取。

## 实施顺序与文件

- [x] 数据与 API：models.py、notes.py、content.py、tags.py、markdown.py、blocks.py、idempotency.py 和 Alembic 第二次迁移；先写双用户标签、稳定块 ID、原子修改、Markdown 往返保护、幂等并发测试，再实现。
- [x] Skill：命令涵盖 list/search/get/create/update/move/folders/tags/blocks/validate/upload-image/download-image/trash/restore；先写 URL/凭证/重定向/重试和实际 API 往返测试，再实现并运行 Skill 校验。
- [x] Web：api/data/useRemoteLibrary、编辑页标签组件、Token 模板及可选回收权限；验证标签选择、刷新、搜索和 Agent 写入网页读取。
- [x] 验证：全部后端与保存队列测试、Web 构建、SQLite 迁移/重开、真实浏览器和一次独立代码审查。

## 验收与边界

重点是失败时数据不受损：标签/图片跨用户失败；Markdown 不静默降级；多块操作全有或全无；同键并发只生成一篇；不同键的访问身份不共享响应；Token 撤销立即阻止 Skill；多标签页草稿规则保持有效。

复杂 Markdown 表格、待办等未支持语法会作为文字并明确警告，不声称支持它们的富文本结构。历史版本、完整图文导出、导入、附件回收、备份恢复与生产 Docker/PostgreSQL/HTTPS 验收仍属于后续阶段。

## 完成与验证记录（2026-10-03）

- 后端/API/CLI 共36项测试通过；前端保存队列与草稿7项测试通过；Vite生产构建与Skill格式校验通过。
- SQLite迁移用带旧正文、旧列表、收藏和version7的数据库验证：新增块ID、列表首段规范化、数据保留、schema check及降级均通过。主本地库迁移前通过SQLite backup保存副本；主库无用户，未引入测试账户。
- 隔离的真实浏览器与REST/CLI验证通过：Web标签创建/重复提示/移除/刷新/离线草稿恢复；Agent Markdown创建及网页读取、图片上传/下载、按块改Web保留图片与标签；相同键重放、不同请求冲突；只读与撤销Token；双用户笔记/标签/图片隔离；320/390/430宽度无横向溢出。
- 独立审查发现Markdown列表项以标题/图片开头时不符合Tiptap paragraph-first规范、列表Enter报错。已规范转换并给出警告、拒绝非法JSON/block输入，并在旧数据迁移中规范旧列表；两项回归先失败后通过。真实浏览器标题列表回车保存与独立审查的实际Tiptap schema/图片Enter复核通过。
- Skill包为可复制分发的项目产物，没有自动安装到用户全局skills目录、没有内置凭证。凭证测试仅在隔离环境中进行。
- 本机Docker不可用，PostgreSQL/容器/HTTPS与真实服务器部署未验证；历史版本、完整图文导出/导入、清理与运维仍待后续。
