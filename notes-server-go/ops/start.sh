#!/usr/bin/env bash
# Restart standalone deployments from their existing private configuration.
set -Eeuo pipefail
umask 077
server_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
cd "$server_dir"
unset DATABASE_URL DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME DB_SSLMODE DOMAIN COMPOSE_FILE COMPOSE_PROJECT_NAME COMPOSE_ENV_FILES COMPOSE_PROFILES POSTGRES_PASSWORD NOTE_HISTORY_LIMIT EXPORT_LIMIT_BYTES
die() { printf '[拾记] %s\n' "$*" >&2; exit 1; }
(($#==0)) || die '启动不再读取ConfigHub；请在ctl配置数据库，或使用现有私有.env。'
[[ -f .env && ! -L .env && $(stat -c %u .env) == $(id -u) ]] || die '需要当前用户拥有的常规.env。'
mode=$(stat -c %a .env)
(( (8#$mode & 077) == 0 )) || die '.env必须禁止组和其他用户读取。'
mkdir -- .install.lock 2>/dev/null || die '另一个安装/启动正在运行。'
trap 'rmdir -- .install.lock' EXIT
DOCKER=(docker)
if ! docker info >/dev/null 2>&1; then
  if ((EUID!=0)); then DOCKER=(sudo docker); fi
  "${DOCKER[@]}" info >/dev/null 2>&1 || die 'Docker不可用。'
fi
COMPOSE=("${DOCKER[@]}" compose --project-directory "$server_dir" --env-file "$server_dir/.env")
"${COMPOSE[@]}" config --quiet
"${COMPOSE[@]}" build --pull app
services=$("${COMPOSE[@]}" config --services)
if grep -qx 'db' <<< "$services"; then
  # The bundled database may be stopped after a reboot or compose down.
  # Bring up only that dependency before the no-deps readonly app check.
  "${COMPOSE[@]}" up -d --wait --wait-timeout 180 db
fi
"${COMPOSE[@]}" run --rm --no-deps --entrypoint shiji app database-check || die '数据库预检失败，已有服务和配置保留。'
"${COMPOSE[@]}" up -d --wait --wait-timeout 180
