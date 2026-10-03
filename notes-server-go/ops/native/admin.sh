#!/usr/bin/env bash
set -Eeuo pipefail
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
source "$here/lib.sh"
config_dir=/etc/shiji
if [[ ${1:-} == --config-dir ]]; then [[ $# -ge 3 ]] || native_die '缺少配置目录或命令。';config_dir=$2;shift 2;fi
[[ $(id -u) == 0 && $# -gt 0 ]] || native_die 'Usage: sudo bash admin.sh [--config-dir /etc/shiji] <shiji command>'
native_meta "$config_dir"
native_load_env "$config_dir"
exec "$install_dir/current/shiji" "$@"
