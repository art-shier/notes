#!/usr/bin/env bash
# Render raw deployctl configuration from ConfigHub before install/upgrade.
set -Eeuo pipefail
umask 077
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
source "$root/notes-server-go/ops/config-hub-lib.sh"
source "$root/notes-server-go/ops/native/lib.sh"
domain=notes.shier.art image='' environment=prod config_root=/etc/deployctl
hub_url=https://config.shier.art hub_project=shier hub_env=prod database=notes
cli='' token_file='' stage='' lock='' publishing=false
cleanup() {
  local status=$?
  if ((status)) && $publishing; then
    for index in "${!destinations[@]}"; do
      if [[ -f $stage/backup-$index ]]; then cp -p -- "$stage/backup-$index" "${destinations[index]}";else rm -f -- "${destinations[index]}";fi
    done
  fi
  if [[ -n $stage ]]; then
    rm -f -- "$stage/config.json" "$stage/database.json" "$stage/target" "$stage/config.env" "$stage/secrets.env" "$stage/metadata" "$stage/saved" "$stage/proposed" "$stage/check.log"
    rm -f -- "$stage/backup-0" "$stage/backup-1" "$stage/backup-2" "$stage/backup-3"
    rmdir -- "$stage"
  fi
  [[ -z $lock ]] || rmdir -- "$lock"
}
trap cleanup EXIT
while (($#)); do
  (($#>=2)) || native_die '缺少参数值。'
  case $1 in
    --domain) domain=$2;; --image) image=$2;; --env) environment=$2;; --config-root) config_root=$2;;
    --config-hub-url) hub_url=$2;; --config-hub-project) hub_project=$2;; --config-hub-env) hub_env=$2;;
    --database-name) database=$2;; --cli-binary) cli=$2;; --token-file) token_file=$2;;
    *) native_die "未知参数：$1";;
  esac
  shift 2
done
[[ $(uname -s) == Linux && $(id -u) == 0 ]] || native_die '配置生成需要Linux和root。'
[[ ${#domain} -le 253 && $domain =~ ^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$ ]] || native_die '需要有效HTTPS域名，不带协议。'
domain=${domain,,}
[[ $image =~ ^[a-z0-9][a-z0-9./:_-]*@sha256:[a-f0-9]{64}$ ]] || native_die '需要发布包中的真实镜像digest，不接受浮动tag。'
[[ $environment =~ ^[a-z][a-z0-9-]{0,31}$ && $hub_project =~ ^[a-z0-9][a-z0-9-]{0,62}$ && $hub_env =~ ^[a-z0-9][a-z0-9-]{0,62}$ ]] || native_die '环境或ConfigHub标识无效。'
[[ $hub_url =~ ^https://[a-zA-Z0-9.-]+(:[0-9]{1,5})?(/[^[:space:]?#]*)?$ && $hub_url != *'@'* ]] || native_die 'ConfigHub地址必须HTTPS。'
[[ $database =~ ^[a-z][a-z0-9_]{0,62}$ && $database != postgres && $database != template0 && $database != template1 ]] || native_die '需要独立业务数据库。'
native_plain_path "$config_root";native_root_chain "$config_root"
config_dir="$config_root/notes/$environment"
native_plain_path "$config_dir";native_root_chain "$config_dir"
cli=${cli:-$(command -v confighub || true)}
native_plain_path "$cli";native_root_chain "$(dirname -- "$cli")"
[[ -x $cli && -f $cli && $(stat -c %u -- "$cli") == 0 ]] || native_die '需要root所有的ConfigHub CLI。'
cli_mode=$(stat -c %a -- "$cli");(( (8#$cli_mode & 022) == 0 )) || native_die 'CLI不能允许组/其他用户写入。'
CLI=("$cli" --server "$hub_url")
if [[ -n $token_file ]]; then native_plain_path "$token_file";native_private_file "$token_file";CLI+=(--token-file "$token_file");fi
for tool in docker jq; do command -v "$tool" >/dev/null || native_die "需要$tool。";done
mkdir -p -- "$config_dir";chmod 700 -- "$config_dir"
for file in config.env secrets.env .notes-team.json .database-target; do
  [[ ! -L $config_dir/$file && ( ! -e $config_dir/$file || -f $config_dir/$file ) ]] || native_die '配置目标必须为常规文件。'
done
mkdir -- "$config_dir/.notes-prepare.lock" 2>/dev/null || native_die '另一个配置生成正在运行。'
lock="$config_dir/.notes-prepare.lock"
stage=$(mktemp -d "$config_dir/.notes-prepare.XXXXXX")
if [[ ! -f $config_dir/.notes-team.json && ( -s $config_dir/config.env || -s $config_dir/secrets.env ) ]]; then native_die '已有配置不属于本生成器，未覆盖。';fi
jq -n --arg domain "$domain" --arg project "$hub_project" --arg env "$hub_env" --arg url "$hub_url" --arg db "$database" '{domain:$domain,project:$project,environment:$env,url:$url,database:$db}' > "$stage/metadata"
if [[ -f $config_dir/.notes-team.json ]]; then
  native_private_file "$config_dir/.notes-team.json"
  jq -S . "$config_dir/.notes-team.json" > "$stage/saved";jq -S . "$stage/metadata" > "$stage/proposed"
  cmp -s -- "$stage/saved" "$stage/proposed" || native_die '域名或配置来源变化，拒绝自动切换。'
fi
shiji_read_database "$stage" "$hub_project" "$hub_env" "$database" "${CLI[@]}" || native_die 'ConfigHub读取或字段校验失败，保留原配置。'
if [[ -f $config_dir/.database-target ]]; then cmp -s -- "$config_dir/.database-target" "$stage/target" || native_die '数据库目标变化，拒绝自动切库。';fi
printf 'APP_ORIGIN=https://%s\nCOOKIE_SECURE=true\nLISTEN_ADDR=0.0.0.0:8000\nWEB_DIR=/app/web\nATTACHMENTS_DIR=/data/attachments\nEXPORTS_DIR=/data/exports\nNOTE_HISTORY_LIMIT=200\nEXPORT_LIMIT_BYTES=2147483648\nTZ=UTC\n' "$domain" > "$stage/config.env"
printf 'DATABASE_URL=%s\n' "$(jq -er .uri "$stage/database.json")" > "$stage/secrets.env"
if ! docker run --rm --pull always --entrypoint shiji --env-file "$stage/config.env" --env-file "$stage/secrets.env" "$image" database-check > "$stage/check.log" 2>&1; then
  native_die '镜像拉取或只读数据库预检失败，保留原配置；确认镜像拉取权限和数据库连接。'
fi
chmod 600 -- "$stage/config.env" "$stage/secrets.env" "$stage/metadata" "$stage/target"
destinations=("$config_dir/config.env" "$config_dir/secrets.env" "$config_dir/.database-target" "$config_dir/.notes-team.json")
for index in "${!destinations[@]}"; do [[ ! -f ${destinations[index]} ]] || cp -p -- "${destinations[index]}" "$stage/backup-$index";done
publishing=true
mv -Tf -- "$stage/config.env" "$config_dir/config.env"
mv -Tf -- "$stage/secrets.env" "$config_dir/secrets.env"
mv -Tf -- "$stage/target" "$config_dir/.database-target"
mv -Tf -- "$stage/metadata" "$config_dir/.notes-team.json"
publishing=false
printf '[拾记] ConfigHub配置及数据库预检通过：%s。现在可执行deployctl install/upgrade。\n' "$config_dir"
