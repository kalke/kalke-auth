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
		/opt/keycloak/bin/kcadm.sh update "clients/${CID}" -r kalke \
			-s "secret=${KC_BFF_CLIENT_SECRET}" \
			-s "standardFlowEnabled=true" \
			-s "directAccessGrantsEnabled=true" \
			-s 'redirectUris=["https://auth.kalke.dev/v1/auth/callback"]' \
			-s 'webOrigins=["https://auth.kalke.dev","https://kalke.dev","https://www.kalke.dev"]'
	else
		echo "kalke-bff client not found" >&2
		exit 1
	fi
	# Public OIDC issuer stays on auth.kalke.dev while KC_HOSTNAME is the Tunnel URL.
	FRONTEND_URL="${KC_REALM_FRONTEND_URL:-https://auth.kalke.dev}"
	echo "setting kalke realm frontendUrl=${FRONTEND_URL}"
	/opt/keycloak/bin/kcadm.sh update realms/kalke -s "attributes.frontendUrl=${FRONTEND_URL}"

	# Silent first-broker login: create user if new, auto-link if email already exists.
	# Avoids Keycloak "review profile" / "confirm link" screens in the OAuth redirect.
	ensure_kalke_first_broker_flow() {
		local flow='kalke first broker login'
		local handle="${flow} Handle Existing Account"
		local execs review confirm verify auto

		if ! /opt/keycloak/bin/kcadm.sh get authentication/flows -r kalke --fields alias --format csv --noquotes \
			| grep -Fxq "$flow"; then
			echo "creating ${flow}"
			/opt/keycloak/bin/kcadm.sh create authentication/flows/first%20broker%20login/copy -r kalke \
				-s "newName=${flow}"
		fi

		execs="$(/opt/keycloak/bin/kcadm.sh get "authentication/flows/${flow// /%20}/executions" -r kalke)"

		set_exec_req() {
			local id="$1" req="$2"
			[[ -n "$id" ]] || return 0
			/opt/keycloak/bin/kcadm.sh update "authentication/flows/${flow// /%20}/executions" -r kalke \
				-b "{\"id\":\"${id}\",\"requirement\":\"${req}\"}" >/dev/null
		}

		review="$(printf '%s' "$execs" | python3 -c 'import json,sys; print(next((e["id"] for e in json.load(sys.stdin) if e.get("providerId")=="idp-review-profile"), ""))')"
		confirm="$(printf '%s' "$execs" | python3 -c 'import json,sys; print(next((e["id"] for e in json.load(sys.stdin) if e.get("providerId")=="idp-confirm-link"), ""))')"
		verify="$(printf '%s' "$execs" | python3 -c 'import json,sys; print(next((e["id"] for e in json.load(sys.stdin) if (e.get("displayName") or "").endswith("Account verification options")), ""))')"
		set_exec_req "$review" DISABLED
		set_exec_req "$confirm" DISABLED
		set_exec_req "$verify" DISABLED

		execs="$(/opt/keycloak/bin/kcadm.sh get "authentication/flows/${flow// /%20}/executions" -r kalke)"
		auto="$(printf '%s' "$execs" | python3 -c 'import json,sys; print(next((e["id"] for e in json.load(sys.stdin) if e.get("providerId")=="idp-auto-link"), ""))')"
		if [[ -z "$auto" ]]; then
			echo "adding idp-auto-link to ${handle}"
			/opt/keycloak/bin/kcadm.sh create "authentication/flows/${handle// /%20}/executions/execution" -r kalke \
				-s provider=idp-auto-link >/dev/null
			execs="$(/opt/keycloak/bin/kcadm.sh get "authentication/flows/${flow// /%20}/executions" -r kalke)"
			auto="$(printf '%s' "$execs" | python3 -c 'import json,sys; print(next((e["id"] for e in json.load(sys.stdin) if e.get("providerId")=="idp-auto-link"), ""))')"
		fi
		set_exec_req "$auto" REQUIRED
		echo "first broker flow ready: ${flow}"
	}
	ensure_kalke_first_broker_flow

	# Ensure Google IdP stays enabled when secrets are present on the host.
	if [[ -n "${GOOGLE_IDP_CLIENT_ID:-}" && -n "${GOOGLE_IDP_CLIENT_SECRET:-}" ]]; then
		echo "configuring google identity provider"
		if /opt/keycloak/bin/kcadm.sh get identity-provider/instances/google -r kalke >/dev/null 2>&1; then
			/opt/keycloak/bin/kcadm.sh update identity-provider/instances/google -r kalke \
				-s enabled=true \
				-s trustEmail=true \
				-s 'firstBrokerLoginFlowAlias=kalke first broker login' \
				-s "config.clientId=${GOOGLE_IDP_CLIENT_ID}" \
				-s "config.clientSecret=${GOOGLE_IDP_CLIENT_SECRET}" \
				-s "config.defaultScope=openid profile email" \
				-s "config.syncMode=IMPORT"
		else
			/opt/keycloak/bin/kcadm.sh create identity-provider/instances -r kalke \
				-s alias=google \
				-s providerId=google \
				-s enabled=true \
				-s displayName=Google \
				-s trustEmail=true \
				-s storeToken=false \
				-s 'firstBrokerLoginFlowAlias=kalke first broker login' \
				-s "config.clientId=${GOOGLE_IDP_CLIENT_ID}" \
				-s "config.clientSecret=${GOOGLE_IDP_CLIENT_SECRET}" \
				-s "config.defaultScope=openid profile email" \
				-s "config.syncMode=IMPORT" \
				-s "config.useJwksUrl=true"
		fi
	fi

	if [[ -n "${GITHUB_IDP_CLIENT_ID:-}" && -n "${GITHUB_IDP_CLIENT_SECRET:-}" ]]; then
		echo "configuring github identity provider"
		if /opt/keycloak/bin/kcadm.sh get identity-provider/instances/github -r kalke >/dev/null 2>&1; then
			/opt/keycloak/bin/kcadm.sh update identity-provider/instances/github -r kalke \
				-s enabled=true \
				-s trustEmail=true \
				-s 'firstBrokerLoginFlowAlias=kalke first broker login' \
				-s "config.clientId=${GITHUB_IDP_CLIENT_ID}" \
				-s "config.clientSecret=${GITHUB_IDP_CLIENT_SECRET}" \
				-s "config.defaultScope=user:email read:user" \
				-s "config.syncMode=IMPORT"
		else
			/opt/keycloak/bin/kcadm.sh create identity-provider/instances -r kalke \
				-s alias=github \
				-s providerId=github \
				-s enabled=true \
				-s displayName=GitHub \
				-s trustEmail=true \
				-s storeToken=false \
				-s 'firstBrokerLoginFlowAlias=kalke first broker login' \
				-s "config.clientId=${GITHUB_IDP_CLIENT_ID}" \
				-s "config.clientSecret=${GITHUB_IDP_CLIENT_SECRET}" \
				-s "config.defaultScope=user:email read:user" \
				-s "config.syncMode=IMPORT"
		fi
	fi
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
