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

## Production (AWS Free Tier)

Keycloak + Go BFF on a free-tier EC2 (`t3.micro`) + Caddy TLS.  
See **[DEPLOY.md](DEPLOY.md)**.

```bash
# on the EC2 instance
bash deploy/aws-bootstrap.sh
cp prod.env.example prod.env   # fill secrets
make aws-up
```
