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

Local Compose uses Keycloak + Caddy for development.

## Production (Oracle Always Free)

Keycloak + Go BFF run on an Oracle VM (Docker Compose + Caddy TLS).  
See **[DEPLOY.md](DEPLOY.md)** for the full checklist.

```bash
# on the VM
bash deploy/oracle-bootstrap.sh
cp prod.env.example prod.env   # fill secrets
make oracle-up
```
