#!/usr/bin/env bash
set -euo pipefail
umask 077

ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
RUNTIME="$ROOT/.es216g-lab"
COMPOSE=(docker compose -p omada-es216g-physical -f "$ROOT/dev/es216g-lab/compose.yaml")
CONTAINER=omada-es216g-controller

if [[ -z ${ES216G_CONTROLLER_IP:-} ]]; then
  ES216G_CONTROLLER_IP=$(python3 -c 'import socket; s=socket.socket(socket.AF_INET, socket.SOCK_DGRAM); s.connect(("192.168.4.22", 443)); print(s.getsockname()[0]); s.close()')
fi
export ES216G_CONTROLLER_IP

owned_container() {
  docker inspect "$CONTAINER" >/dev/null 2>&1 || return 1
  [[ $(docker inspect -f '{{index .Config.Labels "omada.provider.es216g"}}' "$CONTAINER") == true ]]
}

case "${1:-}" in
  up)
    mkdir -p "$RUNTIME"
    chmod 700 "$RUNTIME"
    if docker inspect "$CONTAINER" >/dev/null 2>&1 && ! owned_container; then
      echo "Refusing to touch existing unowned container: $CONTAINER" >&2
      exit 2
    fi
    "${COMPOSE[@]}" up -d
    python3 "$ROOT/dev/es216g-lab/controller.py" initialize
    ;;
  status)
    owned_container || { echo "Physical-test controller is not running."; exit 1; }
    python3 "$ROOT/dev/es216g-lab/controller.py" status
    ;;
  devices)
    owned_container || { echo "Physical-test controller is not running." >&2; exit 1; }
    python3 "$ROOT/dev/es216g-lab/controller.py" devices
    ;;
  environment)
    owned_container || { echo "Physical-test controller is not running." >&2; exit 1; }
    python3 "$ROOT/dev/es216g-lab/controller.py" environment
    echo "Private provider environment written under .es216g-lab/ (mode 0600)."
    ;;
  credentials-csv)
    owned_container || { echo "Physical-test controller is not running." >&2; exit 1; }
    python3 "$ROOT/dev/es216g-lab/controller.py" credentials-csv
    ;;
  shred-credentials-csv)
    if [[ -f "$RUNTIME/controller-credential.csv" ]]; then
      shred -u "$RUNTIME/controller-credential.csv"
      echo "Temporary controller credential CSV securely deleted."
    else
      echo "No temporary controller credential CSV exists."
    fi
    ;;
  down)
    if docker inspect "$CONTAINER" >/dev/null 2>&1; then
      owned_container || { echo "Refusing to stop unowned container: $CONTAINER" >&2; exit 2; }
    fi
    "${COMPOSE[@]}" down --timeout 45
    echo "Stopped the physical-test controller; persistent controller data was retained."
    ;;
  *)
    echo "Usage: $0 {up|status|devices|environment|credentials-csv|shred-credentials-csv|down}" >&2
    exit 2
    ;;
esac
