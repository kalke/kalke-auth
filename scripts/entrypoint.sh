#!/usr/bin/env bash
set -euo pipefail

KC_HTTP_PORT="${KC_HTTP_PORT:-8081}"
export KC_HTTP_PORT

echo "starting keycloak on :${KC_HTTP_PORT}"
/opt/keycloak/bin/kc.sh start --import-realm &
KC_PID=$!

# Management health (KC_HEALTH_ENABLED) — TCP on 8081 opens before bootstrap finishes;
# kcadm against a bootstrapping server returns 503 and would crash the entrypoint.
KC_MGMT_PORT="${KC_HTTP_MANAGEMENT_PORT:-9000}"
kc_http_ready() {
	local port="$1" path="$2" status=""
	exec 3<>"/dev/tcp/127.0.0.1/${port}" 2>/dev/null || return 1
	printf 'GET %s HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\n\r\n' "$path" >&3
	status="$(head -n1 <&3 2>/dev/null || true)"
	exec 3>&- 3<&- 2>/dev/null || true
	[[ "$status" == *" 200 "* ]]
}

ready=0
for _ in $(seq 1 120); do
	if kc_http_ready "$KC_MGMT_PORT" /health/ready; then
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
	echo "keycloak not ready (management /health/ready on :${KC_MGMT_PORT})" >&2
	exit 1
fi

if [[ -n "${KC_BOOTSTRAP_ADMIN_USERNAME:-}" && -n "${KC_BOOTSTRAP_ADMIN_PASSWORD:-}" && -n "${KC_BFF_CLIENT_SECRET:-}" ]]; then
	echo "configuring kalke-bff client secret"
	kcadm_ok=0
	for _ in $(seq 1 30); do
		if /opt/keycloak/bin/kcadm.sh config credentials \
			--server "http://127.0.0.1:${KC_HTTP_PORT}" \
			--realm master \
			--user "$KC_BOOTSTRAP_ADMIN_USERNAME" \
			--password "$KC_BOOTSTRAP_ADMIN_PASSWORD"; then
			kcadm_ok=1
			break
		fi
		sleep 2
	done
	if [[ "$kcadm_ok" != "1" ]]; then
		echo "kcadm login failed after retries" >&2
		exit 1
	fi
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
	# Pure shell (Keycloak image has no python3).
	ensure_kalke_first_broker_flow() {
		local flow='kalke first broker login'
		local handle="${flow} Handle Existing Account"
		local execs review confirm verify auto

		find_exec_id() {
			# $1=field (providerId|displayName) $2=exact or substring value
			# Pure bash — Keycloak image has neither awk nor python3.
			local field="$1" value="$2" id="" line
			while IFS= read -r line || [[ -n "$line" ]]; do
				if [[ "$line" =~ \"id\"[[:space:]]*:[[:space:]]*\"([^\"]+)\" ]]; then
					id="${BASH_REMATCH[1]}"
				fi
				if [[ "$line" == *"\"${field}\""* && "$line" == *"${value}"* ]]; then
					printf '%s\n' "$id"
					return 0
				fi
			done <<<"$execs"
			return 0
		}

		set_exec_req() {
			local id="$1" req="$2"
			[[ -n "$id" ]] || return 0
			/opt/keycloak/bin/kcadm.sh update "authentication/flows/${flow// /%20}/executions" -r kalke \
				-b "{\"id\":\"${id}\",\"requirement\":\"${req}\"}" >/dev/null
		}

		if ! /opt/keycloak/bin/kcadm.sh get authentication/flows -r kalke --fields alias --format csv --noquotes \
			| grep -Fxq "$flow"; then
			echo "creating ${flow}"
			/opt/keycloak/bin/kcadm.sh create authentication/flows/first%20broker%20login/copy -r kalke \
				-s "newName=${flow}"
		fi

		execs="$(/opt/keycloak/bin/kcadm.sh get "authentication/flows/${flow// /%20}/executions" -r kalke)"
		review="$(find_exec_id providerId idp-review-profile)"
		confirm="$(find_exec_id providerId idp-confirm-link)"
		verify="$(find_exec_id displayName 'Account verification options')"
		set_exec_req "$review" DISABLED
		set_exec_req "$confirm" DISABLED
		set_exec_req "$verify" DISABLED

		execs="$(/opt/keycloak/bin/kcadm.sh get "authentication/flows/${flow// /%20}/executions" -r kalke)"
		auto="$(find_exec_id providerId idp-auto-link)"
		if [[ -z "$auto" ]]; then
			echo "adding idp-auto-link to ${handle}"
			/opt/keycloak/bin/kcadm.sh create "authentication/flows/${handle// /%20}/executions/execution" -r kalke \
				-s provider=idp-auto-link >/dev/null
			execs="$(/opt/keycloak/bin/kcadm.sh get "authentication/flows/${flow// /%20}/executions" -r kalke)"
			auto="$(find_exec_id providerId idp-auto-link)"
		fi
		set_exec_req "$auto" REQUIRED
		echo "first broker flow ready: ${flow}"
	}
	if ! ensure_kalke_first_broker_flow; then
		echo "warning: could not configure first broker login flow" >&2
	fi

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
