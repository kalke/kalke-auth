#!/usr/bin/env bash
# Add Cloudflare Tunnel private /32 routes for Kalke Docker Postgres + EC2 SSH.
# Requires: CLOUDFLARE_API_TOKEN (Account: Cloudflare Tunnel:Edit)
#           CLOUDFLARE_ACCOUNT_ID
# Optional: TUNNEL_ID (auto-detected if unset)
# Optional: EC2_PRIVATE_IP (default 172.31.29.66)
#
# Usage:
#   export CLOUDFLARE_API_TOKEN=...
#   export CLOUDFLARE_ACCOUNT_ID=...
#   bash deploy/cloudflare-warp-private-routes.sh
set -euo pipefail

ACCOUNT_ID="${CLOUDFLARE_ACCOUNT_ID:?set CLOUDFLARE_ACCOUNT_ID}"
TOKEN="${CLOUDFLARE_API_TOKEN:?set CLOUDFLARE_API_TOKEN}"
EC2_PRIVATE_IP="${EC2_PRIVATE_IP:-172.31.29.66}"
API="https://api.cloudflare.com/client/v4"

auth_hdr=(-H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json")

if [[ -z "${TUNNEL_ID:-}" ]]; then
  echo "==> Listing tunnels"
  tunnels_json=$(curl -fsS "${auth_hdr[@]}" \
    "${API}/accounts/${ACCOUNT_ID}/cfd_tunnel?is_deleted=false")
  TUNNEL_ID=$(ACCOUNT_ID="$ACCOUNT_ID" python3 -c '
import json, os, sys
d = json.load(sys.stdin)
rows = d.get("result") or []
if not rows:
    raise SystemExit("no tunnels found")
prefer = [t for t in rows if any(x in (t.get("name") or "").lower() for x in ("kalke", "auth", "keycloak"))]
t = (prefer or rows)[0]
print(t["id"], file=sys.stdout)
print(f"Using tunnel {t.get('name')} ({t['id']})", file=sys.stderr)
' <<<"$tunnels_json")
fi

echo "TUNNEL_ID=${TUNNEL_ID}"

add_route() {
  local network="$1" comment="$2"
  echo "==> Route ${network} (${comment})"
  local payload
  payload=$(NETWORK="$network" TUNNEL_ID="$TUNNEL_ID" COMMENT="$comment" python3 -c '
import json, os
print(json.dumps({
  "network": os.environ["NETWORK"],
  "tunnel_id": os.environ["TUNNEL_ID"],
  "comment": os.environ["COMMENT"],
}))
')
  resp=$(curl -sS "${auth_hdr[@]}" \
    -X POST "${API}/accounts/${ACCOUNT_ID}/teamnet/routes" \
    --data "$payload")
  python3 -c '
import json, sys
d = json.load(sys.stdin)
if d.get("success"):
    r = d["result"]
    print("  ok", r.get("network"), r.get("id"))
    raise SystemExit(0)
errs = d.get("errors") or []
msgs = " ".join(e.get("message", "") for e in errs).lower()
if "already" in msgs or "duplicate" in msgs or "conflict" in msgs:
    print("  exists:", msgs)
    raise SystemExit(0)
print(json.dumps(d, indent=2))
raise SystemExit(1)
' <<<"$resp"
}

add_route "172.18.10.10/32" "kalke-auth postgres (WARP/DBeaver)"
add_route "172.18.10.11/32" "e-bank-api postgres (WARP/DBeaver)"
add_route "172.18.10.12/32" "pde postgres (WARP/DBeaver)"
add_route "${EC2_PRIVATE_IP}/32" "kalke EC2 SSH via WARP"

cat <<EOF

Next (dashboard — Split Tunnels + Gateway):
1. Zero Trust → Settings → WARP Client → Device profiles → Default
2. Split Tunnels (Exclude mode): ensure 172.18.10.0/24 and ${EC2_PRIVATE_IP}/32 are NOT excluded
   (default Exclude of 172.16.0.0/12 blocks them — tighten that range or use Include mode).
3. Gateway → Network policies: Allow TCP 5432 → 172.18.10.10-12; Allow TCP 22 → ${EC2_PRIVATE_IP}
   for your identity (optional if default-allow).
4. On Mac: open Cloudflare WARP → log into Zero Trust org → Connect.
5. Test: nc -vz 172.18.10.11 5432

EOF
