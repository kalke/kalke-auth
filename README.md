# kalke-auth

OIDC identity provider for Kalke apps, plus a small **Go API** that sits in front of Keycloak.

- Browser / playground talk to the Go API (`/v1/*`).
- Keycloak stays internal; only discovery + JWKS are proxied publicly.
- Apps validate JWTs against `https://auth.kalke.dev/realms/kalke`.

### Auth API (cookie session)

| Method | Path | Notes |
|--------|------|--------|
| POST | `/v1/auth/login` | email + password → session cookie |
| POST | `/v1/auth/signup` (+ `/verify`, `/resend`) | OTP signup |
| GET | `/v1/auth/me` | current session |
| POST | `/v1/auth/password` | `{current_password,new_password}` (session; ≥10 chars, letter + digit) |
| POST | `/v1/auth/logout` | clear session |

## Local

```bash
cp .env.example .env
cp local.env.example local.env   # or: make setup
make up
make jwks
make token
```

Local stack:

| Service | URL |
|---------|-----|
| Auth BFF (`/v1/*`) | http://localhost:8090 |
| OIDC issuer (Caddy → Keycloak) | http://localhost:8443/realms/kalke |
| Demo user | `demo@kalke.local` / `DemoPass123!` |

Cookies are host-only + `SameSite=Lax` + non-Secure so the Vite app on `:5173` can use session auth over HTTP.

If you change the realm JSON after the first boot, wipe volumes: `make destroy && make up`.

## Production (AWS Free Tier)

Keycloak + Go BFF on a free-tier EC2 (`t3.micro`) + Caddy TLS.  
Push to `main` deploys over SSH (see **[DEPLOY.md](DEPLOY.md)**).

```bash
# one-time on the EC2 instance
bash deploy/aws-bootstrap.sh
cp prod.env.example prod.env   # fill secrets
make aws-up
```
