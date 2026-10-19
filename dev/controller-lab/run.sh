#!/usr/bin/env bash
# Run from any directory. All test mutations target the compose-owned lab.
set -euo pipefail
umask 077
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
runtime="$root/.controller-lab"
compose=(docker compose -f "$root/dev/controller-lab/compose.yaml")
runner=omada-lab-runner
go_image=golang:1.25.6@sha256:06d1251c59a75761ce4ebc8b299030576233d7437c886a68b43464bad62d4bb1

case "${1:-}" in
  up)
    mkdir -p "$runtime"
    python3 "$root/third_party/omada/verify.py"
    "${compose[@]}" up -d
    python3 "$root/dev/controller-lab/api.py" initialize
    python3 "$root/third_party/omada/verify.py" --controller > "$runtime/provenance.json"
    ;;
  probe)
    python3 "$root/dev/controller-lab/api.py" probe
    ;;
  test)
    python3 "$root/dev/controller-lab/api.py" environment
    terraform_binary=${TERRAFORM_BINARY:-$HOME/.local/bin/terraform}
    test -x "$terraform_binary" || { echo 'Set TERRAFORM_BINARY to a Terraform 1.15.8 linux/amd64 binary.' >&2; exit 2; }
    if docker container inspect "$runner" >/dev/null 2>&1; then
      echo "Runner $runner already exists; inspect it before retrying." >&2
      exit 2
    fi
    # A failed build must not leave a previous run's successful test status.
    rm -f "$runtime/build.log" "$runtime/acceptance.log" "$runtime/client-probes.log" "$runtime/test-status.txt"
    extra_mounts=()
    rm -f "$runtime/ssid-followup.patch"
    if [[ -n ${LAB_SSID_PATCH:-} ]]; then
      [[ ${LAB_SSID_INTEGRATION:-0} == 1 ]] || { echo 'LAB_SSID_PATCH requires LAB_SSID_INTEGRATION=1.' >&2; exit 2; }
      cp "$LAB_SSID_PATCH" "$runtime/ssid-followup.patch"
    fi
    if [[ ${LAB_SSID_INTEGRATION:-0} == 1 ]]; then
      : "${LAB_EVIDENCE_DIR:?Set LAB_EVIDENCE_DIR to the docs checkout docs/investigations directory}"
      # Exact D1-D3 stack; G1 supersedes its raw-SSID decoder hunk.
      git -C "$root" diff 6bc71b310bc81a90f3ad32194ba8c17947bf79d6 ec2d949699274c3a6dea6cf3d272f22bde13d12e -- . ':(exclude)internal/client/client.go' > "$runtime/d-other.patch"
      extra_mounts=(-v "$LAB_EVIDENCE_DIR:/evidence:ro")
    fi
    docker run -d --name "$runner" --label omada.provider.lab=true --memory 2g --cpus 2 --network omada-provider-lab_lab \
      -v "$root:/src:ro" -v "$runtime:/run/lab:ro" "${extra_mounts[@]}" "$go_image" sleep infinity
    trap 'docker rm -f "$runner" >/dev/null' EXIT
    docker cp "$terraform_binary" "$runner:/usr/local/bin/terraform"
    docker exec -e LAB_SSID_INTEGRATION="${LAB_SSID_INTEGRATION:-0}" "$runner" bash -euc '
      mkdir /tmp/provider
      tar -C /src --exclude=.git --exclude=.controller-lab --exclude=.history --exclude="./third_party/omada/*.tar.gz" -cf - . | tar -C /tmp/provider -xf -
      cd /tmp/provider
      if [[ $LAB_SSID_INTEGRATION == 1 ]]; then
        git apply /run/lab/d-other.patch
        git apply /evidence/pagination-fix/ssid-client-integration.patch
        git apply /evidence/http-response-fix/ssid-diagnostic-assertions.patch
        git apply /evidence/pagination-fix/ssid-pagination-fixtures.patch
        if [[ -f /run/lab/ssid-followup.patch ]]; then
          git apply /run/lab/ssid-followup.patch
        fi
      fi
      cp dev/controller-lab/client_test.go.txt internal/client/controller_lab_test.go
      cp dev/controller-lab/provider_test.go.txt internal/provider/controller_lab_test.go
      gofmt -w internal/client/controller_lab_test.go internal/provider/controller_lab_test.go
      go version
      terraform version
      terraform version | head -1 | grep -qx "Terraform v1.15.8"
      export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local
      go vet -mod=vendor ./...
      go test -mod=vendor ./...
      go build -mod=vendor -o /tmp/bin/terraform-provider-tplink-omada .
      go test -mod=vendor -c -o /tmp/client-tests ./internal/client
      go test -mod=vendor -c -o /tmp/provider-tests ./internal/provider
      sha256sum go.mod go.sum vendor/modules.txt internal/client/*.go internal/provider/*.go internal/resources/*.go
    ' > "$runtime/build.log" 2>&1
    # Vendored modules: compilation and tests both run on the internal network.
    set +e
    docker exec --env-file "$runtime/runner.env" -w /tmp/provider/internal/provider "$runner" \
      /tmp/provider-tests -test.run "${LAB_ACCEPTANCE_RUN:-^TestAcc(DataSource.*|ResourceNetwork_CRUD|ResourcePortProfile_CRUD|WirelessNetwork.*)$|^TestControllerLab}" \
      -test.v -test.timeout=300s > "$runtime/acceptance.log" 2>&1
    acceptance_status=$?
    docker exec --env-file "$runtime/runner.env" -w /tmp/provider/internal/client "$runner" \
      /tmp/client-tests -test.run "${LAB_CLIENT_RUN:-^TestControllerLab}" -test.v -test.timeout=600s > "$runtime/client-probes.log" 2>&1
    client_status=$?
    set -e
    printf 'acceptance_exit=%s\nclient_exit=%s\n' "$acceptance_status" "$client_status" | tee "$runtime/test-status.txt"
    echo "Logs: $runtime (inspect and redact before publishing)."
    [[ $acceptance_status == 0 && $client_status == 0 ]]
    ;;
  down)
    # Remove only resources owned by this lab; other Docker state is untouched.
    if docker container inspect "$runner" >/dev/null 2>&1; then
      label=$(docker inspect -f '{{index .Config.Labels "omada.provider.lab"}}' "$runner")
      [[ $label == true ]] || { echo 'Runner is not owned by this lab.' >&2; exit 2; }
      docker rm -f "$runner"
    fi
    "${compose[@]}" down --volumes --timeout 45
    python3 - "$runtime" <<'PY'
from pathlib import Path
import sys
root = Path(sys.argv[1])
for name in ('credentials.json', 'runner.env', 'last-response.json', 'd-other.patch', 'ssid-followup.patch'):
    (root / name).unlink(missing_ok=True)
PY
    ;;
  *)
    echo 'Usage: dev/controller-lab/run.sh {up|probe|test|down}' >&2
    exit 2
    ;;
esac
