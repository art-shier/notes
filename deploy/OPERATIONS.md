# Notes：deployctl 部署

application为notes，默认环境prod，主机127.0.0.1:8000，就绪接口 `/api/v1/health/ready`。默认域名为 `notes.shier.art`，可通过 `--domain` 覆盖。Go/Web使用现有Dockerfile；PostgreSQL与HTTPS代理外置。

## 已发布版本与后续发布

notes自己的 `.github/workflows/release.yml` 完成测试、双架构镜像构建与标准包生成。平台工具固定到公开deployctl仓库的v1.2.0提交 `2d3a014a7b5015051a871664c1f4c2cfab2a6566`，无需 `PLATFORM_READ_TOKEN`。镜像与Release发布使用GitHub Actions自动提供的 `GITHUB_TOKEN`。

v0.2.0已发布，标准包内四文件由deployctl生成。发布提交为 `35ff869f089c11db4ba02f0b818f9641b0169d35`；随后单独发布配置工具，源提交 `a32f7574bc60d9fac3469c0a7974a83c2f4b1668`，包括默认域名修改。配置工具为Release附加资产，与标准包分开，不手改Compose。

| 产物 | 地址 / 摘要 |
|---|---|
| 标准包 | [notes-v0.2.0.tar.gz](https://github.com/art-shier/notes/releases/download/v0.2.0/notes-v0.2.0.tar.gz) |
| 标准包SHA256 | `e58726a47620d97b5f5eb5c4432e58a8ad498499153c289bf29581a330f8622a` |
| 镜像 | `ghcr.io/art-shier/notes@sha256:d569f05d9f00890335e40b38922d70cf754a1ee3ffae5ec96343ab0af59f8373` |
| 配置工具 | [notes-config-a32f757.tar.gz](https://github.com/art-shier/notes/releases/download/v0.2.0/notes-config-a32f757.tar.gz) |
| 配置工具SHA256 | `9f86961dcd9f385be4e794f82eeebb1c3738ea0e0b09db5a39fc494593d758d7` |

后续新增v*标签或手动选择已有未发布标签可触发发布，需先取得相应发布授权。发布成功后在Actions摘要查看真实image digest、包URL与SHA256。配置工具是本次单独打包上传的附加文件，后续更新需从相应受检源码生成并校验，不能覆盖同名产物。

原生工作流改为 `release-native.yml` 手动触发，可为已有Release追加原生资产，不覆盖已有文件；原生installer只选择含匹配运行包的稳定版本。

## 服务器与配置

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

## 安装与运维

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
