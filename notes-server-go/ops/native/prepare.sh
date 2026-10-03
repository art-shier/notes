#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
ops_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
source "$ops_dir/config-hub-lib.sh"
source "$ops_dir/native/lib.sh"
unset DATABASE_URL APP_ORIGIN COOKIE_SECURE LISTEN_ADDR WEB_DIR ATTACHMENTS_DIR EXPORTS_DIR NOTE_HISTORY_LIMIT EXPORT_LIMIT_BYTES
config_dir=/etc/shiji; binary=''; stage=''; lock=''
cleanup() {
  if [[ -n $stage ]]; then
    rm -f -- "$stage/config.json" "$stage/database.json" "$stage/target" "$stage/env"
    rmdir -- "$stage"
  fi
  [[ -z $lock ]] || rmdir -- "$lock"
}
trap cleanup EXIT
while (($#)); do
  (($#>=2)) || native_die '缺少参数值。'
  case $1 in --config-dir) config_dir=$2;; --binary) binary=$2;; *) native_die "未知参数：$1";; esac
  shift 2
done
[[ $(id -u) == 0 ]] || native_die '原生配置刷新需要root。'
native_meta "$config_dir"
for file in service.env .database-target; do
  [[ ! -L $config_dir/$file && ( ! -e $config_dir/$file || -f $config_dir/$file ) ]] || native_die '配置目标不是常规文件。'
done
mkdir -- "$config_dir/.prepare.lock" 2>/dev/null || native_die '另一个配置刷新正在运行。'
lock="$config_dir/.prepare.lock"
stage=$(mktemp -d "$config_dir/.prepare.XXXXXX")
printf '[拾记] 拉取 %s/%s 配置\n' "$hub_project" "$hub_env"
shiji_read_database "$stage" "$hub_project" "$hub_env" "$database" "${CLI[@]}" || native_die 'ConfigHub配置不可用；保留原配置。'
if [[ -f $config_dir/.database-target ]]; then
  cmp -s -- "$config_dir/.database-target" "$stage/target" || native_die '数据库目标变化，拒绝自动切库。'
fi
binary=${binary:-$install_dir/current/shiji}
[[ -x $binary && -f $binary && ! -L $binary ]] || native_die 'Go程序不可用。'
export DATABASE_URL=$(jq -er .uri "$stage/database.json")
export APP_ORIGIN="https://$domain" COOKIE_SECURE=true LISTEN_ADDR="127.0.0.1:$port"
export WEB_DIR="$install_dir/current/web" ATTACHMENTS_DIR="$data_dir/attachments" EXPORTS_DIR="$data_dir/exports"
export NOTE_HISTORY_LIMIT=200 EXPORT_LIMIT_BYTES=2147483648 TZ=UTC
runuser -u "$service_user" -- "$binary" database-check || native_die '数据库预检失败；保留原配置。'
printf 'DATABASE_URL=%s\nAPP_ORIGIN=%s\nCOOKIE_SECURE=true\nLISTEN_ADDR=%s\nWEB_DIR=%s\nATTACHMENTS_DIR=%s\nEXPORTS_DIR=%s\nNOTE_HISTORY_LIMIT=200\nEXPORT_LIMIT_BYTES=2147483648\nTZ=UTC\n' "$DATABASE_URL" "$APP_ORIGIN" "$LISTEN_ADDR" "$WEB_DIR" "$ATTACHMENTS_DIR" "$EXPORTS_DIR" > "$stage/env"
chmod 600 -- "$stage/env" "$stage/target"
mv -- "$stage/target" "$config_dir/.database-target"
mv -- "$stage/env" "$config_dir/service.env"
printf '[拾记] 数据库预检通过，原生配置已更新。\n'
