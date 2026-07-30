# kalke-auth

Encapsulated **OIDC Identity Provider** for Kalke apps.

Apps (including [personal-document-extractor](../personal-document-extractor)) talk only to a stable OIDC issuer. They do **not** configure Keycloak-specific URLs, admin APIs, or themes.

Under the hood (internal Docker network only):

1. **Postgres** — Keycloak database  
2. **Keycloak** — IdP implementation (not published)  
3. **Caddy** — public reverse proxy (issuer façade + TLS-ready later)

```text
App / SPA  ──OIDC──►  Caddy (:8443)  ──►  Keycloak  ──►  Postgres
```

## Quick start

```bash
cp .env.example .env
make up
make jwks    # must succeed via the proxy
```

| Value | Default |
|---|---|
| Issuer | `http://localhost:8443/realms/kalke` |
| JWKS | `{issuer}/protocol/openid-connect/certs` |
| Audience (API) | `personal-document-extractor` |
| Demo user | `demo@kalke.local` / `DemoPass123!` |
| Admin UI | `http://localhost:8443/admin/` (loopback only) |

## Wire an app (OIDC only)

```bash
OIDC_ISSUER=http://localhost:8443/realms/kalke
OIDC_AUDIENCE=personal-document-extractor
```

The app validates JWTs locally with JWKS. No per-request call to this stack is required after keys are cached.

### Consumers in Docker (WSL / Desktop)

`localhost` inside a container is not the host. Prefer running the API on the host for JWT smoke, **or** set the same reachable issuer on both sides (e.g. `host.docker.internal`) and align `KC_HOSTNAME` + `OIDC_ISSUER`.

## Clients in realm `kalke`

| Client | Type | Purpose |
|---|---|---|
| `personal-document-extractor` | bearer-only | API **audience** |
| `kalke-spa` | public + PKCE | Future website |
| `kalke-cli` | confidential + password grant | **Dev smoke only** |

Realm roles (mapped into access-token claim `permissions`):

- `extract:write`
- `keys:manage`
- `admin`

## Dev token (smoke)

```bash
TOKEN=$(make -s token)
curl -sS http://localhost:8080/v1/me -H "Authorization: Bearer $TOKEN"
```

Password grant exists only on `kalke-cli` for local testing. Production websites must use Authorization Code + PKCE (`kalke-spa`).

## Make targets

```bash
make help
make up | down | logs | ps
make jwks
make token
```

## What this is / is not

**Is:** IdP product boundary — reverse proxy + Keycloak + realm import.  
**Is not:** a custom Go auth BFF, a password store for product APIs, or HA Keycloak.

API keys (M2M) stay in each product API (e.g. `pde_live_…` in the extractor). This repo covers **human** OIDC login.

## License

Apache-2.0
