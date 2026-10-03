#!/usr/bin/env bash
set -Eeuo pipefail
python=''
for candidate in python3 python; do
  if command -v "$candidate" >/dev/null && "$candidate" -c 'import sys; sys.exit(sys.version_info < (3, 10))' >/dev/null 2>&1; then
    python=$candidate; break
  fi
done
[[ -n $python ]] || { printf 'Please install Python 3.10+ and rerun.\n' >&2; exit 1; }
local_dir=''
if [[ -n ${BASH_SOURCE[0]:-} && -f ${BASH_SOURCE[0]} ]]; then
  local_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
fi
if [[ -n $local_dir && -f $local_dir/install-client.py ]]; then
  "$python" "$local_dir/install-client.py" "$@"
else
  command -v curl >/dev/null || { printf 'curl is required.\n' >&2; exit 1; }
  temporary=$(mktemp -d)
  cleanup() { rm -f -- "$temporary/install-client.py"; rmdir -- "$temporary"; }
  trap cleanup EXIT
  curl --fail --silent --show-error --proto '=https' https://raw.githubusercontent.com/art-shier/notes/main/install-client.py -o "$temporary/install-client.py"
  "$python" "$temporary/install-client.py" "$@"
fi
