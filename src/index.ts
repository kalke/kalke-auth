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
	DATABASE_URL: string;
	REDIS_ADDR: string;
	REDIS_PASSWORD: string;
	REDIS_TLS: string;
	SESSION_SECRET: string;
	TOKEN_HASH_PEPPER: string;
	INTROSPECT_SECRET: string;
	KC_BFF_CLIENT_ID: string;
	KC_BFF_CLIENT_SECRET: string;
	CORS_ORIGINS: string;
	COOKIE_DOMAIN: string;
}

function containerEnvVars(env: Env): Record<string, string> {
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
		KC_HTTP_PORT: "8081",
		HTTP_ADDR: ":8080",
		KC_INTERNAL_URL: "http://127.0.0.1:8081",
		KC_PUBLIC_ISSUER: "https://auth.kalke.dev/realms/kalke",
		DATABASE_URL: env.DATABASE_URL,
		REDIS_ADDR: env.REDIS_ADDR,
		REDIS_PASSWORD: env.REDIS_PASSWORD || "",
		REDIS_TLS: env.REDIS_TLS || "true",
		SESSION_SECRET: env.SESSION_SECRET,
		TOKEN_HASH_PEPPER: env.TOKEN_HASH_PEPPER,
		INTROSPECT_SECRET: env.INTROSPECT_SECRET,
		KC_BFF_CLIENT_ID: env.KC_BFF_CLIENT_ID || "kalke-bff",
		KC_BFF_CLIENT_SECRET: env.KC_BFF_CLIENT_SECRET,
		CORS_ORIGINS: env.CORS_ORIGINS || "https://kalke.dev,https://www.kalke.dev",
		COOKIE_DOMAIN: env.COOKIE_DOMAIN || ".kalke.dev",
	};
}

export class KeycloakContainer extends Container<Env> {
	defaultPort = 8080;
	sleepAfter = "30m";

	override onStart(): void {
		this.envVars = containerEnvVars(this.env);
	}
}

export default {
	async fetch(request: Request, env: Env): Promise<Response> {
		const container = getContainer(env.KEYCLOAK, "primary");
		await container.startAndWaitForPorts({
			startOptions: {
				envVars: containerEnvVars(env),
			},
			cancellationOptions: {
				portReadyTimeoutMS: 240_000,
			},
		});
		return container.fetch(request);
	},
};
