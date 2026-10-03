#!/usr/bin/env bash
# Native Linux deployment using a checksummed release, systemd and external PG.
set -Eeuo pipefail
umask 077
domain='' email='' version='' artifact='' checksum_file='' port=8000 service=shiji
install_dir=/opt/shiji config_dir=/etc/shiji data_dir=/var/lib/shiji unit_dir=/etc/systemd/system
hub_url=https://config.shier.art hub_project=shier hub_env=prod database=notes token_file='' cli=''
skip_dependencies=false stage='' lock='' link_tmp=''
old_current='' transaction=false restarting=false prior_running=false
die() { printf '[拾记] %s\n' "$*" >&2; exit 1; }
plain_path() {
  local path=$1 check=$1
  [[ $path =~ ^/[A-Za-z0-9_./-]+$ && $path != / && $path == "$(realpath -m -- "$path")" ]] || die '需要独立规范绝对路径，不支持空格或特殊字符。'
  while [[ $check != / ]]; do [[ ! -L $check ]] || die '路径不能包含符号链接。';check=$(dirname -- "$check");done
}
root_directory_chain() {
  local check=$1 mode
  while :; do
    if [[ -e $check ]]; then
      [[ -d $check && $(stat -c %u -- "$check") == 0 ]] || die '程序、配置及目录祖先必须属于root。'
      mode=$(stat -c %a -- "$check")
      (( (8#$mode & 022) == 0 )) || die "程序、配置及目录祖先不能允许组或其他用户写入：$check（权限$mode）。"
    fi
    [[ $check != / ]] || break
    check=$(dirname -- "$check")
  done
}
rollback() {
  printf '[拾记] 发布失败，恢复上次程序和配置。\n' >&2
  if $restarting; then systemctl stop "$service.service" || true;fi
  ln -s -- "$old_current" "$install_dir/.rollback-current" && mv -Tf -- "$install_dir/.rollback-current" "$install_dir/current"
  for index in "${!outputs[@]}"; do
    if [[ -f $stage/previous-$index ]]; then
      cp -p -- "$stage/previous-$index" "${outputs[index]}"
    else
      rm -f -- "${outputs[index]}"
    fi
  done
  systemctl daemon-reload || true
  if $restarting && $prior_running; then
    # A failed new migration must not force an incompatible old binary to run.
    if bash "$old_current/ops/native/admin.sh" --config-dir "$config_dir" database-check; then
      systemctl start "$service.service" || printf '[拾记] 旧服务重启失败，请查看systemd日志。\n' >&2
    else
      printf '[拾记] 已恢复旧程序，数据库兼容检查失败；请从备份恢复后启动。\n' >&2
    fi
  fi
}
cleanup() {
  local status=$?
  if ((status)) && $transaction && [[ -n $old_current ]]; then rollback || true;fi
  [[ -z $link_tmp ]] || rm -f -- "$link_tmp"
  if [[ -n $stage ]]; then
    [[ $stage == /tmp/notes-native-install.* && $(realpath -m -- "$stage") == "$stage" ]] && rm -rf -- "$stage"
  fi
  [[ -z $lock ]] || rmdir -- "$lock" 2>/dev/null || true
  if ((status)); then printf '[拾记] 原生部署中断，未删除数据库或持久数据。\n' >&2; fi
}
trap cleanup EXIT
while (($#)); do
  case $1 in
    --skip-dependencies) skip_dependencies=true;shift;;
    --help|-h) echo 'Usage: sudo bash install-native.sh --domain notes.example.com --email admin@example.com [--token-file /private/token] [--port 8000] [--version v0.1.0]';exit;;
    --domain|--email|--version|--artifact|--checksum-file|--port|--service-name|--install-dir|--config-dir|--data-dir|--unit-dir|--config-hub-url|--config-hub-project|--config-hub-env|--database-name|--token-file|--cli-binary)
      (($#>=2)) || die '缺少参数值。'
      case $1 in
        --domain) domain=$2;; --email) email=$2;; --version) version=$2;; --artifact) artifact=$2;; --checksum-file) checksum_file=$2;;
        --port) port=$2;; --service-name) service=$2;; --install-dir) install_dir=$2;; --config-dir) config_dir=$2;; --data-dir) data_dir=$2;; --unit-dir) unit_dir=$2;;
        --config-hub-url) hub_url=$2;; --config-hub-project) hub_project=$2;; --config-hub-env) hub_env=$2;; --database-name) database=$2;; --token-file) token_file=$2;; --cli-binary) cli=$2;;
      esac
      shift 2;;
    *) die "未知参数：$1";;
  esac
done
[[ $(uname -s) == Linux && $(id -u) == 0 ]] || die '原生安装需要 Linux 和root（可使用sudo）。'
[[ ${#domain} -le 253 && $domain =~ ^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$ ]] || die '需要有效域名。'
domain=${domain,,}
[[ ${#email} -le 254 && $email =~ ^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$ ]] || die '需要有效管理员邮箱。'
[[ $port =~ ^[0-9]{1,5}$ && $port -ge 1024 && $port -le 65535 && $service =~ ^[a-z][a-z0-9-]{1,27}$ ]] || die '端口或服务名无效。'
for path in "$install_dir" "$config_dir" "$data_dir" "$unit_dir"; do plain_path "$path";done
root_directory_chain "$install_dir";root_directory_chain "$config_dir";root_directory_chain "$unit_dir"
root_directory_chain "$(dirname -- "$data_dir")"
paths=("$install_dir" "$config_dir" "$data_dir" "$unit_dir")
for ((i=0;i<${#paths[@]};i++)); do
  for ((j=i+1;j<${#paths[@]};j++)); do
    [[ ${paths[i]} != "${paths[j]}" && ${paths[i]} != "${paths[j]}"/* && ${paths[j]} != "${paths[i]}"/* ]] || die '安装、配置、数据和单元需要互不嵌套的独立目录。'
  done
done
command -v systemctl >/dev/null || die '需要 systemd。'
if ! $skip_dependencies; then
  for tool in curl jq tar runuser; do
    if ! command -v "$tool" >/dev/null; then
      command -v apt-get >/dev/null || die '请先安装 curl/jq/tar/util-linux；自动依赖安装仅支持Debian/Ubuntu。'
      apt-get update;apt-get install -y ca-certificates curl jq tar util-linux;break
    fi
  done
fi
for tool in curl jq tar sha256sum runuser; do command -v "$tool" >/dev/null || die "缺少工具：$tool";done
initial=true
if [[ -e $install_dir/native-install.json ]]; then
  [[ -f $install_dir/native-install.json && ! -L $install_dir/native-install.json && $(stat -c %u "$install_dir/native-install.json") == 0 ]] || die '安装标记不安全。'
  initial=false
  jq -e --arg install "$install_dir" --arg config "$config_dir" --arg data "$data_dir" --arg service "$service" --arg units "$unit_dir" '.install_dir==$install and .config_dir==$config and .data_dir==$data and .service==$service and .unit_dir==$units' "$install_dir/native-install.json" >/dev/null || die '安装目标和已有部署不一致。'
elif [[ -d $install_dir && -n $(ls -A -- "$install_dir") ]]; then die '安装目录包含非本项目文件。';fi
if $initial; then
  for path in "$config_dir" "$data_dir"; do [[ ! -d $path || -z $(ls -A -- "$path") ]] || die '配置/数据目录已有其他内容。';done
  [[ ! -e $unit_dir/$service.service && ! -e $unit_dir/$service-config.service ]] || die '同名 systemd 服务已存在。'
  if systemctl cat "$service.service" >/dev/null 2>&1 || systemctl cat "$service-config.service" >/dev/null 2>&1; then die '系统已存在同名服务，请使用独立 --service-name。';fi
  if getent passwd "$service" >/dev/null; then die '同名OS账号已存在，请使用独立 --service-name。';fi
fi
stage=$(mktemp -d /tmp/notes-native-install.XXXXXX)
if [[ -z $version ]]; then version=$(curl --fail --silent --show-error --proto '=https' https://api.github.com/repos/art-shier/notes/releases/latest | jq -er .tag_name);fi
[[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] || die 'Release版本格式无效。'
case $(uname -m) in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; *) die '只支持Linux amd64/arm64。';;esac
name="notes-server_${version#v}_linux_${arch}.tar.gz"
if [[ -n $artifact ]]; then
  plain_path "$artifact";plain_path "$checksum_file"
  [[ -f $artifact && -f $checksum_file ]] || die '需要运行包与校验清单。'
  cp -- "$artifact" "$stage/$name";cp -- "$checksum_file" "$stage/checksums.txt"
else
  base="https://github.com/art-shier/notes/releases/download/$version"
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' "$base/$name" -o "$stage/$name"
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' "$base/checksums.txt" -o "$stage/checksums.txt"
fi
expected=$(awk -v name="$name" '$2==name || $2=="*"name {print $1}' "$stage/checksums.txt")
actual=$(sha256sum "$stage/$name" | awk '{print $1}')
[[ $expected =~ ^[a-f0-9]{64}$ && $actual == "$expected" ]] || die '运行包SHA-256校验失败。'
tar -tzf "$stage/$name" > "$stage/list";tar -tvzf "$stage/$name" > "$stage/types"
while IFS= read -r entry; do
  [[ $entry =~ ^[A-Za-z0-9_./-]+$ && $entry != /* && $entry != *..* ]] || die '运行包包含非法路径。'
  case $entry in shiji|version.txt|web|web/*|ops|ops/*) ;; *) die '运行包内容不匹配。';;esac
done < "$stage/list"
while IFS= read -r entry; do [[ ${entry:0:1} == - || ${entry:0:1} == d ]] || die '运行包不能包含链接或设备。';done < "$stage/types"
mkdir -- "$stage/package";tar --no-same-owner -xzf "$stage/$name" -C "$stage/package"
[[ -x $stage/package/shiji && -f $stage/package/web/index.html && -f $stage/package/ops/native/prepare.sh && $(cat "$stage/package/version.txt") == "$version" ]] || die '运行包不完整。'
mkdir -p -- "$install_dir" "$install_dir/releases" "$install_dir/tools"
chmod 755 -- "$install_dir" "$install_dir/releases" "$install_dir/tools"
mkdir -- "$install_dir/.install.lock" 2>/dev/null || die '另一个原生安装正在运行。';lock="$install_dir/.install.lock"
# A concurrent first install may have finished while this process downloaded.
if $initial && [[ -e $install_dir/native-install.json ]]; then die '另一个安装已完成，请重新执行以验证已有部署。';fi
if [[ -e $install_dir/current || -L $install_dir/current ]]; then
  [[ -L $install_dir/current && $(realpath -- "$install_dir/current") == "$install_dir"/releases/* ]] || die 'current不是受管理的发行链接。'
  old_current=$(realpath -- "$install_dir/current")
fi
if $initial; then
  jq -n --arg install "$install_dir" --arg config "$config_dir" --arg data "$data_dir" --arg service "$service" --arg units "$unit_dir" '{install_dir:$install,config_dir:$config,data_dir:$data,service:$service,unit_dir:$units}' > "$install_dir/native-install.json"
  chmod 600 -- "$install_dir/native-install.json"
fi
if ! getent passwd "$service" >/dev/null; then useradd --system --user-group --no-create-home --home-dir "$data_dir" --shell /usr/sbin/nologin "$service";fi
[[ $(id -u "$service") != 0 ]] || die '应用账号不能是root。'
mkdir -p -- "$config_dir" "$data_dir" "$data_dir/attachments" "$data_dir/exports" "$unit_dir"
chmod 700 -- "$config_dir";chmod 750 -- "$data_dir";chmod 700 -- "$data_dir/attachments" "$data_dir/exports"
chown -- "$service:$service" "$data_dir" "$data_dir/attachments" "$data_dir/exports"
if [[ -z $cli ]]; then cli=$(command -v confighub || true);fi
if [[ -z $cli && -x $install_dir/tools/confighub ]]; then cli="$install_dir/tools/confighub";fi
if [[ -z $cli ]]; then
  curl --fail --silent --show-error --proto '=https' https://raw.githubusercontent.com/art-shier/config-hub/main/scripts/install-cli.sh -o "$stage/install-cli.sh"
  bash "$stage/install-cli.sh" --install-dir "$install_dir/tools"
  cli="$install_dir/tools/confighub"
fi
plain_path "$cli";[[ -x $cli && -f $cli ]] || die 'ConfigHub CLI不可用。'
if [[ -f $config_dir/native.json && -z $token_file ]]; then token_file=$(jq -er '.token_file | select(type=="string")' "$config_dir/native.json");fi
jq -n --arg domain "$domain" --arg port "$port" --arg user "$service" --arg install "$install_dir" --arg data "$data_dir" --arg url "$hub_url" --arg project "$hub_project" --arg env "$hub_env" --arg db "$database" --arg cli "$cli" --arg token "$token_file" '{domain:$domain,port:$port,service_user:$user,install_dir:$install,data_dir:$data,hub_url:$url,hub_project:$project,hub_env:$env,database:$db,cli:$cli,token_file:$token}' > "$stage/native.json"
if [[ -e $config_dir/native.json ]]; then
  [[ ! -L $config_dir/native.json ]] || die '原生元数据不能是链接。'
  jq -S . "$config_dir/native.json" > "$stage/saved";jq -S . "$stage/native.json" > "$stage/proposed"
  if ! cmp -s -- "$stage/saved" "$stage/proposed"; then
    # A failed first preflight has never published or used this target.
    if [[ ! -e $install_dir/current && ! -L $install_dir/current && ! -e $config_dir/service.env && ! -e $config_dir/.database-target ]]; then
      install -m 600 -- "$stage/native.json" "$config_dir/native.json"
    else
      die '域名、端口、数据或配置目标改变，未自动迁移。'
    fi
  fi
else
  install -m 600 -- "$stage/native.json" "$config_dir/native.json"
fi
release="$install_dir/releases/${version}-${actual}"
if [[ ! -e $release ]]; then
  mv -- "$stage/package" "$release"
  chmod -R a+rX -- "$release"
  chmod -R go-w,u-s,g-s -- "$release"
fi
[[ -d $release && ! -L $release ]] || die '发行目录不安全。'
# Validate every publication target before changing any existing runtime state.
for template in shiji.service shiji-config.service; do
  if [[ $template == shiji.service ]]; then destination="$unit_dir/$service.service";else destination="$unit_dir/$service-config.service";fi
  [[ ! -L $destination ]] || die 'systemd单元不能是链接。'
  if [[ -e $destination ]]; then head -n 1 "$destination" | grep -qx '# Managed by art-shier/notes native installer' || die '拒绝覆盖其他systemd单元。';fi
  sed -e "s|@SERVICE@|$service|g" -e "s|@INSTALL@|$install_dir|g" -e "s|@CONFIG@|$config_dir|g" -e "s|@DATA@|$data_dir|g" "$release/ops/native/$template.in" > "$stage/$template"
done
for snippet in nginx-location.conf caddy-site.conf; do
  destination="$config_dir/$snippet"
  [[ ! -L $destination && ( ! -e $destination || -f $destination ) ]] || die '反向代理片段不能是链接或特殊文件。'
  if [[ -f $destination ]]; then head -n 1 "$destination" | grep -qx '# Managed by art-shier/notes native installer' || die '拒绝覆盖其他反向代理配置。';fi
done
invite_file="$config_dir/.bootstrap-invite"
[[ ! -L $invite_file && ( ! -e $invite_file || -f $invite_file ) ]] || die '邀请备份路径必须为常规文件。'
cat > "$stage/nginx-location.conf" <<EOF
# Managed by art-shier/notes native installer
# Add inside the existing HTTPS server for $domain; configure its certificate first.
location / {
    proxy_pass http://127.0.0.1:$port;
    proxy_set_header Host \$host;
    proxy_set_header X-Forwarded-Proto \$scheme;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    client_max_body_size 11m;
}
EOF
cat > "$stage/caddy-site.conf" <<EOF
# Managed by art-shier/notes native installer
$domain {
    header {
        X-Content-Type-Options nosniff
        Referrer-Policy same-origin
        X-Frame-Options DENY
        Content-Security-Policy "default-src 'self'; img-src 'self' blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'self'"
    }
    reverse_proxy 127.0.0.1:$port
}
EOF
outputs=("$config_dir/service.env" "$config_dir/.database-target" "$unit_dir/$service.service" "$unit_dir/$service-config.service" "$config_dir/nginx-location.conf" "$config_dir/caddy-site.conf")
for index in "${!outputs[@]}"; do
  [[ ! -L ${outputs[index]} && ( ! -e ${outputs[index]} || -f ${outputs[index]} ) ]] || die '发布目标必须为常规文件。'
  [[ ! -f ${outputs[index]} ]] || cp -p -- "${outputs[index]}" "$stage/previous-$index"
done
[[ ! -L $install_dir/.rollback-current && ! -e $install_dir/.rollback-current ]] || die '回退链接路径已占用。'
if systemctl is-active --quiet "$service.service"; then prior_running=true;fi
transaction=true
# All remote/DB validation completes while an existing application keeps running.
bash "$release/ops/native/prepare.sh" --config-dir "$config_dir" --binary "$release/shiji"
link_tmp=$(mktemp "$install_dir/.current.XXXXXX");rm -f -- "$link_tmp";ln -s -- "$release" "$link_tmp";mv -Tf -- "$link_tmp" "$install_dir/current";link_tmp=''
install -m 644 -- "$stage/shiji.service" "$unit_dir/$service.service"
install -m 644 -- "$stage/shiji-config.service" "$unit_dir/$service-config.service"
for snippet in nginx-location.conf caddy-site.conf; do
  install -m 600 -- "$stage/$snippet" "$config_dir/.$snippet.new"
  mv -Tf -- "$config_dir/.$snippet.new" "$config_dir/$snippet"
done
systemctl daemon-reload
systemctl enable "$unit_dir/$service-config.service" "$unit_dir/$service.service"
restarting=true
bash "$release/ops/native/start.sh" --config-dir "$config_dir"
transaction=false
state=$(bash "$release/ops/native/admin.sh" --config-dir "$config_dir" bootstrap-status)
[[ ! -L $invite_file ]] || die '邀请备份路径不能为链接。'
case $state in
  empty) bash "$release/ops/native/admin.sh" --config-dir "$config_dir" bootstrap --email "$email" > "$invite_file";chmod 600 -- "$invite_file";cat -- "$invite_file";;
  pending) [[ ! -f $invite_file ]] || cat -- "$invite_file";;
  registered) rm -f -- "$invite_file";echo '[拾记] 已有账户，未创建新管理员。';;
  *) die '账户初始化状态无效。';;
esac
printf '[拾记] Go/Web原生服务已启动。请将现有HTTPS反向代理指向127.0.0.1:%s。\n' "$port"
printf '配置片段：%s/nginx-location.conf 或 %s/caddy-site.conf\n' "$config_dir" "$config_dir"
printf '日志：journalctl -u %s.service -f\n' "$service"
