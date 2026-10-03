#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
server_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$server_dir"
if [[ $# != 1 || $EUID != 0 ]]; then
  echo 'Usage: sudo bash ops/backup.sh /absolute/new-backup-directory' >&2; exit 2
fi
destination=$1
[[ $destination == /* && $destination != *:* && ! -e $destination && ! -L $destination ]] || { echo 'A new absolute backup directory is required.' >&2; exit 2; }
docker compose config --quiet
docker compose run --rm --no-deps --entrypoint pg_dump app --version >/dev/null
mkdir -m 700 -- "$destination"
chown 10001:10001 -- "$destination"
running_ids=$(docker compose ps --status running -q app)
running=()
if [[ -n $running_ids ]]; then mapfile -t running <<< "$running_ids"; fi
resume() {
  local status=$?
  trap - EXIT
  if ((${#running[@]})); then
    if ! docker start "${running[@]}" >/dev/null; then
      echo 'Backup finished/failed, but application restart failed. Check docker compose ps.' >&2
      status=1
    fi
  fi
  exit "$status"
}
trap resume EXIT
docker compose stop app
docker compose run --rm --no-deps --volume "$destination:/backup" --entrypoint shiji app backup-create --output /backup/snapshot --app-stopped
echo "Backup published at $destination/snapshot. Run ops/check.sh after the application resumes."
