# kalke-auth

Encapsulated **OIDC Identity Provider** for Kalke apps.

Apps ([personal-document-extractor](https://github.com/kalke/personal-document-extractor), [e-bank-api](https://github.com/kalke/e-bank-api), [kalke.dev sandbox](https://github.com/kalke/kalke)) talk only to a stable OIDC issuer. They do **not** configure Keycloak-specific URLs, admin APIs, or themes.

Under the hood (Docker network `kalke-auth`):

1. **kc-db** — Keycloak database (service name is *not* `postgres`, to avoid DNS clashes with other stacks)  
2. **Keycloak** — IdP implementation (not published on the host)  
3. **Caddy** — public reverse proxy (issuer façade)

Production: Cloudflare Containers on **`auth.kalke.dev`** with Neon Postgres — see [DEPLOY.md](DEPLOY.md).

```text
App / SPA / M2M  ──OIDC──►  Caddy (:8443 on host)  ──►  Keycloak  ──►  kc-db

Docker consumers on network kalke-auth reach JWKS via http://caddy:8443
while JWT iss stays http://localhost:8443/realms/kalke
```

## Quick start

```bash
cp .env.example .env
make up
make jwks       # must succeed via the proxy
make token      # human (demo user) access token
make m2m-token  # PDE machine client_credentials token
make ebank-m2m-token
```

| Value | Default |
|---|---|
| Issuer (local) | `http://localhost:8443/realms/kalke` |
| Issuer (prod) | `https://auth.kalke.dev/realms/kalke` |
| JWKS | `{issuer}/protocol/openid-connect/certs` |
| Audiences | `personal-document-extractor`, `e-bank-api` |
| Demo user | `demo@kalke.local` / `DemoPass123!` |
| M2M (PDE) | `pde-m2m` / `pde-m2m-dev-secret` |
| M2M (e-bank) | `ebank-m2m` / `ebank-m2m-dev-secret` |
| Admin UI | `http://localhost:8443/admin/` (loopback only) |
| Docker network | `kalke-auth` |

### Re-importing the realm

Keycloak import uses `IGNORE_EXISTING`. After changing `keycloak/kalke-realm.json`, wipe the volume so the realm is recreated:

```bash
make destroy
make up
```

## Wire an app (OIDC only)

**Host / Postman clients:**

```bash
OIDC_ISSUER=http://localhost:8443/realms/kalke
OIDC_AUDIENCE=personal-document-extractor   # or e-bank-api
```

**App container on network `kalke-auth`:**

```bash
OIDC_ISSUER=http://localhost:8443/realms/kalke          # must match JWT iss
OIDC_DISCOVERY_URL=http://caddy:8443/realms/kalke       # reachable JWKS/discovery
OIDC_AUDIENCE=personal-document-extractor               # or e-bank-api
```

The app validates JWTs locally with JWKS. Env vars for this IdP DB are namespaced (`KC_DB_USER`, …) so they do not collide with product `POSTGRES_*` variables.

## Clients in realm `kalke`

| Client | Type | Purpose |
|---|---|---|
| `personal-document-extractor` | bearer-only | PDE API **audience** |
| `e-bank-api` | bearer-only | E-Bank API **audience** |
| `kalke-spa` | public + PKCE | kalke.dev sandbox (+ local Vite) |
| `kalke-cli` | confidential + password grant | **Dev human smoke only** |
| `pde-m2m` | confidential + service account | PDE **M2M** |
| `ebank-m2m` | confidential + service account | E-Bank **M2M** |

Realm roles (mapped into access-token claim `permissions`):

- `extract:write`
- `bank:write`
- `admin`

## Tokens

**Human (dev):**

```bash
TOKEN=$(make -s token)
```

**M2M:**

```bash
TOKEN=$(make -s m2m-token)
curl -sS -X POST "http://localhost:8080/v1/extract?doc_type=identity_document" \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@./documento.pdf"
```

Password grant exists only on `kalke-cli` for local testing. Production websites must use Authorization Code + PKCE (`kalke-spa`). Machines use `pde-m2m` / `ebank-m2m` client credentials — not product-issued API keys.

## Make targets

```bash
make help
make up | down | destroy | logs | ps
make jwks
make token
make m2m-token
make ebank-m2m-token
```

## Cloudflare deploy

See [DEPLOY.md](DEPLOY.md). CI on `main` validates the realm JSON, builds the Keycloak image, and deploys the Worker + Container.

## What this is / is not

**Is:** IdP product boundary — reverse proxy + Keycloak + realm import for human and M2M OIDC.  
**Is not:** a custom Go auth BFF, a password store inside product APIs, or HA Keycloak.

## License

Apache-2.0
