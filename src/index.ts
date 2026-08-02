/**
 * Optional Cloudflare Worker edge for auth.kalke.dev.
 *
 * Preferred prod path: point DNS (A/AAAA) at the AWS EC2 and let Caddy terminate TLS.
 * This Worker is only needed if you want an orange-cloud proxy in front of the VM.
 *
 * Set secret/var ORIGIN_URL to the upstream, e.g. https://AUTH_VM_IP or http://10.0.0.x:8080
 * (when using HTTPS to a raw IP, prefer DNS-only + Caddy instead).
 */

export interface Env {
	/** Upstream kalke-auth (Caddy or :8080 on the AWS EC2). */
	ORIGIN_URL: string;
}

export default {
	async fetch(request: Request, env: Env): Promise<Response> {
		const origin = (env.ORIGIN_URL || "").replace(/\/$/, "");
		if (!origin) {
			return new Response(
				"auth origin not configured — set ORIGIN_URL or point DNS at the AWS EC2",
				{ status: 503 },
			);
		}

		const incoming = new URL(request.url);
		const upstream = new URL(origin);
		upstream.pathname = incoming.pathname;
		upstream.search = incoming.search;

		const headers = new Headers(request.headers);
		headers.set("X-Forwarded-Host", incoming.host);
		headers.set("X-Forwarded-Proto", incoming.protocol.replace(":", ""));
		headers.delete("host");

		return fetch(
			new Request(upstream.toString(), {
				method: request.method,
				headers,
				body: request.body,
				redirect: "manual",
			}),
		);
	},
};
