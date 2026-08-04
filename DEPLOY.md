# Deploy

Production auth (**Keycloak + Go BFF**) runs on **AWS Free Tier EC2**.
Push to `main` → GitHub Actions SSH → `make aws-up` on the instance.

> Free Tier is typically **~12 months** of `t2.micro` / `t3.micro` (750 h/mês).

## One-time: EC2 + secrets on the VM

### 1. Create the instance

| Field | Value |
|---|---|
| Name | `kalke-auth` |
| AMI | **Ubuntu Server 24.04 LTS** (Free tier eligible) |
| Type | **t3.micro** or **t2.micro** |
| Key pair | `.pem` (keep private) |
| Security group | **22** from `0.0.0.0/0` (CI needs this; key-only auth), **80/443** from internet |
| Storage | 8–30 GB gp3 |

Associate an **Elastic IP**.

> SSH aberto na internet + só chave `.pem` é o padrão simples para Actions.
> Não uses password login.

### 2. Bootstrap + `prod.env` (só uma vez)

```bash
ssh -i first.pem ubuntu@EIP
git clone https://github.com/kalke/kalke-auth.git
cd kalke-auth
bash deploy/aws-bootstrap.sh
# reconnect for docker group
cp prod.env.example prod.env && nano prod.env   # Neon/Redis/Mailgun/session secrets
make aws-up   # first manual boot
```

`prod.env` **nunca** vai para o git. No deploy, o CI sincroniza `PDE_BASE_URL`,
`PDE_M2M_CLIENT_ID`, `PDE_M2M_CLIENT_SECRET` e `PDE_USER_FORWARD_SECRET` a partir
dos GitHub secrets homônimos e mantém o restante do arquivo.

### 3. DNS

`auth.kalke.dev` **A** → Elastic IP → Cloudflare **DNS only** until TLS works.

## CI/CD (automático)

Push em `main`: **Lint → Test → Security → Build → Deploy**.

Deploy corre num **self-hosted runner** na própria EC2 (`kalke-auth-ec2`). Runners do GitHub costumam tomar timeout em SSH:22.

### O que o deploy faz

1. Runner local em `~/kalke-auth`
2. `git fetch` de `main` (via `GITHUB_TOKEN`)
3. `make aws-up` (rebuild + restart; **mantém** `prod.env`, sync `PDE_*`)

## Updates manuais (opcional)

```bash
ssh -i first.pem ubuntu@EIP
cd ~/kalke-auth && bash deploy/remote-update.sh   # precisa GH_TOKEN
# ou: git pull && make aws-up
```

## Stay on free tier

- Uma micro 24/7 · Neon · Upstash · Mailgun free · Workers Free (sem Containers)
- Budget alert $1 na AWS

## Keycloak admin — `keycloak.kalke.dev` (Cloudflare Tunnel + Access)

O admin **não** passa pelo Caddy público. Sobe atrás do **Cloudflare Zero Trust** (Tunnel + Access).

### A) Criar o Tunnel (dashboard)

1. [One Dashboard](https://one.dash.cloudflare.com/) → **Networks** → **Tunnels** → **Create tunnel**
2. Tipo **Cloudflared** → nome `kalke-keycloak`
3. Copia o **token** (`eyJ...`)
4. **Public Hostname**:
   - Subdomain: `keycloak`
   - Domain: `kalke.dev`
   - Type: **HTTP**
   - URL: `auth:8081`  
     (hostname do serviço Docker na mesma compose network)
5. Save

Isso cria o DNS `keycloak.kalke.dev` automaticamente (CNAME pro tunnel).

### B) Access (a “VPN” / login)

1. Zero Trust → **Access** → **Applications** → **Add application** → **Self-hosted**
2. Application domain: `keycloak.kalke.dev` (path opcional: `/`)
3. Policy: **Allow**
   - Include → **Emails** → `henriquekalke@icloud.com`
4. Save

Sem esse login da Cloudflare, ninguém chega no Keycloak.

### C) Token na EC2

```bash
ssh -i first.pem ubuntu@EIP
cd ~/kalke-auth
nano prod.env
# adiciona:
# CLOUDFLARE_TUNNEL_TOKEN='eyJ...'
# KC_HOSTNAME='https://keycloak.kalke.dev'
# KC_HOSTNAME_ADMIN='https://keycloak.kalke.dev'
# KC_REALM_FRONTEND_URL='https://auth.kalke.dev'

git pull origin main
make aws-up
docker compose -f docker-compose.aws.yml --env-file prod.env --profile tunnel ps
```

**Importante:** `KC_HOSTNAME` e `KC_HOSTNAME_ADMIN` devem ser **iguais** (`https://keycloak.kalke.dev`).  
Se `KC_HOSTNAME` apontar para `auth.kalke.dev`, o admin SPA usa `auth-server-url=https://auth.kalke.dev` onde o BFF **não** é o Keycloak → *"Something went wrong"*.  
O issuer público do realm `kalke` continua em `auth.kalke.dev` via `attributes.frontendUrl` / `KC_REALM_FRONTEND_URL`.

Abre: `https://keycloak.kalke.dev/admin/`  
→ login Cloudflare (teu e-mail) → login Keycloak (`admin` / senha do `prod.env`).

## Optional Cloudflare Worker proxy

Só se `DEPLOY_CF_WORKER=true` + `ORIGIN_URL`. Por defeito **não** corre.

## Signup / lockdown

Email OTP; admin nunca via signup (`ADMIN_EMAILS`).
