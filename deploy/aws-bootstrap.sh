#!/usr/bin/env bash
# Bootstrap an AWS Free Tier Ubuntu EC2 for kalke-auth.
# Run as ubuntu (or any sudo user) on the instance:
#
#   bash deploy/aws-bootstrap.sh
#
set -euo pipefail

if [[ "$(id -u)" -eq 0 ]]; then
  SUDO=""
else
  SUDO="sudo"
fi

echo "==> 2G swap (required on t3.micro / 1 GB RAM)"
if ! $SUDO swapon --show | grep -q .; then
  if [[ ! -f /swapfile ]]; then
    $SUDO fallocate -l 2G /swapfile || $SUDO dd if=/dev/zero of=/swapfile bs=1M count=2048
    $SUDO chmod 600 /swapfile
    $SUDO mkswap /swapfile
  fi
  $SUDO swapon /swapfile || true
  if ! grep -q '/swapfile' /etc/fstab 2>/dev/null; then
    echo '/swapfile none swap sw 0 0' | $SUDO tee -a /etc/fstab >/dev/null
  fi
else
  echo "swap already active"
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

echo "==> Firewall: SSH + HTTP/HTTPS"
$SUDO ufw allow OpenSSH
$SUDO ufw allow 80/tcp
$SUDO ufw allow 443/tcp
$SUDO ufw --force enable || true

echo "==> Done. Log out/in (docker group), then:"
echo "  cd ~/kalke-auth   # or: git clone … && cd kalke-auth"
echo "  git checkout cursor/aws-auth-f44b   # until merged"
echo "  cp prod.env.example prod.env && nano prod.env"
echo "  make aws-up"
echo ""
echo "DNS: auth.kalke.dev A → this instance Elastic IP (Cloudflare DNS-only until TLS works)."
