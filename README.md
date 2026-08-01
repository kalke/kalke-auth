# kalke-auth

OIDC identity provider for Kalke apps, plus a small **Go API** that sits in front of Keycloak.

- Browser / playground talk to the Go API (`/v1/*`).
- Keycloak stays internal; only discovery + JWKS are proxied publicly.
- Apps validate JWTs against `https://auth.kalke.dev/realms/kalke`.

## Local

```bash
cp .env.example .env
make up
make jwks
make token
```

Local Compose still uses Keycloak + Caddy for development. Production runs Go + Keycloak in one Cloudflare container — see [DEPLOY.md](DEPLOY.md).

## Production

Push to `main` deploys via GitHub Actions. Secret **names** are listed in `DEPLOY.md`; values stay in GitHub Actions / Cloudflare only.
