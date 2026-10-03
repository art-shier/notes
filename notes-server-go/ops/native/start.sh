#!/usr/bin/env bash
set -Eeuo pipefail
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
source "$here/lib.sh"
config_dir=/etc/shiji
if (($#)); then [[ $# == 2 && $1 == --config-dir ]] || native_die 'Usage: sudo bash start.sh [--config-dir /etc/shiji]'; config_dir=$2; fi
[[ $(id -u) == 0 ]] || native_die '原生服务启动需要root。'
native_meta "$config_dir"
if systemctl is-active --quiet "$service_user-config.service"; then
  bash "$here/prepare.sh" --config-dir "$config_dir"
else
  systemctl start "$service_user-config.service"
fi
systemctl restart "$service_user.service"
for attempt in {1..30}; do
  if response=$(curl --fail --silent --max-time 3 "http://127.0.0.1:$port/api/v1/health/ready") && [[ $response == '{"status":"ready"}' ]]; then
    printf '[拾记] 原生服务已就绪，本机端口 %s。\n' "$port"; exit 0
  fi
  sleep 2
done
native_die 'Go服务未就绪，请查看systemd日志。'
