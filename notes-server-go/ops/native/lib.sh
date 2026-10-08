#!/usr/bin/env bash
native_die() { printf '[拾记] %s\n' "$*" >&2; exit 1; }
native_plain_path() {
  local path=$1 check=$1
  [[ $path =~ ^/[A-Za-z0-9_./-]+$ && $path != / && $path == "$(realpath -m -- "$path")" ]] || native_die '需要规范的绝对路径，不支持空格或特殊字符。'
  while [[ $check != / ]]; do
    [[ ! -L $check ]] || native_die '原生部署路径不能包含符号链接。'
    check=$(dirname -- "$check")
  done
}
native_private_file() {
  local mode
  [[ -f $1 && ! -L $1 && $(stat -c %u -- "$1") == 0 ]] || native_die '配置必须是 root 拥有的常规文件。'
  mode=$(stat -c %a -- "$1")
  (( (8#$mode & 077) == 0 )) || native_die '配置文件必须禁止组和其他用户读取。'
}
native_root_chain() {
  local check=$1 mode
  while :; do
    if [[ -e $check ]]; then
      [[ -d $check && $(stat -c %u -- "$check") == 0 ]] || native_die '程序/配置目录及祖先必须属于root。'
      mode=$(stat -c %a -- "$check")
      (( (8#$mode & 022) == 0 )) || native_die '程序/配置目录及祖先不能允许组或其他用户写入。'
    fi
    [[ $check != / ]] || break
    check=$(dirname -- "$check")
  done
}
native_meta() {
  local config_dir=$1
  native_plain_path "$config_dir"
  native_private_file "$config_dir/native.json"
  field() { jq -er --arg key "$1" '.[$key] | select(type == "string")' "$config_dir/native.json" 2>/dev/null; }
  domain=$(field domain); port=$(field port); service_user=$(field service_user)
  install_dir=$(field install_dir); data_dir=$(field data_dir)
  [[ ${#domain} -le 253 && $domain =~ ^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$ ]] || native_die '域名无效。'
  [[ $port =~ ^[0-9]{1,5}$ && $port -ge 1024 && $port -le 65535 ]] || native_die '本机监听端口必须在1024至65535之间。'
  [[ $service_user =~ ^[a-z][a-z0-9-]{1,27}$ ]] || native_die '服务账号标识无效。'
  native_plain_path "$install_dir"; native_plain_path "$data_dir"
  native_root_chain "$install_dir";native_root_chain "$config_dir"
}
native_load_env() {
  native_read_env "$1/service.env"
}
native_read_env() {
  local file=$1 key value
  native_plain_path "$file"; native_root_chain "$(dirname -- "$file")"
  native_private_file "$file"
  while IFS='=' read -r key value; do
    case $key in
      DATABASE_URL|DB_HOST|DB_PORT|DB_USER|DB_PASSWORD|DB_NAME|DB_SSLMODE|REQUIRE_DATABASE_URL|APP_ORIGIN|COOKIE_SECURE|LISTEN_ADDR|WEB_DIR|ATTACHMENTS_DIR|EXPORTS_DIR|NOTE_HISTORY_LIMIT|EXPORT_LIMIT_BYTES|TZ) export "$key=$value";;
      *) native_die '生成配置包含未知字段。';;
    esac
  done < "$file"
  export REQUIRE_DATABASE_URL=true
}
