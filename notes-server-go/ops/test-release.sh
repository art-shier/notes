#!/usr/bin/env bash
# Platform runner runtimes are not application build dependencies.
set -Eeuo pipefail
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)
cd "$root"
docker build --target web-test --file notes-server-go/Dockerfile .
docker build --target server-test --file notes-server-go/Dockerfile .
