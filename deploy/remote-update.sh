#!/usr/bin/env bash
# Run on the EC2 host (or via GitHub Actions SSH) to update production.
# Expects: repo at ~/kalke-auth, prod.env already present, Docker installed.
set -euo pipefail

REPO_DIR="${REPO_DIR:-${HOME}/kalke-auth}"
BRANCH="${BRANCH:-main}"

if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "GH_TOKEN is required (GitHub Actions passes this for private clone/pull)" >&2
  exit 1
fi

if [[ ! -d "${REPO_DIR}/.git" ]]; then
  echo "==> Cloning kalke-auth into ${REPO_DIR}"
  git clone "https://x-access-token:${GH_TOKEN}@github.com/kalke/kalke-auth.git" "${REPO_DIR}"
fi

cd "${REPO_DIR}"

if [[ ! -f prod.env ]]; then
  echo "prod.env missing in ${REPO_DIR}. Create it once on the VM before CI deploy." >&2
  exit 1
fi

echo "==> Updating to origin/${BRANCH}"
git remote set-url origin "https://x-access-token:${GH_TOKEN}@github.com/kalke/kalke-auth.git"
git fetch --depth=1 origin "${BRANCH}"
# Discard local tracked edits on the host (prod.env is gitignored and kept).
git checkout -f -B "${BRANCH}" "FETCH_HEAD"
git clean -fd
# Drop token from remote URL so it is not stored on disk.
git remote set-url origin "https://github.com/kalke/kalke-auth.git"

if [[ -n "${PDE_USER_FORWARD_SECRET:-}" ]]; then
  echo "==> Syncing PDE_USER_FORWARD_SECRET into prod.env"
  umask 077
  PDE_USER_FORWARD_SECRET="${PDE_USER_FORWARD_SECRET}" python3 - <<'PY'
import os
from pathlib import Path

path = Path("prod.env")
secret = os.environ["PDE_USER_FORWARD_SECRET"].strip()
if not secret:
    raise SystemExit("PDE_USER_FORWARD_SECRET is empty")

def q(v: str) -> str:
    return "'" + v.replace("'", "'\"'\"'") + "'"

key = "PDE_USER_FORWARD_SECRET"
lines = path.read_text().splitlines(keepends=True)
out = []
found = False
for line in lines:
    if line.startswith(f"{key}=") or line.startswith(f"{key} ="):
        out.append(f"{key}={q(secret)}\n")
        found = True
    else:
        out.append(line)
if not found:
    if out and not str(out[-1]).endswith("\n"):
        out[-1] = str(out[-1]) + "\n"
    out.append(f"{key}={q(secret)}\n")
path.write_text("".join(out))
print(f"updated {path}")
PY
fi

echo "==> Freeing Docker disk (t3.micro root is tight)"
docker builder prune -af >/dev/null || true
docker image prune -af >/dev/null || true

echo "==> Building and restarting stack"
make aws-up

echo "==> Status"
make aws-ps
