#!/usr/bin/env bash
# Bootstrap an Oracle Always Free Ubuntu VM for kalke-auth.
# Run as a sudo-capable user on the VM:
#
#   curl -fsSL … | bash   # or: bash deploy/oracle-bootstrap.sh
#
set -euo pipefail

if [[ "$(id -u)" -eq 0 ]]; then
  SUDO=""
else
  SUDO="sudo"
fi

echo "==> Installing Docker Engine + Compose plugin"
$SUDO apt-get update -y
$SUDO apt-get install -y ca-certificates curl git ufw
$SUDO install -m 0755 -d /etc/apt/keyrings
if [[ ! -f /etc/apt/keyrings/docker.asc ]]; then
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | $SUDO tee /etc/apt/keyrings/docker.asc >/dev/null
  $SUDO chmod a+r /etc/apt/keyrings/docker.asc
fi
. /etc/os-release
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
  | $SUDO tee /etc/apt/sources.list.d/docker.list >/dev/null
$SUDO apt-get update -y
$SUDO apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
$SUDO usermod -aG docker "${SUDO_USER:-$USER}" || true

echo "==> Firewall: allow SSH + HTTP/HTTPS"
$SUDO ufw allow OpenSSH
$SUDO ufw allow 80/tcp
$SUDO ufw allow 443/tcp
$SUDO ufw --force enable || true

echo "==> Done. Log out/in (for docker group), then:"
echo "  git clone https://github.com/kalke/kalke-auth.git && cd kalke-auth"
echo "  cp prod.env.example prod.env   # fill secrets"
echo "  docker compose -f docker-compose.oracle.yml --env-file prod.env up -d --build"
echo ""
echo "Point auth.kalke.dev A/AAAA to this VM public IP (Cloudflare DNS-only / grey cloud"
echo "until TLS works; then you may orange-cloud if you want)."
