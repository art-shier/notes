#!/usr/bin/env bash
# Linux installer for https://github.com/art-shier/notes
set -Eeuo pipefail
umask 077
REPO=https://github.com/art-shier/notes.git
DOMAIN='' ADMIN_EMAIL='' INSTALL_DIR="${HOME}/notes" PROJECT=shiji
lock='' env_tmp='' askpass=''
say() { printf '\n[拾记] %s\n' "$*"; }
die() { printf '\n[拾记] %s\n' "$*" >&2; exit 1; }
usage() {
  cat <<'HELP'
Usage: bash install.sh --domain notes.example.com --email admin@example.com [--dir /path/notes] [--project shiji]
Linux server; domain must resolve to this server, TCP 80/443 must be available.
Existing configuration and persistent Docker volumes are preserved.
HELP
}
cleanup() {
  local status=$?
  [[ -z $env_tmp ]] || rm -f -- "$env_tmp"
  [[ -z $askpass ]] || rm -f -- "$askpass"
  [[ -z $lock ]] || rmdir -- "$lock" 2>/dev/null || true
  if ((status)); then printf '\n[拾记] 部署中断；已有数据和容器保留。修复错误后可再次执行相同命令。\n' >&2; fi
}
trap cleanup EXIT
while (($#)); do
  case $1 in
    --domain|--email|--dir|--project)
      (($#>=2)) || die "$1 缺少参数。"
      case $1 in --domain) DOMAIN=$2;; --email) ADMIN_EMAIL=$2;; --dir) INSTALL_DIR=$2;; --project) PROJECT=$2;; esac
      shift 2;;
    -h|--help) usage; exit 0;;
    *) usage >&2; die "未知参数：$1";;
  esac
done
[[ $(uname -s) == Linux ]] || die '一键部署面向 Linux 服务器。'
[[ ${#DOMAIN} -le 253 && $DOMAIN =~ ^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$ ]] || die '--domain 需要有效域名，不带协议、端口或路径。'
DOMAIN=${DOMAIN,,}
[[ ${#ADMIN_EMAIL} -le 254 && $ADMIN_EMAIL =~ ^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$ ]] || die '--email 需要有效管理员邮箱。'
[[ $PROJECT =~ ^[a-z0-9][a-z0-9_-]{0,62}$ ]] || die '--project 只能使用小写字母、数字、下划线与短横线。'
[[ -n $INSTALL_DIR && ! -L $INSTALL_DIR ]] || die '安装目录不能为空或为符号链接。'
INSTALL_DIR=$(realpath -m -- "$INSTALL_DIR")
[[ $INSTALL_DIR != / && $INSTALL_DIR != "$HOME" ]] || die '请使用独立项目子目录。'

PRIV=()
privilege() {
  if ((EUID!=0)); then
    command -v sudo >/dev/null || die '安装系统依赖需要 root 或 sudo。'
    PRIV=(sudo)
  fi
}
apt_install() {
  command -v apt-get >/dev/null || die '自动安装依赖支持 Ubuntu/Debian；其他发行版请先安装 Git、Docker 和 Compose v2。'
  privilege
  "${PRIV[@]}" apt-get update
  "${PRIV[@]}" apt-get install -y "$@"
}
command -v curl >/dev/null || apt_install ca-certificates curl
command -v git >/dev/null || apt_install git

if [[ -d $INSTALL_DIR/.git ]]; then
  [[ $(git -C "$INSTALL_DIR" remote get-url origin) == "$REPO" ]] || die '已有目录的 Git 远程不是拾记仓库；请指定新的 --dir。'
  say "使用已有项目：$INSTALL_DIR（不自动更新代码）"
elif [[ -e $INSTALL_DIR ]]; then
  die '安装目录已存在且不是本项目仓库；请指定新的 --dir。'
else
  say "下载项目到 $INSTALL_DIR"
  mkdir -p -- "$(dirname -- "$INSTALL_DIR")"
  if [[ -n ${GITHUB_TOKEN:-} ]]; then
    askpass=$(mktemp)
    cat > "$askpass" <<'ASKPASS'
#!/usr/bin/env sh
case "$1" in
  *Username*) printf '%s\n' x-access-token;;
  *Password*) printf '%s\n' "$GITHUB_TOKEN";;
  *) exit 1;;
esac
ASKPASS
    chmod 700 -- "$askpass"
    GIT_ASKPASS="$askpass" GIT_TERMINAL_PROMPT=0 git -c credential.helper= clone --depth 1 --branch main "$REPO" "$INSTALL_DIR"
    rm -f -- "$askpass"
    askpass=''
  else
    git clone --depth 1 --branch main "$REPO" "$INSTALL_DIR"
  fi
fi
SERVER_DIR="$INSTALL_DIR/notes-server-go"
[[ -f $SERVER_DIR/compose.yaml ]] || die '项目不完整：缺少 notes-server-go/compose.yaml。'
mkdir -- "$SERVER_DIR/.install.lock" 2>/dev/null || die '另一个安装进程正在运行；异常退出后请先确认再移除 .install.lock 目录。'
lock="$SERVER_DIR/.install.lock"

if ! command -v docker >/dev/null; then
  say '安装 Docker Engine 与 Compose v2（需要管理员权限）'
  privilege
  command -v apt-get >/dev/null || die '此发行版请先安装 Docker Engine 与 Compose v2，然后重新执行。'
  docker_setup=$(mktemp)
  if ! curl --fail --silent --show-error --proto '=https' https://get.docker.com -o "$docker_setup"; then rm -f -- "$docker_setup"; die 'Docker 安装脚本下载失败。'; fi
  "${PRIV[@]}" sh "$docker_setup"
  rm -f -- "$docker_setup"
  "${PRIV[@]}" systemctl enable --now docker
fi
DOCKER=(docker)
if ! docker info >/dev/null 2>&1; then
  privilege
  DOCKER=("${PRIV[@]}" docker)
  "${DOCKER[@]}" info >/dev/null 2>&1 || die 'Docker 不可用；请检查 daemon 状态和权限。'
fi
if ! "${DOCKER[@]}" compose version >/dev/null 2>&1; then
  apt_install docker-compose-plugin
  "${DOCKER[@]}" compose version >/dev/null 2>&1 || die '需要 Docker Compose v2 插件。'
fi

ENV_FILE="$SERVER_DIR/.env"
if [[ -e $ENV_FILE || -L $ENV_FILE ]]; then
  [[ -f $ENV_FILE && ! -L $ENV_FILE ]] || die '.env 不是安全的常规文件。'
  configured_domain=$(sed -n 's/^DOMAIN=//p' "$ENV_FILE")
  [[ $configured_domain == "$DOMAIN" ]] || die '指定域名与已有 .env 不一致，未覆盖配置。请手动核对后再部署。'
  say '保留已有 .env 和数据库密码'
else
  password=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
  [[ ${#password} == 64 ]] || die '随机密码生成失败。'
  env_tmp=$(mktemp "$SERVER_DIR/.env.XXXXXX")
  printf 'DOMAIN=%s\nPOSTGRES_PASSWORD=%s\nCOMPOSE_PROJECT_NAME=%s\nNOTE_HISTORY_LIMIT=200\nEXPORT_LIMIT_BYTES=2147483648\n' "$DOMAIN" "$password" "$PROJECT" > "$env_tmp"
  chmod 600 -- "$env_tmp"
  mv -- "$env_tmp" "$ENV_FILE"
  env_tmp=''
  unset password
fi
COMPOSE=("${DOCKER[@]}" compose --project-directory "$SERVER_DIR" --env-file "$ENV_FILE")
"${COMPOSE[@]}" config --quiet
say '构建 Web 与 Go 服务；首次构建可能需要数分钟'
"${COMPOSE[@]}" build --pull app
say '启动 PostgreSQL、Go 服务与 HTTPS 入口'
"${COMPOSE[@]}" up -d --wait --wait-timeout 180
say "等待 HTTPS 就绪：https://$DOMAIN"
health=$(curl --fail --silent --show-error --proto '=https' --retry 20 --retry-delay 3 --retry-all-errors --max-time 10 "https://$DOMAIN/api/v1/health/ready") || die 'HTTPS 未就绪；请检查域名解析、80/443端口和 Caddy 日志。服务与数据保留。'
[[ $health == '{"status":"ready"}' ]] || die 'HTTPS 返回内容不是拾记就绪响应。'
state=$("${COMPOSE[@]}" exec -T db psql -U notes -d notes -At -c "SELECT CASE WHEN EXISTS(SELECT 1 FROM users) THEN 'registered' WHEN EXISTS(SELECT 1 FROM invitations WHERE role='admin' AND used_at IS NULL AND expires_at > CURRENT_TIMESTAMP AT TIME ZONE 'UTC') THEN 'pending' ELSE 'empty' END;")
state=${state//$'\r'/}
INVITE_FILE="$SERVER_DIR/.bootstrap-invite"
[[ ! -L $INVITE_FILE ]] || die '邀请备份路径为符号链接，未写入或读取。'
case $state in
  empty)
    invite=$("${COMPOSE[@]}" exec -T app shiji bootstrap --email "$ADMIN_EMAIL")
    printf '%s\n' "$invite" > "$INVITE_FILE"
    chmod 600 -- "$INVITE_FILE"
    say '部署完成。打开下方邀请链接，自行设置管理员密码（24小时有效）：'
    printf '%s\n' "$invite";;
  pending)
    say '服务已启动，初始管理员邀请仍有效。'
    if [[ -f $INVITE_FILE && ! -L $INVITE_FILE ]]; then cat -- "$INVITE_FILE"; else say '邀请链接不在本机；使用原邀请链接，或在24小时过期后重新执行。'; fi;;
  registered)
    say '部署完成，已有账户，未创建新管理员或改动账号。'
    rm -f -- "$INVITE_FILE";;
  *) die '无法判断账户初始化状态，未创建管理员邀请。';;
esac
say "访问地址：https://$DOMAIN；配置目录：$SERVER_DIR"
say '数据保存在 Docker 持久卷中。更新/备份操作见 README；不要执行 docker compose down -v。'
