# Deploy kalke-auth (auth.kalke.dev)

Cloudflare Containers + Neon Postgres (free). Local Docker Compose is unchanged.

## 1. Neon (free, no card)

1. Create a project at [neon.tech](https://neon.tech).
2. Create database `kalke_auth` (or use the default and a dedicated role).
3. Copy the **pooled** connection string and convert to JDBC for Keycloak:

```text
# Neon URI (example)
postgresql://user:pass@ep-xxx.region.aws.neon.tech/kalke_auth?sslmode=require

# Keycloak KC_DB_URL
jdbc:postgresql://ep-xxx.region.aws.neon.tech/kalke_auth?sslmode=require
```

Use the same user/password for `KC_DB_USERNAME` / `KC_DB_PASSWORD`.

## 2. Cloudflare

1. Enable **Workers Paid** (Containers require it, ~$5/mo).
2. Ensure zone `kalke.dev` is on the account (custom domain `auth.kalke.dev`).
3. Create an API token with Workers + Containers edit permissions.
4. Note Account ID.

## 3. GitHub secrets (`kalke/kalke-auth`)

| Secret | Value |
|---|---|
| `CLOUDFLARE_API_TOKEN` | Cloudflare API token |
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare account id |
| `KC_DB_URL` | `jdbc:postgresql://…/kalke_auth?sslmode=require` |
| `KC_DB_USERNAME` | Neon user |
| `KC_DB_PASSWORD` | Neon password |
| `KC_BOOTSTRAP_ADMIN_USERNAME` | Keycloak admin user (e.g. `admin`) |
| `KC_BOOTSTRAP_ADMIN_PASSWORD` | Strong admin password |

## 4. Deploy

Push to `main` (after PR merge). CI validates the realm, builds the image, then `wrangler deploy`.

Manual:

```bash
npm ci
npx wrangler secret put KC_DB_URL
npx wrangler secret put KC_DB_USERNAME
npx wrangler secret put KC_DB_PASSWORD
npx wrangler secret put KC_BOOTSTRAP_ADMIN_USERNAME
npx wrangler secret put KC_BOOTSTRAP_ADMIN_PASSWORD
npm run deploy
```

Issuer after deploy: `https://auth.kalke.dev/realms/kalke`

## 5. Branch protection (owner)

See [kalke BRANCH_PROTECTION.md](https://github.com/kalke/kalke/blob/main/BRANCH_PROTECTION.md). Required checks: `Validate realm`, `Docker build`. Restrict push to `kalke` only.

## Demo sandbox user

Imported from realm (change password after first shared use):

- `demo@kalke.local` / `DemoPass123!`
- Roles: `extract:write`, `bank:write`
