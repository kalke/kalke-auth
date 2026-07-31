# kalke-auth

Encapsulated **OIDC Identity Provider** for Kalke apps.

Apps (including [personal-document-extractor](../personal-document-extractor)) talk only to a stable OIDC issuer. They do **not** configure Keycloak-specific URLs, admin APIs, or themes.

Under the hood (Docker network `kalke-auth`):

1. **kc-db** — Keycloak database (service name is *not* `postgres`, to avoid DNS clashes with other stacks)  
2. **Keycloak** — IdP implementation (not published on the host)  
3. **Caddy** — public reverse proxy (issuer façade)

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
make m2m-token  # machine client_credentials token
```

| Value | Default |
|---|---|
| Issuer | `http://localhost:8443/realms/kalke` |
| JWKS | `{issuer}/protocol/openid-connect/certs` |
| Audience (API) | `personal-document-extractor` |
| Demo user | `demo@kalke.local` / `DemoPass123!` |
| M2M client | `pde-m2m` / `pde-m2m-dev-secret` |
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
OIDC_AUDIENCE=personal-document-extractor
```

**App container on network `kalke-auth`:**

```bash
OIDC_ISSUER=http://localhost:8443/realms/kalke          # must match JWT iss
OIDC_DISCOVERY_URL=http://caddy:8443/realms/kalke       # reachable JWKS/discovery
OIDC_AUDIENCE=personal-document-extractor
```

The app validates JWTs locally with JWKS. Env vars for this IdP DB are namespaced (`KC_DB_USER`, …) so they do not collide with product `POSTGRES_*` variables.

## Clients in realm `kalke`

| Client | Type | Purpose |
|---|---|---|
| `personal-document-extractor` | bearer-only | API **audience** |
| `kalke-spa` | public + PKCE | Future website |
| `kalke-cli` | confidential + password grant | **Dev human smoke only** |
| `pde-m2m` | confidential + service account | **M2M** (`client_credentials`) |

Realm roles (mapped into access-token claim `permissions`):

- `extract:write`
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

Password grant exists only on `kalke-cli` for local testing. Production websites must use Authorization Code + PKCE (`kalke-spa`). Machines use `pde-m2m` client credentials — not product-issued API keys.

## Make targets

```bash
make help
make up | down | destroy | logs | ps
make jwks
make token
make m2m-token
```

## What this is / is not

**Is:** IdP product boundary — reverse proxy + Keycloak + realm import for human and M2M OIDC.  
**Is not:** a custom Go auth BFF, a password store inside product APIs, or HA Keycloak.

## License

Apache-2.0
