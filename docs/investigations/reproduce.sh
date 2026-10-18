#!/usr/bin/env bash
# Reproduce the investigation in a disposable container. Failure is expected.
set -euo pipefail

repo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
terraform_binary=${TF_ACC_TERRAFORM_PATH:-$(command -v terraform || true)}
if [[ -z "$terraform_binary" || ! -x "$terraform_binary" ]]; then
    echo 'Set TF_ACC_TERRAFORM_PATH to an executable Linux Terraform binary.' >&2
    exit 2
fi
terraform_binary=$(realpath -- "$terraform_binary")

exec docker run --rm --init -i \
    --mount "type=bind,source=$repo_dir,target=/src,readonly" \
    --mount "type=bind,source=$terraform_binary,target=/usr/local/bin/terraform,readonly" \
    golang:1.25.8@sha256:3ac2864710f25e84381bf5d4272261c7ba73ada0339d62034df4de20dabb33ca \
    bash -s <<'CONTAINER'
set -euo pipefail
git -c safe.directory=/src clone --quiet --no-hardlinks /src /tmp/review
cd /tmp/review
git checkout --quiet --detach 6bc71b310bc81a90f3ad32194ba8c17947bf79d6
for package in client resources provider; do
    cp "/src/docs/investigations/reproductions/${package}_probe_test.go.txt" \
        "internal/${package}/investigation_test.go"
done

go version
terraform version
git rev-parse HEAD

result=0
run_group() {
    local label=$1
    shift
    printf '\n%s\n' "$label"
    if "$@"; then
        printf '%s: PASS\n' "$label"
    else
        local status=$?
        printf '%s: FAIL (exit %s)\n' "$label" "$status"
        result=1
    fi
}

run_group 'Existing suite (expected pass)' \
    go test ./... -skip Investigation -count=1 -cover
run_group 'Behavior probes (expected failures on reviewed commit)' \
    go test ./internal/client ./internal/resources -run TestInvestigation -count=1 -v -timeout=90s
run_group 'Token race (expected failure on reviewed commit)' \
    go test -race ./internal/client -run TestInvestigationTokenRace -count=1 -v -timeout=90s
run_group 'AP rename acceptance (expected failure on reviewed commit)' \
    env TF_ACC=1 TF_ACC_TERRAFORM_PATH=/usr/local/bin/terraform \
    go test ./internal/provider -run TestAccInvestigation -count=1 -v -timeout=90s
exit "$result"
CONTAINER
