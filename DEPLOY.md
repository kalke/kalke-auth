# Deploy

Production auth (**Keycloak + Go BFF**) runs on **AWS Free Tier EC2**.
No Cloudflare Containers → no Workers Paid.

> Free Tier is typically **~12 months** of `t2.micro` / `t3.micro` (750 h/mês).
> Depois disso a AWS cobra — para ficar $0 para sempre precisarias de outro always-free.

## AWS Free Tier EC2

### 1. Create the instance

EC2 → Launch instance:

| Field | Value |
|---|---|
| Name | `kalke-auth` |
| AMI | **Ubuntu Server 24.04 LTS** (Free tier eligible) |
| Type | **t3.micro** or **t2.micro** (1 GB RAM) |
| Key pair | create/download `.pem` |
| Network | public IP / Elastic IP later |
| Security group | inbound **22** (your IP), **80**, **443** from `0.0.0.0/0` |
| Storage | 8–30 GB gp3 (Free Tier allowance) |

Allocate an **Elastic IP** and associate it (IP estável para o DNS).

### 2. Bootstrap

```bash
ssh -i your.pem ubuntu@EIP
git clone https://github.com/kalke/kalke-auth.git
cd kalke-auth
git checkout cursor/aws-auth-f44b   # until this PR is merged
bash deploy/aws-bootstrap.sh
# disconnect + reconnect (docker group)
```

O script cria **2 GB de swap** (obrigatório no micro) + Docker + UFW.

### 3. Secrets

```bash
cp prod.env.example prod.env
nano prod.env
```

Usa os valores de `Documents/kalke/secrets/prod.env.generated` + Neon/Redis/Mailgun:

- `KC_DB_*` → Neon **direct** + `currentSchema=keycloak`
- `DATABASE_URL` → Neon **pooler**
- Redis, session/pepper/introspect, `KC_BFF_CLIENT_SECRET`, Mailgun

### 4. Start

```bash
make aws-up
make aws-logs   # first boot can take several minutes on t3.micro
```

### 5. DNS

Cloudflare → `auth.kalke.dev` **A** → Elastic IP → **DNS only** (grey) until Caddy gets a cert.

```bash
curl -fsS https://auth.kalke.dev/realms/kalke/.well-known/openid-configuration | head
curl -fsS -o /dev/null -w '%{http_code}\n' https://auth.kalke.dev/v1/health
```

### 6. Updates

```bash
cd ~/kalke-auth && git pull && make aws-up
```

## Stay on free tier

- Keep **one** always-on micro (750 h/mês = 1 instância 24/7)
- DB = Neon free, Redis = Upstash free, mail = Mailgun free tier
- `kalke.dev` Worker fica no Cloudflare **Free** (sem Containers)
- Monitora o billing alarm da AWS (Billing → Budgets → $1 alert)

## Optional Cloudflare Worker proxy

Default CI **does not** deploy to Cloudflare. Only if you set `DEPLOY_CF_WORKER=true` + `ORIGIN_URL`.

## Signup / lockdown

Email OTP signup; admin never via site signup (`ADMIN_EMAILS`). Privileged perms stripped unless allowlisted.
