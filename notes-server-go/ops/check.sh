#!/usr/bin/env bash
set -Eeuo pipefail
[[ $# == 1 && $1 == https://* ]] || { echo 'Usage: bash ops/check.sh https://notes.example.com' >&2; exit 2; }
origin=${1%/}
for endpoint in live ready; do
  expected=ok
  [[ $endpoint == ready ]] && expected=ready
  response=$(curl --fail --silent --show-error --max-time 15 --proto '=https' "$origin/api/v1/health/$endpoint")
  [[ $response == "{\"status\":\"$expected\"}" ]] || { echo "Invalid health response for $endpoint" >&2; exit 1; }
  echo "PASS: HTTPS health $expected"
done
curl --fail --silent --show-error --max-time 15 --proto '=https' "$origin/" >/dev/null
echo 'PASS: HTTPS Web entry. Complete account/image/Agent checks in the deployment guide.'
