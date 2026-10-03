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
  hub_url=$(field hub_url); hub_project=$(field hub_project); hub_env=$(field hub_env)
  database=$(field database); cli=$(field cli); token_file=$(field token_file)
  [[ ${#domain} -le 253 && $domain =~ ^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$ ]] || native_die '域名无效。'
  [[ $port =~ ^[0-9]{1,5}$ && $port -ge 1024 && $port -le 65535 ]] || native_die '本机监听端口必须在1024至65535之间。'
  [[ $service_user =~ ^[a-z][a-z0-9-]{1,27}$ ]] || native_die '服务账号标识无效。'
  native_plain_path "$install_dir"; native_plain_path "$data_dir"; native_plain_path "$cli"
  native_root_chain "$install_dir";native_root_chain "$config_dir"
  native_root_chain "$(dirname -- "$cli")"
  [[ $hub_url =~ ^https://[a-zA-Z0-9.-]+(:[0-9]{1,5})?(/[^[:space:]?#]*)?$ && $hub_url != *'@'* ]] || native_die 'ConfigHub地址必须HTTPS。'
  [[ $hub_project =~ ^[a-z0-9][a-z0-9-]{0,62}$ && $hub_env =~ ^[a-z0-9][a-z0-9-]{0,62}$ ]] || native_die 'ConfigHub项目/环境无效。'
  [[ $database =~ ^[a-z][a-z0-9_]{0,62}$ && $database != postgres && $database != template0 && $database != template1 ]] || native_die '需要独立的业务数据库。'
  [[ -x $cli && -f $cli && $(stat -c %u -- "$cli") == 0 ]] || native_die 'ConfigHub CLI必须属于root且可执行。'
  local cli_mode
  cli_mode=$(stat -c %a -- "$cli")
  (( (8#$cli_mode & 022) == 0 )) || native_die 'ConfigHub CLI不能允许组或其他用户写入。'
  CLI=("$cli" --server "$hub_url")
  if [[ -n $token_file ]]; then
    native_plain_path "$token_file"; native_private_file "$token_file"
    CLI+=(--token-file "$token_file")
  fi
}
native_load_env() {
  local config_dir=$1 key value
  native_private_file "$config_dir/service.env"
  while IFS='=' read -r key value; do
    case $key in
      DATABASE_URL|APP_ORIGIN|COOKIE_SECURE|LISTEN_ADDR|WEB_DIR|ATTACHMENTS_DIR|EXPORTS_DIR|NOTE_HISTORY_LIMIT|EXPORT_LIMIT_BYTES|TZ) export "$key=$value";;
      *) native_die '生成配置包含未知字段。';;
    esac
  done < "$config_dir/service.env"
}
