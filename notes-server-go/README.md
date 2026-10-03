# 拾记 Go 服务端

当前开发与部署入口。模块化单体：Go 1.26 + Gin + GORM；生产 PostgreSQL 16，本地 SQLite。前端保持 React/TypeScript/Tiptap，Agent 使用已有 Skill + REST API。

## 本地运行

在本目录执行（Go 1.26+）：

```powershell
go build -o shiji.exe ./cmd/shiji
$env:DATABASE_URL='sqlite:///data/notes.db'
$env:APP_ORIGIN='http://127.0.0.1:5173'
.\shiji.exe migrate
.\shiji.exe bootstrap --email your-email@example.com
.\shiji.exe serve
```

首次用户通过 CLI 输出的邀请链接自行设置密码。默认 API 监听 127.0.0.1:8000，前端 Vite 5173 已配置代理。邀请其他用户：`shiji invite --admin-email 管理员邮箱 --email 新用户邮箱`。不开放公共注册。

## 配置与部署

环境配置沿用 DATABASE_URL、APP_ORIGIN、COOKIE_SECURE、ATTACHMENTS_DIR、EXPORTS_DIR、NOTE_HISTORY_LIMIT、EXPORT_LIMIT_BYTES；Go 增加 LISTEN_ADDR。数据库 URL 使用 `postgresql://`，服务端无需 Python。

推荐根目录 `install-native.sh` 下载 Linux Go/Web，由 systemd 管理非root服务，通过 ConfigHub 连接已有 PostgreSQL，Nginx/Caddy 提供 HTTPS。原生脚本在 `ops/native`；`ops/build-native.sh` 构建 amd64/arm64 Release。刷新配置执行 `sudo bash /opt/shiji/current/ops/native/start.sh`；普通 `systemctl restart shiji` 复用上次配置。详见上级 [原生部署指南](../notes-native-deployment.md)。

也可从本目录使用 Docker Compose；复制 `.env.example` 为 `.env`，配置域名与随机数据库密码。Caddy 终止 HTTPS，Go 同时提供静态 Web 和 `/api/v1`，数据库及私有附件不公开映射。服务端容器包含 PostgreSQL 16 客户端。详见上级 `notes-deployment-guide.md`。

离线备份：所有实例停止后执行 `shiji backup-create --output 新目录 --app-stopped`；校验 `shiji backup-verify --backup 目录`；指向空的新环境执行 `shiji backup-restore --backup 目录 --app-stopped`。Compose 运维脚本位于 ops；原生通过 `ops/native/admin.sh` 执行管理命令，另需安装 PostgreSQL16客户端。恢复保留密码和 Token，清除会话及临时导出。

## 兼容性

仅直接读取 Alembic head `8942cd035480` 的旧库；更早版本先通过旧实现升级。Go 不改写已有正文、密码哈希及 Token。原 Python 服务保留在 `../notes-server` 作为旧版本参考，迁移和部署以本目录为准。Skill 中的 Python 脚本仍是独立客户端，不属于服务端。

API 字段、Cookie/CSRF、Token scopes、版本检查和幂等创建与旧版兼容。正文以 Tiptap JSON 为事实来源；Markdown 无损往返保护、块操作、私有图片、历史恢复及 ZIP 导出保留。

## 验证

```text
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/shiji
```

SQLite 写入使用单连接，PostgreSQL 写事务锁定所属用户；连接池、密码计算及导出并发受限。图片处理每实例最多同时两项，超出返回429。下载释放数据库连接后流式传输。生产并发容量需要在真实 PostgreSQL、CPU、内存和磁盘环境中压测，不能从语言选型推断。
