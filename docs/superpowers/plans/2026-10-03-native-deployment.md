# 原生部署实施计划

> **For agentic workers:** Use superpowers:executing-plans inline; review the full change at the end.

**Goal:** 提供不依赖 Docker 的一键 Linux 部署，保留 ConfigHub 拉取、私有配置和数据库预检流程。
**Architecture:** 共用 Bash/jq 数据库读取库；root 配置 oneshot + 非 root Go systemd 单元；下载已构建 Release、验证 SHA-256、发布版本目录和 current；输出已有反向代理的配置片段。
**Tech Stack:** Bash、jq、Go、React/Vite、systemd、GitHub Release、PostgreSQL16。
**Spec:** ../specs/2026-10-03-native-deployment.md

## Constraints / review focus

- 密码和 Token 不进入 Git/日志；只读预检失败保留已有 env 和正在运行服务。
- 不更改生产库、不覆盖其他站点/账号/非本项目路径。
- 特殊字符 URI 编码、环境覆盖隔离、固定目标、symlink 与路径保护。
- systemd 权限、开机配置刷新、PID/配置在失败时保留、安装升级和依赖可用性。
- SHA校验、平台架构、运行包内容、版本切换失败时旧版本保留。

## Task 1: 共用配置读取与原生运维

Files: ops/config-hub-lib.sh; ops/native/{prepare,start,admin}.sh; tests/native_config_test.py; existing ConfigHub regression.

- [x] 先写原生 prepare 缺失的失败测试；覆盖原生 env、专用/共享字段、编码、失败保留、目标锁定。
- [x] 实现共用读取函数，Docker start 复用，更新其测试fixture。
- [x] 实现原生 prepare/start/admin；runuser 进行预检，私有 env 原子更新，不执行配置文本。
- [x] 运行 Bash语法、原 Docker回归和原生回归，通过后记录证据。

## Task 2: 运行包、一键安装及 systemd

Files: ops/build-native.sh; install-native.sh; native unit templates; tests/native_install_test.py; .github/workflows/release.yml.

- [x] 先补缺少安装器、坏checksum、已有路径和参数保护的失败测试。
- [x] 构建静态 amd64/arm64 Go + Web 包及SHA；Release工作流发布。
- [x] 安装器验证平台、包、配置、独立路径，准备 OS账号/私有配置/data、单元、反向代理片段，检查本机就绪并创建管理员邀请。
- [x] systemd 首次启动/开机拉取；原生刷新先预检再重启；现有业务站点不自动更改。
- [x] 运行安装器回归及真实运行包构建验证。

## Task 3: Linux 原生真实集成与发布

Files: tests/native_smoke.py; .github/workflows/ci.yml; README.md; server README; deployment guide.

- [x] 用临时TLS PG、真实 systemd/非root账号/发行包验证Go和Web、初始化状态、失败刷新保留、停止后重新启动时拉取。
- [x] 文档给出一行原生安装、域名入口、Token/数据库前提、运维/备份、Docker区分和升级保护。
- [x] 独立审查并修复重要问题；合入main并等待完整CI。
- [x] 创建并验证首个稳定Release，校验公开运行包及安装入口，更新实施证据。


本地证据：Go test/vet、Web 7项测试、原生配置/安装及共享Docker配置回归通过；双架构运行包构建与SHA/ELF检查通过。完整Linux systemd与TLS PG16验证已通过，未接触生产库。独立审查发现并修复发布回退、root目录祖先/CLI权限、片段链接保护、自定义单元目录及首次预检失败重试问题。


发布证据：

- 完整CI（代码60c8be1）：https://github.com/art-shier/notes/actions/runs/37130540349
- 真实systemd检查包含首次Token路径错误重试、非root Go/Web、TLS、600配置、重复安装、自定义unit目录、不安全目录/片段链接拒绝、失败升级回退、失败刷新保留PID/env及模拟开机拉取失败后恢复。
- 稳定Release v0.1.0：https://github.com/art-shier/notes/releases/tag/v0.1.0；发布工作流：https://github.com/art-shier/notes/actions/runs/37130820558
- 未登录下载验证两个运行包：SHA-256、版本、ELF amd64/arm64、Web及全部原生脚本与已验证源码一致；公开main安装入口与源码一致。
- 初轮CI的权限拒绝仅发生在共享宿主机目录；测试改用专用root子目录，保留生产祖先目录保护。没有执行生产服务器部署、生产库修改或本地预览迁移。
