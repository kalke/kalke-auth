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

```bash
# one-time on the instance
bash deploy/aws-bootstrap.sh
cp prod.env.example prod.env   # Neon, Redis, Mailgun, KC, PDE_*, EBANK_*
make aws-up
```

| Host | Upstream |
|------|----------|
| `auth.kalke.dev` | `auth:8080` |
| `pde.kalke.dev` | `pde-api:8080` |
| `ebank.kalke.dev` | `ebank-api:8000` |

DNS: grey-cloud A records → EIP. Keycloak admin via Tunnel + Access (`keycloak.kalke.dev`).

With PDE + e-bank on the same box, prefer **t3.small (2 GB)** + 2G swap.
