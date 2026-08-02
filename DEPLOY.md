# Deploy

Production deploy runs automatically on push to `main` (GitHub Actions → Cloudflare Containers).

Required GitHub Actions secret **names** (values are private; never commit them):

- `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`
- `KC_DB_URL`, `KC_DB_USERNAME`, `KC_DB_PASSWORD`
- `KC_BOOTSTRAP_ADMIN_USERNAME`, `KC_BOOTSTRAP_ADMIN_PASSWORD`
- `DATABASE_URL`, `REDIS_ADDR`, `REDIS_PASSWORD`
- `SESSION_SECRET`, `TOKEN_HASH_PEPPER`, `INTROSPECT_SECRET`
- `KC_BFF_CLIENT_SECRET`
- `SIGNUP_INVITE_CODE` (required while signup is enabled)

Public OIDC surface is discovery + JWKS only. Login/signup are rate-limited fail-closed
(Redis errors deny). Set wrangler var `SIGNUP_ENABLED=false` to disable signup entirely.

Admin is email-allowlisted (`ADMIN_EMAILS`, default your owner email). Public signup creates
users with **no** realm roles and cannot register an allowlisted admin email. Privileged
permissions (`admin`, `bank:write`) are stripped from sessions/PATs unless the email matches.

Operational details for schemas, playground users, and secret generation live in private notes — not in this public repo.
