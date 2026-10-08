#!/usr/bin/env bash
# Validate private native configuration; no remote configuration refresh.
set -Eeuo pipefail
umask 077
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
source "$here/lib.sh"
unset DATABASE_URL DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME DB_SSLMODE APP_ORIGIN COOKIE_SECURE LISTEN_ADDR WEB_DIR ATTACHMENTS_DIR EXPORTS_DIR NOTE_HISTORY_LIMIT EXPORT_LIMIT_BYTES REQUIRE_DATABASE_URL
config_dir=/etc/shiji; binary=''; incoming=''; stage=''; lock=''
cleanup() {
  [[ -z $stage ]] || rm -f -- "$stage"
  [[ -z $lock ]] || rmdir -- "$lock"
}
trap cleanup EXIT
while (($#)); do
  (($#>=2)) || native_die '缺少参数值。'
  case $1 in --config-dir) config_dir=$2;; --binary) binary=$2;; --env-file) incoming=$2;; *) native_die "未知参数：$1";; esac
  shift 2
done
[[ $(id -u) == 0 ]] || native_die '原生配置预检需要root。'
native_meta "$config_dir"
[[ ! -L $config_dir/service.env && ( ! -e $config_dir/service.env || -f $config_dir/service.env ) ]] || native_die '配置目标不是常规文件。'
mkdir -- "$config_dir/.prepare.lock" 2>/dev/null || native_die '另一个配置预检正在运行。'
lock="$config_dir/.prepare.lock"
binary=${binary:-$install_dir/current/shiji}
[[ -x $binary && -f $binary && ! -L $binary ]] || native_die 'Go程序不可用。'
if [[ -n $incoming ]]; then
  native_read_env "$incoming"
  stage=$(mktemp "$config_dir/.prepare.XXXXXX")
  cat -- "$incoming" > "$stage"
  printf '\nAPP_ORIGIN=https://%s\nCOOKIE_SECURE=true\nLISTEN_ADDR=127.0.0.1:%s\nWEB_DIR=%s/current/web\nATTACHMENTS_DIR=%s/attachments\nEXPORTS_DIR=%s/exports\nREQUIRE_DATABASE_URL=true\n' "$domain" "$port" "$install_dir" "$data_dir" "$data_dir" >> "$stage"
  # Input is raw key=value, not shell code. Empty separating lines are omitted.
  sed -i '/^$/d' "$stage"
  native_read_env "$stage"
else
  native_load_env "$config_dir"
fi
export REQUIRE_DATABASE_URL=true
runuser -u "$service_user" -- "$binary" database-check || native_die '数据库预检失败；保留原配置。'
if [[ -n $stage ]]; then
  chmod 600 -- "$stage"
  mv -Tf -- "$stage" "$config_dir/service.env";stage=''
fi
printf '[拾记] 私有数据库配置预检通过。\n'
