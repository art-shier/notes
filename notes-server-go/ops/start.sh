#!/usr/bin/env bash
# Pull via ConfigHub CLI, validate privately, then publish configuration/start.
set -Eeuo pipefail
umask 077
server_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
cd "$server_dir"
# Compose environment variables must come from the checked file, not ambient overrides.
unset DATABASE_URL DOMAIN COMPOSE_FILE COMPOSE_PROJECT_NAME COMPOSE_ENV_FILES COMPOSE_PROFILES POSTGRES_PASSWORD NOTE_HISTORY_LIMIT EXPORT_LIMIT_BYTES
domain='' project='' hub_url='' hub_project='' hub_env='' database='' token_file='' cli=''
temp_dir='' lock=''
die() { printf '[拾记] %s\n' "$*" >&2; exit 1; }
cleanup() {
  local status=$?
  if [[ -n $temp_dir ]]; then
    rm -f -- "$temp_dir/config.json" "$temp_dir/database.json" "$temp_dir/env" "$temp_dir/metadata" "$temp_dir/target"
    rmdir -- "$temp_dir" 2>/dev/null || true
  fi
  [[ -z $lock ]] || rmdir -- "$lock" 2>/dev/null || true
  if ((status)); then printf '[拾记] 启动中断；配置检查失败时未覆盖原配置，已有数据保留。\n' >&2; fi
}
trap cleanup EXIT
while (($#)); do
  (($#>=2)) || die "缺少参数值：$1"
  case $1 in
    --domain) domain=$2;; --project) project=$2;;
    --config-hub-url) hub_url=$2;; --config-hub-project) hub_project=$2;; --config-hub-env) hub_env=$2;;
    --database-name) database=$2;; --token-file) token_file=$2;; --cli-binary) cli=$2;;
    *) die "未知参数：$1";;
  esac
  shift 2
done
for file in .env .config-hub.json .database-target; do
  [[ ! -L $file && ( ! -e $file || -f $file ) ]] || die "$file 必须是常规文件。"
done
mkdir -- .install.lock 2>/dev/null || die '另一个安装/启动正在运行；未改动配置。'
lock="$server_dir/.install.lock"
DOCKER=(docker)
if ! docker info >/dev/null 2>&1; then
  if ((EUID!=0)); then DOCKER=(sudo docker); fi
  "${DOCKER[@]}" info >/dev/null 2>&1 || die 'Docker 不可用。'
fi
# Standalone deployments may also use this entry point without ConfigHub.
if [[ ! -f .config-hub.json && -z $hub_url && -z $hub_project && -z $hub_env ]]; then
  [[ -f .env ]] || die '缺少 .env；请先运行 install.sh 或指定 ConfigHub 参数。'
  "${DOCKER[@]}" compose up -d --wait --wait-timeout 180
  exit
fi
command -v jq >/dev/null || die '需要 jq；请运行新版 install.sh 安装依赖。'
if [[ -f .config-hub.json ]]; then
  saved() { jq -er --arg key "$1" '.[$key] | select(type == "string")' .config-hub.json 2>/dev/null; }
  domain=${domain:-$(saved domain)}; project=${project:-$(saved project)}
  hub_url=${hub_url:-$(saved hub_url)}; hub_project=${hub_project:-$(saved hub_project)}; hub_env=${hub_env:-$(saved hub_env)}
  database=${database:-$(saved database)}; token_file=${token_file:-$(saved token_file)}; cli=${cli:-$(saved cli)}
fi
project=${project:-shiji}; database=${database:-notes}; cli=${cli:-confighub}
[[ ${#domain} -le 253 && $domain =~ ^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$ ]] || die '需要有效 --domain。'
domain=${domain,,}
[[ $project =~ ^[a-z0-9][a-z0-9_-]{0,62}$ ]] || die 'Compose 项目标识无效。'
[[ $hub_url =~ ^https://[a-zA-Z0-9.-]+(:[0-9]{1,5})?(/[^[:space:]?#]*)?$ && $hub_url != *'@'* ]] || die 'ConfigHub 地址必须是有效 HTTPS URL。'
[[ $hub_project =~ ^[a-z0-9][a-z0-9-]{0,62}$ && $hub_env =~ ^[a-z0-9][a-z0-9-]{0,62}$ ]] || die '需要有效 ConfigHub 项目和环境标识。'
[[ $database =~ ^[a-z][a-z0-9_]{0,62}$ && $database != postgres && $database != template0 && $database != template1 ]] || die '请使用独立业务数据库名，不能使用 PostgreSQL 系统库。'
command -v "$cli" >/dev/null || die 'ConfigHub CLI 不存在。'
if [[ -f .env ]]; then
  [[ -f .database-target && -f .config-hub.json ]] || die '已有配置不是 ConfigHub 外部数据库部署；请使用新目录，不会自动迁移或切库。'
  [[ $(sed -n 's/^DOMAIN=//p' .env) == "$domain" ]] || die '域名与已有配置不一致，未覆盖配置。'
  [[ $(sed -n 's/^COMPOSE_PROJECT_NAME=//p' .env) == "$project" ]] || die 'Compose 项目名与已有配置不一致，未切换持久卷。'
fi
CLI=("$cli" --server "$hub_url")
if [[ -n $token_file ]]; then
  [[ -f $token_file && ! -L $token_file && $token_file == /* ]] || die '--token-file 必须是已有绝对路径的常规文件。'
  CLI+=(--token-file "$token_file")
fi
temp_dir=$(mktemp -d "$server_dir/.config-tmp.XXXXXX")
printf '[拾记] 读取 ConfigHub %s/%s 配置\n' "$hub_project" "$hub_env"
"${CLI[@]}" export --project "$hub_project" --env "$hub_env" --format json > "$temp_dir/config.json" || die 'ConfigHub 拉取失败；未启动或替换配置。'
# Encode values as data; never source/eval a remote dotenv or print credentials.
jq -e --arg database "$database" --arg project "$hub_project" --arg environment "$hub_env" '
  select(.project == $project and .environment == $environment) | .values as $v |
  {host: ($v.notes_db_address // $v.db_address), port: (($v.notes_db_port // $v.db_port) | tostring),
   user: ($v.notes_db_username // $v.db_username), password: ($v.notes_db_password // $v.db_password),
   database: $database, sslmode: ($v.notes_db_sslmode // "require")} |
  select(.host | type == "string") | select(.host | test("^[A-Za-z0-9:.\\[\\]-]+$")) |
  select(.port | test("^[0-9]{1,5}$")) | select((.port | tonumber) >= 1 and (.port | tonumber) <= 65535) |
  select(.user | type == "string") | select(.user | length > 0) |
  select(.password | type == "string") | select(.password | length > 0) |
  select(.sslmode == "require" or .sslmode == "verify-ca" or .sslmode == "verify-full")
' "$temp_dir/config.json" > "$temp_dir/database.json" 2>/dev/null || die '数据库字段缺失或无效；未覆盖配置。'
jq -c '{host, port, user, database}' "$temp_dir/database.json" > "$temp_dir/target"
if [[ -f .database-target ]]; then
  cmp -s -- .database-target "$temp_dir/target" || die '数据库主机/端口/名称/账号变化，拒绝自动切库；请先核对迁移计划。'
fi
database_url=$(jq -er '
  (if (.host | contains(":")) and (.host | startswith("[") | not) then "[" + .host + "]" else .host end) as $host |
  "postgresql://" + (.user | @uri) + ":" + (.password | @uri) + "@" + $host + ":" + .port + "/" + .database + "?sslmode=" + .sslmode + "&connect_timeout=10"
' "$temp_dir/database.json")
history=200; export_limit=2147483648
if [[ -f .env ]]; then
  history=$(sed -n 's/^NOTE_HISTORY_LIMIT=//p' .env); export_limit=$(sed -n 's/^EXPORT_LIMIT_BYTES=//p' .env)
fi
[[ $history =~ ^[0-9]+$ && $history -ge 2 && $history -le 2000 && $export_limit =~ ^[1-9][0-9]{0,17}$ ]] || die '历史或导出限制无效。'
printf "DOMAIN=%s\nCOMPOSE_PROJECT_NAME=%s\nCOMPOSE_FILE=compose.external.yaml\nDATABASE_URL='%s'\nNOTE_HISTORY_LIMIT=%s\nEXPORT_LIMIT_BYTES=%s\n" "$domain" "$project" "$database_url" "$history" "$export_limit" > "$temp_dir/env"
unset database_url
jq -n --arg domain "$domain" --arg project "$project" --arg hub_url "$hub_url" --arg hub_project "$hub_project" --arg hub_env "$hub_env" --arg database "$database" --arg token_file "$token_file" --arg cli "$cli" '{domain:$domain, project:$project, hub_url:$hub_url, hub_project:$hub_project, hub_env:$hub_env, database:$database, token_file:$token_file, cli:$cli}' > "$temp_dir/metadata"
COMPOSE=("${DOCKER[@]}" compose --project-directory "$server_dir" -f "$server_dir/compose.external.yaml" --env-file "$temp_dir/env")
"${COMPOSE[@]}" config --quiet
printf '[拾记] 构建服务，并只读检查外部数据库\n'
"${COMPOSE[@]}" build --pull app
"${COMPOSE[@]}" run --rm --no-deps --entrypoint shiji app database-check || die '数据库不可用、未创建或不是空库/笔记库；未覆盖配置。'
chmod 600 -- "$temp_dir/env" "$temp_dir/metadata" "$temp_dir/target"
mv -- "$temp_dir/metadata" .config-hub.json
mv -- "$temp_dir/target" .database-target
mv -- "$temp_dir/env" .env
COMPOSE=("${DOCKER[@]}" compose --project-directory "$server_dir" --env-file "$server_dir/.env")
printf '[拾记] 配置已更新，启动 Go/Web 与 HTTPS 入口\n'
"${COMPOSE[@]}" up -d --wait --wait-timeout 180
