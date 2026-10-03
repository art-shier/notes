#!/usr/bin/env bash
set -Eeuo pipefail
server_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$server_dir"
if [[ $# != 1 || $EUID != 0 || -z ${COMPOSE_PROJECT_NAME:-} ]]; then
  echo 'Usage: sudo env COMPOSE_PROJECT_NAME=shiji-recovery bash ops/restore.sh /absolute/backup/snapshot' >&2; exit 2
fi
source=$1
[[ $source == /* && $source != *:* && -f $source/manifest.json && ! -L $source ]] || { echo 'A checked absolute backup directory is required.' >&2; exit 2; }
docker compose config --quiet
[[ -z $(docker compose ps -aq app) ]] || { echo 'Restore only into a new Compose project, before creating its application container.' >&2; exit 2; }
# Validate before starting or writing the destination database.
docker compose run --rm --no-deps --volume "$source:/backup:ro" --entrypoint shiji app backup-verify --backup /backup
docker compose up -d db
ready=false
for attempt in {1..30}; do
  if docker compose exec -T db pg_isready -U notes -d notes >/dev/null 2>&1; then ready=true; break; fi
  sleep 2
done
[[ $ready == true ]] || { echo 'Destination database did not become ready.' >&2; exit 1; }
docker compose run --rm --no-deps --volume "$source:/backup:ro" --entrypoint shiji app backup-restore --backup /backup --app-stopped
echo 'Restore verified. Application and Caddy are not started. Review domain/ports, then start and run ops/check.sh.'
