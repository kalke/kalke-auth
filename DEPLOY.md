# Deploy

Production deploy runs automatically on push to `main` (GitHub Actions → Cloudflare Containers).

Required GitHub Actions secret **names** (values are private; never commit them):

- `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`
- `KC_DB_URL`, `KC_DB_USERNAME`, `KC_DB_PASSWORD`
- `KC_BOOTSTRAP_ADMIN_USERNAME`, `KC_BOOTSTRAP_ADMIN_PASSWORD`
- `DATABASE_URL`, `REDIS_ADDR`, `REDIS_PASSWORD`
- `SESSION_SECRET`, `TOKEN_HASH_PEPPER`, `INTROSPECT_SECRET`
- `KC_BFF_CLIENT_SECRET`

Operational details for schemas, playground users, and secret generation live in private notes — not in this public repo.
