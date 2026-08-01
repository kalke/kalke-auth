import { Container, getContainer } from "@cloudflare/containers";

export interface Env {
	KEYCLOAK: DurableObjectNamespace<KeycloakContainer>;
	KC_HOSTNAME: string;
	KC_DB: string;
	KC_HTTP_ENABLED: string;
	KC_HOSTNAME_STRICT: string;
	KC_PROXY_HEADERS: string;
	KC_HEALTH_ENABLED: string;
	KC_DB_URL: string;
	KC_DB_USERNAME: string;
	KC_DB_PASSWORD: string;
	KC_BOOTSTRAP_ADMIN_USERNAME: string;
	KC_BOOTSTRAP_ADMIN_PASSWORD: string;
}

function keycloakEnvVars(env: Env): Record<string, string> {
	return {
		KC_DB: env.KC_DB || "postgres",
		KC_DB_URL: env.KC_DB_URL,
		KC_DB_USERNAME: env.KC_DB_USERNAME,
		KC_DB_PASSWORD: env.KC_DB_PASSWORD,
		KC_HOSTNAME: env.KC_HOSTNAME || "https://auth.kalke.dev",
		KC_HTTP_ENABLED: env.KC_HTTP_ENABLED || "true",
		KC_HOSTNAME_STRICT: env.KC_HOSTNAME_STRICT || "true",
		KC_PROXY_HEADERS: env.KC_PROXY_HEADERS || "xforwarded",
		KC_HEALTH_ENABLED: env.KC_HEALTH_ENABLED || "true",
		KC_BOOTSTRAP_ADMIN_USERNAME: env.KC_BOOTSTRAP_ADMIN_USERNAME || "admin",
		KC_BOOTSTRAP_ADMIN_PASSWORD: env.KC_BOOTSTRAP_ADMIN_PASSWORD,
	};
}

/** Admin console and master realm stay off the public hostname. */
function isBlockedPublicPath(pathname: string): boolean {
	const p = pathname.toLowerCase();
	return (
		p === "/admin" ||
		p.startsWith("/admin/") ||
		p === "/realms/master" ||
		p.startsWith("/realms/master/")
	);
}

export class KeycloakContainer extends Container<Env> {
	defaultPort = 8080;
	// Keep the IdP warm enough for interactive sandbox login.
	sleepAfter = "30m";

	override onStart(): void {
		this.envVars = keycloakEnvVars(this.env);
	}
}

export default {
	async fetch(request: Request, env: Env): Promise<Response> {
		const url = new URL(request.url);
		if (url.pathname === "/api/health") {
			return Response.json({ ok: true, service: "kalke-auth" });
		}
		if (isBlockedPublicPath(url.pathname)) {
			return new Response("Not Found", { status: 404 });
		}

		const container = getContainer(env.KEYCLOAK, "primary");
		await container.startAndWaitForPorts({
			startOptions: {
				envVars: keycloakEnvVars(env),
			},
			cancellationOptions: {
				// Keycloak cold start against Neon can take a while.
				portReadyTimeoutMS: 180_000,
			},
		});
		return container.fetch(request);
	},
};
