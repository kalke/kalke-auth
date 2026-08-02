# Deploy

Production auth (**Keycloak + Go BFF**) runs on an **Oracle Cloud Always Free** VM.
Cloudflare Containers are no longer required (avoids Workers Paid).

## Oracle Always Free (recommended)

### 1. Create the VM

In Oracle Cloud Console:

1. **Compute → Instances → Create**
2. Image: **Ubuntu 22.04/24.04**
3. Shape: **VM.Standard.A1.Flex** (Ampere ARM) — Always Free eligible  
   Suggested: **2 OCPU / 12 GB RAM** (Keycloak is happier with ≥4 GB)
4. Networking: assign a **public IPv4**
5. **VCN security list / NSG**: ingress **22**, **80**, **443** from `0.0.0.0/0` (or lock SSH to your IP)

### 2. Bootstrap the VM

SSH in, then:

```bash
git clone https://github.com/kalke/kalke-auth.git
cd kalke-auth
bash deploy/oracle-bootstrap.sh
# log out/in once so your user is in the docker group
```

### 3. Secrets on the VM

```bash
cp prod.env.example prod.env
nano prod.env   # fill from your private secrets notes
```

Critical:

- `KC_DB_*` → Neon **direct** host + `currentSchema=keycloak`
- `DATABASE_URL` → Neon **pooler** (app schema via `DB_SEARCH_PATH=app`)
- Redis, session/pepper/introspect, `KC_BFF_CLIENT_SECRET`, Mailgun
- `COOKIE_DOMAIN=.kalke.dev` (set in compose; keep consistent)

### 4. Start

```bash
docker compose -f docker-compose.oracle.yml --env-file prod.env up -d --build
docker compose -f docker-compose.oracle.yml logs -f
```

### 5. DNS

Cloudflare DNS for `auth.kalke.dev`:

- Type **A** → VM public IP  
- Proxy status: **DNS only** (grey cloud) until Caddy issues the cert  
- After HTTPS works you may enable the orange cloud if you want

Smoke:

```bash
curl -fsS https://auth.kalke.dev/realms/kalke/.well-known/openid-configuration | head
curl -fsS https://auth.kalke.dev/v1/health
```

### 6. Updates

```bash
cd ~/kalke-auth
git pull
docker compose -f docker-compose.oracle.yml --env-file prod.env up -d --build
```

## Optional Cloudflare Worker proxy

Only if you want `auth.kalke.dev` on a Worker in front of the VM:

1. Set GitHub secret `ORIGIN_URL` (e.g. `https://auth.kalke.dev` on the VM IP via hosts, or the VM URL)
2. Set repo variable `DEPLOY_CF_WORKER=true`
3. Keep `CLOUDFLARE_API_TOKEN` / `CLOUDFLARE_ACCOUNT_ID`

Default CI **does not** deploy to Cloudflare.

## Signup / lockdown (unchanged)

1. `POST /v1/auth/signup` — `{ name, email, password }` → email OTP  
2. `POST /v1/auth/signup/verify` — creates Keycloak user (**no realm roles**) + session  
3. `POST /v1/auth/signup/resend` — after 2 minutes  

Admin email allowlist: `ADMIN_EMAILS`. Privileged perms (`admin`, `bank:write`) are stripped unless allowlisted.
