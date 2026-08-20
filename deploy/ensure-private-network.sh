#!/usr/bin/env bash
# Ensure kalke-auth_default exists with subnet 172.18.10.0/24 for WARP static DB IPs.
# If the live network has a different subnet/IPAM, tear down sibling stacks first,
# remove the network, and let `make aws-up` recreate it.
set -euo pipefail

NET_NAME="${NET_NAME:-kalke-auth_default}"
WANT_SUBNET="${WANT_SUBNET:-172.18.10.0/24}"
AUTH_DIR="${AUTH_DIR:-${HOME}/kalke-auth}"
EBANK_DIR="${EBANK_DIR:-${HOME}/e-bank-api}"
PDE_DIR="${PDE_DIR:-${HOME}/personal-document-extractor}"

subnet_ok() {
  docker network inspect "$NET_NAME" --format '{{range .IPAM.Config}}{{.Subnet}}{{end}}' 2>/dev/null \
    | grep -qx "$WANT_SUBNET"
}

if docker network inspect "$NET_NAME" >/dev/null 2>&1 && subnet_ok; then
  echo "==> Network ${NET_NAME} already on ${WANT_SUBNET}"
  exit 0
fi

echo "==> Recreating ${NET_NAME} with ${WANT_SUBNET} (sibling stacks will briefly stop)"

down_stack() {
  local dir="$1"
  local file="$2"
  local profile="${3:-}"
  if [[ -d "$dir" && -f "$dir/$file" && -f "$dir/prod.env" ]]; then
    echo "==> compose down in $dir"
    (
      cd "$dir"
      if [[ -n "$profile" ]]; then
        docker compose -f "$file" --env-file prod.env --profile "$profile" down || true
      else
        docker compose -f "$file" --env-file prod.env down || true
      fi
    )
  fi
}

down_stack "$EBANK_DIR" docker-compose.aws.yml
down_stack "$PDE_DIR" docker-compose.aws.yml
down_stack "$AUTH_DIR" docker-compose.aws.yml tunnel

# Detach any leftover containers still on the network.
if docker network inspect "$NET_NAME" >/dev/null 2>&1; then
  mapfile -t ids < <(docker network inspect "$NET_NAME" -f '{{range $k,$v := .Containers}}{{$k}}{{"\n"}}{{end}}' 2>/dev/null || true)
  for id in "${ids[@]:-}"; do
    [[ -z "$id" ]] && continue
    echo "==> disconnect $id from ${NET_NAME}"
    docker network disconnect -f "$NET_NAME" "$id" || true
  done
  docker network rm "$NET_NAME" || true
fi

echo "==> ${NET_NAME} removed (or absent); aws-up will recreate with static subnet"
