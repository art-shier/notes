#!/usr/bin/env bash
# Arguments: private temporary directory, project, environment, database, CLI argv.
# Produces private database.json/target; no remote value is evaluated as shell code.
shiji_read_database() {
  local stage=$1 hub_project=$2 hub_env=$3 database=$4
  shift 4
  "$@" export --project "$hub_project" --env "$hub_env" --format json > "$stage/config.json" || return $?
  jq -e --arg database "$database" --arg project "$hub_project" --arg environment "$hub_env" '
    select(.project == $project and .environment == $environment) | .values as $v |
    {host: ($v.notes_db_address // $v.db_address), port: (($v.notes_db_port // $v.db_port) | tostring),
     user: ($v.notes_db_username // $v.db_username), password: ($v.notes_db_password // $v.db_password),
     database: $database, sslmode: ($v.notes_db_sslmode // "require")} |
    select(.host | type == "string") | select(.host | test("^[A-Za-z0-9:.\\[\\]-]+$")) |
    select(.port | test("^[0-9]{1,5}$")) | select((.port | tonumber) >= 1 and (.port | tonumber) <= 65535) |
    select(.user | type == "string") | select(.user | length > 0) |
    select(.password | type == "string") | select(.password | length > 0) |
    select(.sslmode == "require" or .sslmode == "verify-ca" or .sslmode == "verify-full") |
    (if (.host | contains(":")) and (.host | startswith("[") | not) then "[" + .host + "]" else .host end) as $host |
    . + {uri: ("postgresql://" + (.user | @uri) + ":" + (.password | @uri) + "@" + $host + ":" + .port + "/" + .database + "?sslmode=" + .sslmode + "&connect_timeout=10")}
  ' "$stage/config.json" > "$stage/database.json" 2>/dev/null || {
    printf '[拾记] ConfigHub返回的配置格式或数据库字段无效，保留原配置。\n' >&2
    return 1
  }
  jq -c '{host, port, user, database}' "$stage/database.json" > "$stage/target"
}
