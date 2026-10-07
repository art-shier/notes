#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ ${DEPLOYCTL_APPLICATION:-} == notes ]] || exit 1
# Consume the effective snapshot, including operator overrides, never source env.
env_file="$(dirname -- "${DEPLOYCTL_ENV_FILE:?}")/effective.env"
[[ -f $env_file && ! -L $env_file ]] || exit 1
image=${DEPLOYCTL_IMAGE:?}
state=$(docker run --rm --pull never --entrypoint shiji --env-file "$env_file" "$image" bootstrap-status)
case $state in
  empty)
    if [[ -n ${DEPLOYCTL_PARAM_ADMIN_EMAIL:-} ]]; then
      # Invitation is printed only into ctl's protected hook log.
      docker run --rm --pull never --entrypoint shiji --env-file "$env_file" "$image" bootstrap --email "$DEPLOYCTL_PARAM_ADMIN_EMAIL"
    else
      printf '[拾记] 服务就绪；尚未创建管理员邀请。\n'
    fi;;
  pending) printf '[拾记] 已有待注册邀请，保留现状。\n';;
  registered) printf '[拾记] 已有注册账号，保留现状。\n';;
  *) printf '[拾记] 账户状态检查失败。\n' >&2;exit 1;;
esac
