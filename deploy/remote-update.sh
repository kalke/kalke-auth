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

# Upsert PDE proxy settings from CI secrets (required for cookie → PDE extract).
echo "==> Syncing PDE_* into prod.env"
umask 077
python3 - <<'PY'
import os
from pathlib import Path

path = Path("prod.env")

def q(v: str) -> str:
    return "'" + v.replace("'", "'\"'\"'") + "'"

# Non-empty values from the environment win; missing keys are left untouched
# so a partial secret set cannot wipe a working prod.env.
updates: dict[str, str] = {}
for key in (
    "PDE_BASE_URL",
    "PDE_M2M_CLIENT_ID",
    "PDE_M2M_CLIENT_SECRET",
    "PDE_USER_FORWARD_SECRET",
):
    val = os.environ.get(key, "").strip()
    if val:
        updates[key] = val

# Defaults when enabling the proxy for the first time.
if "PDE_BASE_URL" not in updates and not any(
    line.startswith("PDE_BASE_URL=") for line in path.read_text().splitlines()
):
    updates["PDE_BASE_URL"] = "https://pde.kalke.dev"
if "PDE_M2M_CLIENT_ID" not in updates and not any(
    line.startswith("PDE_M2M_CLIENT_ID=") for line in path.read_text().splitlines()
):
    updates["PDE_M2M_CLIENT_ID"] = "pde-m2m"

if not updates:
    print("no PDE_* secrets provided; leaving prod.env unchanged")
else:
    lines = path.read_text().splitlines(keepends=True)
    out = []
    seen: set[str] = set()
    for line in lines:
        s = line.strip()
        if s and not s.startswith("#") and "=" in s:
            k = s.split("=", 1)[0].strip()
            if k in updates:
                out.append(f"{k}={q(updates[k])}\n")
                seen.add(k)
                continue
        out.append(line)
    if out and not str(out[-1]).endswith("\n"):
        out[-1] = str(out[-1]) + "\n"
    for k, v in updates.items():
        if k not in seen:
            out.append(f"{k}={q(v)}\n")
    path.write_text("".join(out))
    print("updated:", ", ".join(sorted(updates)))
PY

echo "==> Freeing Docker disk (t3.micro root is tight)"
docker builder prune -af >/dev/null || true
docker image prune -af >/dev/null || true

echo "==> Building and restarting stack"
make aws-up

echo "==> Status"
make aws-ps
