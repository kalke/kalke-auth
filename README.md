# kalke-auth

OIDC IdP (Keycloak) + Go BFF for Kalke apps.

- Browsers use cookie sessions on `/v1/*` (`auth.kalke.dev`)
- APIs validate JWTs from `https://auth.kalke.dev/realms/kalke`
- BFF proxies `/v1/extract*` → PDE and `/v1/bank/*` → e-bank (M2M + user-forward)

## Local

```bash
cp .env.example .env && cp local.env.example local.env
make up && make jwks && make token
```

| Service | URL |
|---------|-----|
| BFF | http://localhost:8090 |
| OIDC (Caddy → Keycloak) | http://localhost:8443/realms/kalke |
| Demo | `demo@kalke.local` / `DemoPass123!` |

Realm JSON changes: `make destroy && make up`.

## Production (EC2)

Push `main` → self-hosted runner `kalke-auth-ec2` → `make aws-up`.

Postgres runs in Docker on the same instance (no public `:5432`). Prefer **t3.small (2 GB)** + 2G swap when PDE + e-bank share the box — a 1 GB `t3.micro` will OOM with Keycloak + Postgres.

```bash
# one-time on the instance
bash deploy/aws-bootstrap.sh
cp prod.env.example prod.env   # POSTGRES_PASSWORD, Redis, Mailgun, KC, PDE_*, EBANK_*
# URL-safe password (hex). aws-up generates one if missing.
# POSTGRES_PASSWORD=$(openssl rand -hex 16)
make aws-up                    # starts postgres, dumps Neon if the volume is empty, then auth
```

Cutover from Neon: `make aws-up` runs `deploy/migrate-from-neon.sh --if-empty` (uses `NEON_DATABASE_URL` / `DATABASE_URL` in Secrets Manager while those still point at Neon). To re-run: `make aws-migrate-from-neon` or the **Migrate Neon to EC2 Postgres** workflow (`workflow_dispatch`). Leave Neon up until login on `https://auth.kalke.dev` looks good, then delete the Neon project.

Optional later: cron `pg_dump` of the `postgres` container to S3. The Docker volume `pgdata` is the live data.

| Host | Upstream |
|------|----------|
| `auth.kalke.dev` | `auth:8080` |
| `pde.kalke.dev` | `pde-api:8080` |
| `ebank.kalke.dev` | `ebank-api:8000` |

DNS: grey-cloud A records → EIP. Keycloak admin via Tunnel + Access (`keycloak.kalke.dev`).
