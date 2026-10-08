# Notes：deployctl 部署

ctl1.7.0管理台/托管Registry接入见 [平台模式](CONTROL-PLANE.md)；本页保留原发布包与ConfigHub安装入口。

application为notes，默认环境prod，主机127.0.0.1:8000，就绪接口 `/api/v1/health/ready`。默认域名为 `notes.shier.art`，新版用 `--set DOMAIN=域名` 覆盖。Go/Web使用现有Dockerfile；PostgreSQL与HTTPS代理外置。

## 新版自动配置流程（待发布）

不用填写DATABASE_URL、用户名、密码或提前下载prepare脚本。服务器需Linux、Python>=3.10、Docker Engine、Compose>=2.30、jq、root所有且不可被普通用户写入的ConfigHub CLI；Token文件默认 `/root/shier-prod.token`，root所有、权限600。沿用已有CLI凭据时显式传 `--set TOKEN_FILE=`。

新包要求ctl>=1.6.0，v1.5.0不支持pre后配置回读。以下命令仅在新ctl/Notes版本发布后，使用Actions给出的真实RELEASE_URL与SHA256：

```bash
sudo deployctl install notes --env prod --release "$RELEASE_URL" --sha256 "$SHA256"
# 可选：指定私有token文件、自定义域名和首个管理员邮箱
sudo deployctl install notes --env prod --release "$RELEASE_URL" --sha256 "$SHA256" \
  --set TOKEN_FILE=/root/shier-prod.token --set DOMAIN=notes.shier.art --set ADMIN_EMAIL=you@example.com
```

以上两个install是替代示例；已有安装用upgrade，同版本也可重新拉取配置。安装参数只作用于本次，使用自定义参数时每次升级都需传相同值。默认ConfigHub是 `https://config.shier.art` 的 `shier/prod`，数据库notes；需要覆盖时用 `--set CONFIG_HUB_URL=...`、CONFIG_HUB_PROJECT、CONFIG_HUB_ENV、DATABASE_NAME或CLI_BINARY。

执行顺序：验证标准包与镜像 → pre读取ConfigHub、字段校验/URI编码、只读数据库检查 → 写私有config.env/secrets.env → ctl回读并创建新运行快照 → Go迁移/启动/就绪检查 → post查询账户状态 → 提交成功。新镜像缺失DATABASE_URL会停止启动，避免悄悄使用SQLite。

主机/端口优先notes_db_address/notes_db_port，回退db_address/db_port；账号优先notes_db_username/notes_db_password，回退共享字段。不会重复手工配置地址和端口。仅连接已创建的notes库，不创建数据库/账号。配置来源、域名及数据库目标固定，密码可经预检更新；读取失败、预检失败或部分写入失败保留原配置。

`--set DOMAIN`会生成对应APP_ORIGIN；已有显式env-var覆盖优先，修改域名还需调整或移除旧APP_ORIGIN override。数据库URL保存在服务器600的secrets.env与受保护的运行快照，不进入默认配置、包或普通日志。Token仅供宿主机pre读取，不注入应用。

post只在账户状态empty且传入ADMIN_EMAIL时生成邀请，pending/registered保持现状。邀请仅在服务器600的hook日志中，可由管理员读取 `/etc/deployctl/notes/prod/hook-logs/`；不自动回显到部署命令。

pre失败不替换旧服务；启动/就绪/post失败恢复旧成功快照和版本。回滚不重跑hooks，也不撤销数据库迁移、邀请或原始配置文件变更。restart使用上次成功快照；要重新读取ConfigHub使用upgrade。

ctl和Notes新版尚未发布；下面的v0.2.0步骤仍是旧版本的真实可用方式。

## 已发布版本与后续发布

notes自己的 `.github/workflows/release.yml` 完成测试、双架构镜像构建与标准包生成。新版构建与CI使用工作流中固定的配置回读平台源码SHA，无需 `PLATFORM_READ_TOKEN`。新版默认发布到 `https://ctl.shier.art`，镜像仓库默认 `ctl.shier.art`；GitHub Variables中的 `CTL_SERVER_URL`、`CTL_REGISTRY_HOST` 可覆盖这两个值，未配置或为空时分别使用默认值。推送托管镜像与登记版本使用必填的 `CTL_PUBLISH_TOKEN`，GitHub Release下载入口继续使用Actions提供的 `GITHUB_TOKEN`。

v0.2.0已发布，标准包内四文件由deployctl生成。发布提交为 `35ff869f089c11db4ba02f0b818f9641b0169d35`；随后单独发布配置工具，源提交 `a32f7574bc60d9fac3469c0a7974a83c2f4b1668`，包括默认域名修改。配置工具为Release附加资产，与标准包分开，不手改Compose。

新版源码使用自包含pre/post hook，安装时自动生成数据库连接，required_config为空。它依赖待发布的ctl>=1.6.0配置回读能力；旧v0.2.0保持不变。

| 产物 | 地址 / 摘要 |
|---|---|
| 标准包 | [notes-v0.2.0.tar.gz](https://github.com/art-shier/notes/releases/download/v0.2.0/notes-v0.2.0.tar.gz) |
| 标准包SHA256 | `e58726a47620d97b5f5eb5c4432e58a8ad498499153c289bf29581a330f8622a` |
| 镜像 | `ghcr.io/art-shier/notes@sha256:d569f05d9f00890335e40b38922d70cf754a1ee3ffae5ec96343ab0af59f8373` |
| 配置工具 | [notes-config-a32f757.tar.gz](https://github.com/art-shier/notes/releases/download/v0.2.0/notes-config-a32f757.tar.gz) |
| 配置工具SHA256 | `9f86961dcd9f385be4e794f82eeebb1c3738ea0e0b09db5a39fc494593d758d7` |

后续新增v*标签或手动选择已有未发布标签可触发发布，需先取得相应发布授权。发布成功后在Actions摘要查看真实image digest、包URL与SHA256。配置工具是本次单独打包上传的附加文件，后续更新需从相应受检源码生成并校验，不能覆盖同名产物。

原生工作流改为 `release-native.yml` 手动触发，可为已有Release追加原生资产，不覆盖已有文件；原生installer只选择含匹配运行包的稳定版本。

## v0.2.0 服务器与配置（旧流程）

需要Linux、Python>=3.10、deployctl>=1.2、Docker Engine、Compose>=2.30、jq、ConfigHub CLI。HTTPS由现有Nginx/Caddy代理至127.0.0.1:8000。代码仓库公开不代表GHCR镜像公开：服务器需要镜像拉取权限，或发布者将镜像设为公开。

准备root所有、权限600的 `/root/shier-prod.token`，数据库沿用ConfigHub的shier/prod与notes专用账号。无需clone源码：在Bash中下载已发布的小配置包，校验后解压到独立root目录，再生成配置：

```bash
set -euo pipefail
curl -fL --proto '=https' https://github.com/art-shier/notes/releases/download/v0.2.0/notes-config-a32f757.tar.gz -o notes-config-a32f757.tar.gz
echo '9f86961dcd9f385be4e794f82eeebb1c3738ea0e0b09db5a39fc494593d758d7  notes-config-a32f757.tar.gz' | sha256sum -c -
sudo mkdir -p /opt/notes-config-a32f757
sudo tar --no-same-owner -xzf notes-config-a32f757.tar.gz -C /opt/notes-config-a32f757
IMAGE='ghcr.io/art-shier/notes@sha256:d569f05d9f00890335e40b38922d70cf754a1ee3ffae5ec96343ab0af59f8373'
sudo bash /opt/notes-config-a32f757/deploy/prepare.sh --image "$IMAGE" --token-file /root/shier-prod.token
# 默认notes.shier.art；其他域名在prepare参数中追加 --domain 域名
```

准备脚本先拉取/校验/URI编码，再用指定镜像执行只读database-check，成功后生成 `/etc/deployctl/notes/prod/config.env`、secrets.env（600、raw、不加shell引号）。使用0.0.0.0:8000及/app、/data容器路径；失败保留原配置，域名/配置来源/数据库目标锁定，密码可经检查更新。它不安装/重启服务、不创建数据库、不输出凭据。

## v0.2.0 安装与运维（旧流程）

v0.2.0的真实包地址和SHA256如下。先查询状态；未安装用install，已安装用upgrade。transaction非空先诊断，不能继续升级：

```bash
RELEASE_URL='https://github.com/art-shier/notes/releases/download/v0.2.0/notes-v0.2.0.tar.gz'
SHA256='e58726a47620d97b5f5eb5c4432e58a8ad498499153c289bf29581a330f8622a'
sudo deployctl status notes --env prod
```

未安装时执行：

```bash
sudo deployctl install notes --env prod --release "$RELEASE_URL" --sha256 "$SHA256"
sudo deployctl status notes --env prod
curl -fsS http://127.0.0.1:8000/api/v1/health/ready
```

已安装且目标版本较旧时，先完整备份、重新prepare，再执行upgrade：

```bash
sudo deployctl upgrade notes --env prod --release "$RELEASE_URL" --sha256 "$SHA256"
sudo deployctl status notes --env prod
curl -fsS http://127.0.0.1:8000/api/v1/health/ready
```

目标版本就绪且transaction为空才是部署成功。镜像启动时执行Go迁移；应用回滚不回滚数据库/配置。原生/旧Compose实例不自动停止或迁移附件，切换前停止所有写入者并保留完整备份。

新库就绪后使用同一image和配置执行CLI，bootstrap-status为empty时才生成管理员邀请（已有账户不重复初始化）：

```bash
sudo docker run --rm --entrypoint shiji --env-file /etc/deployctl/notes/prod/config.env --env-file /etc/deployctl/notes/prod/secrets.env "$IMAGE" bootstrap-status
sudo docker run --rm --entrypoint shiji --env-file /etc/deployctl/notes/prod/config.env --env-file /etc/deployctl/notes/prod/secrets.env "$IMAGE" bootstrap --email you@example.com
```

通过已配置HTTPS域名打开邀请，没有默认密码或公开注册。其他运维通过deployctl status/logs/rollback；回滚在对应授权下执行，不删除state.json。自定义环境/配置根时prepare与deployctl保持相同选项。

## 当前图片边界

用户决定暂缓持久卷/OSS。图片与导出暂存容器可写层：同一容器restart通常保留，升级/重建/回滚重建可能丢失，数据库记录不包含原图片文件。当前适合验证文字笔记/API及临时图片；重要图片继续保留原部署备份，后续再接OSS。尚未操作生产服务器。
