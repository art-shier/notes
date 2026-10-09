# 拾记 REST API 合约

所有路径相对于 API 地址（以 `/api/v1` 结尾）。CLI 优先使用同时提供的 `NOTES_API_URL` + `NOTES_API_TOKEN`，否则自动读取 `~/.shiji-notes/client.json`。命令帮助 `python scripts/notes.py COMMAND --help`。Web 使用独立会话 Cookie 与 CSRF。

## App 授权与本地连接

`login [--server https://notes.shier.art] [--no-browser]` 发起十分钟授权请求。CLI 本地生成强随机 Token，仅发送 SHA256 和前缀；服务端不保存或向浏览器返回完整 Token。用户在 `/#agent-authorize?code=ABCD-EFGH` 核对授权码和账户，选择只读或读写、独立回收站权限、1–365天有效期。Agent 必须等待用户亲自确认。

| 接口 | 鉴权 | 说明 |
|---|---|---|
| POST /auth/agent/request | 无 | `{name, token_hash, token_prefix}`；返回私有 device_code、公开 user_code、verification_uri、expires_in、interval |
| POST /auth/agent/poll | device_code | `{device_code}`；返回 pending / approved / denied / expired / canceled，approved 包含 Token 元数据，无 secret |
| POST /auth/agent/cancel | device_code | 请求有效期内取消；若已批准则撤销此次 Token |
| GET /auth/agent/requests/{code} | Web 会话 | 获取待确认的名称、公开码、状态和到期时间 |
| POST /auth/agent/requests/{code} | Web 会话 + Origin + CSRF | `{approve, access:"read"或"write", allow_trash, expires_days}`；只允许 pending 被处理一次；此接口仅供用户在 App 确认，Agent 不调用 |
| GET /auth/agent/me | Bearer | 当前账户和 Token 的非密钥元数据，用于 whoami 与保存前核验 |
| POST /auth/agent/logout | Bearer | 撤销当前 Token |

CLI 的成功输出不含完整 Token；私有文件由当前用户持有，Unix 700/600 或 Windows 私有 ACL。拒绝、超时、写文件失败和 Ctrl+C 不覆盖旧连接，并尽力取消新请求。服务不可达时去 App 检查是否需撤销。`login --replace` 可以更换连接，旧远端 Token 仍需在 App 撤销。不能只修改服务地址继续使用其他实例的缓存密钥。

服务公开提供 `/agent/SKILL.md`、`/agent/shiji-notes.zip`、`/agent/install-client.py` 和脚本/参考文档；`GET /api/v1/agent-access` 返回同版资源 URL、版本和 SHA256，不返回配置或凭据。安装器 `--server` 从最终同源地址下载并校验 ZIP，拒绝重定向和异常包成员。

## 接口与权限

| 操作 | HTTP | 权限 |
|---|---|---|
| 笔记列表/搜索 | GET /notes?query=&folder_id=&tag_id=&trash=false&favorite=&limit=20&cursor= | notes:read |
| 完整笔记 | GET /notes/{id}?content_format=json 或 markdown | notes:read |
| 正文转换预览 | POST /notes/validate-content | notes:read |
| 创建 | POST /notes + 可选 Idempotency-Key | notes:create |
| 修改/移动/标签关联 | PATCH /notes/{id} | notes:update |
| 按块修改 | POST /notes/{id}/blocks | notes:update |
| 移入回收站 | DELETE /notes/{id}?expected_version=N | notes:trash |
| 恢复 | POST /notes/{id}/restore | notes:trash |
| 历史列表/详情 | GET /notes/{id}/history，GET /notes/{id}/history/{version} | notes:read |
| 恢复旧标题和正文 | POST /notes/{id}/history/{version}/restore | notes:update |
| 准备/列出导出 | POST/GET /exports | 四项读取权限 |
| 下载已准备的ZIP | GET /exports/{id}/download | 四项读取权限 |
| 文件夹列表/创建 | GET/POST /folders | folders:read/write |
| 标签列表/创建 | GET/POST /tags | tags:read/write |
| 标签改名/删除 | PATCH/DELETE /tags/{id} | tags:write |
| 上传/读取图片 | POST /attachments，GET /attachments/{id} | attachments:write/read |

Token 最长 365 天；Web 模板默认 30 天。旧 Token 不自动新增标签权限。所有对象必须属于当前用户，跨用户对象与不存在对象均返回 404。标签关联属于修改笔记；管理标签对象需要 tags:write。

## 笔记

列表只返回元数据：`id, folder_id, title, excerpt, thumbnail, tags, favorite, trashed, source, version, created_at, updated_at`。`folder_id` 包含该文件夹的后代；`trash=false` 默认排除回收站；`query` 搜索标题与正文。每页最多 100，返回 `{items, next_cursor}`。

详情额外返回 `content_json, blocks:[{id,content}], markdown_roundtrip_safe`。指定 `content_format=markdown` 还返回 `markdown`；即使 flag 为 false，仍可读取 Markdown，但这不是无损备份。source 为原始创建来源，不会因后续编辑改写。

创建例子：

```json
{"folder_id":"UUID","title":"随手记","content_format":"markdown","content":"## 想法\n\n正文","tag_ids":[]}
```

修改字段均可选，版本必传：

```json
{"expected_version":3,"title":"新标题","content_format":"json","content":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"正文"}]}]},"tag_ids":["UUID"]}
```

`content_json` 兼容旧客户端，不能与 `content` 同时提交。JSON 是持久化事实来源。创建未传正文则空文档；修改未传正文不改正文。`tag_ids` 是完整替换，`[]` 解除所有标签，省略则不变。返回创建/修改结果包含 `warnings`。

Markdown 支持标题、段落、粗体、斜体、删除线、代码、链接、列表、引用、分隔线、换行及内部图片。HTML 不执行；表格、待办语法保留为文字并警告。图片示例 `![说明](attachment://UUID)`。不支持远程图片 URL。图片在独立块中；尺寸和下划线等无法无损转成 Markdown 的正文不允许用 Markdown 全文覆盖（422 unsafe_markdown_overwrite）。

JSON 节点：doc 下含 paragraph、heading(level1–6)、bulletList、orderedList(start)、blockquote、codeBlock(language)、image、horizontalRule。列表含 listItem（首节点必须是 paragraph，再跟其他块；Markdown 标题/图片开头的列表项会补空首段并警告）；段落/标题含 text、hardBreak。text 支持 marks:bold/italic/strike/underline/code/link(href)。image attrs 含 attachment_id、alt、title、可选 width/height。GET 的 src 是展示地址，存储和权限由 attachment_id 决定。正文最大 1 MiB、10000 节点、深度32；图片 JPEG/PNG/WebP/GIF，最多10 MiB。

## 按块编辑

块 ID 对应顶层块。GET 读取 ID 后提交原子批次（1–100 操作），任一步失败全部不保存：

```json
{
  "expected_version":3,
  "operations":[
    {"op":"replace","block_id":"现有块UUID","content":{"type":"paragraph","content":[{"type":"text","text":"新正文"}]}},
    {"op":"insert","after_id":"现有块UUID","content":{"type":"paragraph","content":[{"type":"text","text":"追加"}]}},
    {"op":"delete","block_id":"另一个现有块UUID"}
  ]
}
```

CLI `--operations-file` 只放 operations 数组，版本从参数提供。`after_id:null` 插入开头；其他 after_id 必须在当前批次中存在。replace 保留目标 ID，insert 生成新 ID。块内嵌套编辑通过替换整个顶层块完成，保留其他块。网页全文更新按内容顺序尽量保持现有块 ID；段落合并/拆分后应重新 GET。

## 文件夹、标签、图片

`folders create "名称" --parent UUID`，文件夹最多三级，同层不能重名。标签名称1–40字符，同用户唯一。`tags create "名称"`、`tags rename UUID "新名称"`、`tags delete UUID`。删除标签只解除关联，相关笔记版本增加；改名不增加笔记版本。

`upload-image image.png` 返回 `{id,url,...}`；创建笔记时引用这个 id。`download-image UUID --output image.png` 使用授权请求下载，不覆盖已有文件。不要把 Token 放到展示图片 URL 中。

## 历史版本和图文导出

历史列表参数 limit 默认20、最大100，before_version返回更早版本。响应含 items、next_before_version、current_version、retention_limit。每条含 version、title、excerpt、action、actor(web/agent/system)、saved_at、restored_from。详情含 snapshot：当时的标题、JSON正文、block_ids、文件夹、标签、收藏、回收状态和原创建来源。历史读取不需要附件权限，但实际图片读取仍检查 attachments:read。

成功的笔记写入与快照在同一事务。失败、版本冲突、幂等重放不新增历史。默认每篇保留最近200条已保存版本（服务器可配置2–2000）；旧笔记升级时只保留当前基线。恢复请求为 `{ "expected_version":5 }`，从指定历史版本恢复标题、JSON正文、块ID；其他设置沿用当前笔记。回收站笔记需要先用原恢复接口恢复，历史恢复不扩大 notes:trash 权限。恢复后版本递增，恢复前正文仍在保留范围内。缺失历史图片、跨用户引用或旧版本已被清理时失败，不部分恢复。

导出需要 notes:read、folders:read、tags:read、attachments:read。POST /exports 的 JSON 为 `{ "include_trash":true, "include_history":true }`，默认两项都包含。返回 id、ready、download_url、size_bytes、expires_at、counts。GET /exports 可找回当前用户未过期任务，包含仍在准备的任务；ready=true后可下载。客户端从UUID构造同源下载路径，不跟随其他URL。

ZIP含 library.json（schema_version2，folders/tags/notes）、notes/{UUID}.html（可离线图文阅读）、index.html、history/{noteUUID}/{version}.json、attachments/{UUID}.{格式}、manifest.json。原始JSON是无损数据；manifest列出其他文件的大小和SHA256，不包含自身哈希。图片去重，包含历史时收集历史独占图片。缺图、越权引用、正文异常和大小超限都会取消导出，不交付缺图包。只导出当前用户数据，不包含密码、Token、会话或邀请；不等同于整个服务备份，也没有自动导入接口。

生成按同一数据库读取快照保持一致。默认未压缩总内容上限2GiB（服务器可配置），每用户最多2个未过期任务。准备成功后保留15分钟；expired410，之后清理再访问可能404。构建预留最长1小时，失败即时清理，过期/崩溃残留由定期清理处理。准备请求等待最多120秒，下载连接等待60秒；长时间生成超时先查 exports，再用 `export --bundle UUID --output new.zip` 下载。大包逐块写入，不受普通JSON响应12MiB限制，失败删除不完整文件，不覆盖已有文件。

## 重试与错误

幂等键为8–128个 ASCII 字母、数字或 `_.:-`，绑定用户+Token/会话+创建路由，保留24小时。同键同请求返回原201快照，即便笔记后来已编辑；同键不同请求409 idempotency_conflict。失效键可能创建新笔记，超过24小时须先查询确认。创建失败不占用键。

客户端最多3次尝试，仅 GET 和带键的笔记创建可重试；限429/502/503/504或连接故障，Retry-After 最多等待5秒。其他写入发生服务或连接错误可能结果不确定，先 GET，不自动重发。自动生成键会在成功或失败 JSON 中返回；跨进程使用同一键与原请求参数重试。

服务错误格式：`{"error":{"code":"version_conflict","message":"...","details":{"current_version":4}}}`。401 检查失效/撤销凭证；403 缺权限；404 无对象或非本用户；409 版本或幂等冲突；413 超限；422 输入/正文错误。409 不能自动改 expected_version 重试，应重新读取、比较并合并。Token 不可用于管理 Token 或注册账户。
