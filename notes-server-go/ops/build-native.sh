#!/usr/bin/env bash
set -Eeuo pipefail
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)
version=${1:-}; output=${2:-}
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ && $output == /* && $output != / ]] || { echo 'Usage: bash build-native.sh 0.1.0 /absolute/output' >&2; exit 2; }
mkdir -p -- "$output"
output=$(CDPATH= cd -- "$output" && pwd -P)
(cd "$root/notes-web"; npm ci; npm run build)
stage=$(mktemp -d "$output/.build.XXXXXX")
cleanup() {
  # Only this generated stage under the checked output directory may be removed.
  [[ $stage == "$output"/.build.* && $(realpath -m -- "$stage") == "$stage" ]] && rm -rf -- "$stage"
}
trap cleanup EXIT
for arch in amd64 arm64; do
  package="$stage/$arch"
  mkdir -p -- "$package/web" "$package/ops/native"
  (cd "$root/notes-server-go"; GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$package/shiji" ./cmd/shiji)
  cp -R -- "$root/notes-web/dist/." "$package/web/"
  cp -- "$root/notes-server-go/ops/check.sh" "$package/ops/"
  cp -- "$root/notes-server-go/ops/native/"*.sh "$root/notes-server-go/ops/native/"*.in "$package/ops/native/"
  printf 'v%s\n' "$version" > "$package/version.txt"
  chmod 755 -- "$package/shiji" "$package/ops/"*.sh "$package/ops/native/"*.sh
  tar -C "$package" -czf "$output/notes-server_${version}_linux_${arch}.tar.gz" shiji web ops version.txt
done
(cd "$output"; sha256sum "notes-server_${version}_linux_amd64.tar.gz" "notes-server_${version}_linux_arm64.tar.gz" > checksums.txt)
printf 'Native runtime packages written to %s\n' "$output"
