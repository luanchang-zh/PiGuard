#!/usr/bin/env bash
set -euo pipefail

task_root=$(cd -- "$(dirname -- "$0")/.." && pwd)
cd "$task_root"
mkdir -p data/acceptance

docker version --format '{{.Server.Version}}'
go test -count=1 ./... 2>&1 | tee data/acceptance/windows-or-host-tests.log
go vet ./... 2>&1 | tee data/acceptance/host-vet.log

task_source_path=$task_root
task_module_path=$(go env GOMODCACHE)
case "$(uname -s)" in
  MINGW*|MSYS*)
    task_source_path=$(cygpath -m "$task_source_path")
    task_module_path=$(cygpath -m "$task_module_path")
    ;;
esac
task_container="piguard-linux-verification-$$"
trap 'docker rm -f "$task_container" >/dev/null 2>&1 || true' EXIT
MSYS_NO_PATHCONV=1 docker run --rm --name "$task_container" \
  -v "$task_source_path:/src:ro" -v "$task_module_path:/go/pkg/mod:ro" \
  -w /src golang:1.25-alpine sh -c \
  'apk add --no-cache build-base && go test -count=1 ./... && go vet ./... && go test -race -count=1 ./...' \
  2>&1 | tee data/acceptance/linux-tests-vet-race.log

PIGUARD_SOAK_DURATION="${PIGUARD_SOAK_DURATION:-30m}" \
  go test -tags integration ./internal/server -run '^TestDockerAcceptance$' \
  -count=1 -timeout 50m -v 2>&1 | tee data/acceptance/docker-acceptance.log
