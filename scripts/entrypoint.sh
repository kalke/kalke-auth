#!/usr/bin/env bash
set -euo pipefail

KC_HTTP_PORT="${KC_HTTP_PORT:-8081}"
export KC_HTTP_PORT

echo "starting keycloak on :${KC_HTTP_PORT}"
/opt/keycloak/bin/kc.sh start --import-realm &
KC_PID=$!

ready=0
for _ in $(seq 1 90); do
	if (echo >"/dev/tcp/127.0.0.1/${KC_HTTP_PORT}") >/dev/null 2>&1; then
		# port open; give health a moment
		sleep 1
		ready=1
		break
	fi
	if ! kill -0 "$KC_PID" 2>/dev/null; then
		echo "keycloak exited early" >&2
		exit 1
	fi
	sleep 2
done
if [[ "$ready" != "1" ]]; then
	echo "keycloak not ready" >&2
	exit 1
fi

if [[ -n "${KC_BOOTSTRAP_ADMIN_USERNAME:-}" && -n "${KC_BOOTSTRAP_ADMIN_PASSWORD:-}" && -n "${KC_BFF_CLIENT_SECRET:-}" ]]; then
	echo "configuring kalke-bff client secret"
	/opt/keycloak/bin/kcadm.sh config credentials \
		--server "http://127.0.0.1:${KC_HTTP_PORT}" \
		--realm master \
		--user "$KC_BOOTSTRAP_ADMIN_USERNAME" \
		--password "$KC_BOOTSTRAP_ADMIN_PASSWORD"
	CID="$(/opt/keycloak/bin/kcadm.sh get clients -r kalke -q clientId=kalke-bff --fields id --format csv --noquotes | head -n1)"
	if [[ -n "$CID" && "$CID" != "id" ]]; then
		/opt/keycloak/bin/kcadm.sh update "clients/${CID}" -r kalke -s "secret=${KC_BFF_CLIENT_SECRET}"
	else
		echo "kalke-bff client not found" >&2
		exit 1
	fi
	# Public OIDC issuer stays on auth.kalke.dev while KC_HOSTNAME is the Tunnel URL.
	FRONTEND_URL="${KC_REALM_FRONTEND_URL:-https://auth.kalke.dev}"
	echo "setting kalke realm frontendUrl=${FRONTEND_URL}"
	/opt/keycloak/bin/kcadm.sh update realms/kalke -s "attributes.frontendUrl=${FRONTEND_URL}"
fi

export KC_INTERNAL_URL="${KC_INTERNAL_URL:-http://127.0.0.1:${KC_HTTP_PORT}}"
export HTTP_ADDR="${HTTP_ADDR:-:8080}"

echo "starting auth api on ${HTTP_ADDR}"
/opt/kalke/api &
API_PID=$!

cleanup() {
	kill "$API_PID" "$KC_PID" 2>/dev/null || true
}
trap cleanup EXIT

# Exit if either process dies.
while kill -0 "$KC_PID" 2>/dev/null && kill -0 "$API_PID" 2>/dev/null; do
	sleep 2
done
echo "a process exited" >&2
exit 1
