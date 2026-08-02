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

`prod.env` **nunca** vai para o git; o CI não o substitui.

### 3. DNS

`auth.kalke.dev` **A** → Elastic IP → Cloudflare **DNS only** until TLS works.

## CI/CD (automático)

Depois do merge em `main`, cada push corre: Validate realm → Go test → Docker build → **Deploy to AWS EC2**.

### GitHub Actions secrets (kalke-auth)

No teu PC (onde está o `.pem`):

```bash
# IP público / Elastic IP da instance
gh secret set AWS_EC2_HOST -R kalke/kalke-auth -b '54.82.62.18'

gh secret set AWS_EC2_USER -R kalke/kalke-auth -b 'ubuntu'

# Conteúdo completo do .pem (incluindo -----BEGIN/END-----)
gh secret set AWS_EC2_SSH_KEY -R kalke/kalke-auth < first.pem
```

Confere:

```bash
gh secret list -R kalke/kalke-auth
# deve listar AWS_EC2_HOST, AWS_EC2_USER, AWS_EC2_SSH_KEY
```

### O que o deploy faz

1. SSH na EC2 com a key do secret  
2. `git fetch` de `main` (repo privado via `GITHUB_TOKEN`)  
3. `make aws-up` (rebuild + restart; **mantém** `prod.env`)

### Security group

Se o SSH da instance estiver só no “My IP”, o Actions **falha** (IP do runner muda).  
Abre **TCP 22** para `0.0.0.0/0` (ou ao menos o range da AWS/GitHub — o mais simples é `0.0.0.0/0` + só key).

## Updates manuais (opcional)

```bash
ssh -i first.pem ubuntu@EIP
cd ~/kalke-auth && bash deploy/remote-update.sh   # precisa GH_TOKEN
# ou: git pull && make aws-up
```

## Stay on free tier

- Uma micro 24/7 · Neon · Upstash · Mailgun free · Workers Free (sem Containers)
- Budget alert $1 na AWS

## Optional Cloudflare Worker proxy

Só se `DEPLOY_CF_WORKER=true` + `ORIGIN_URL`. Por defeito **não** corre.

## Signup / lockdown

Email OTP; admin nunca via signup (`ADMIN_EMAILS`).
