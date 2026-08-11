#!/usr/bin/env bash
# Run on the EC2 host (or via GitHub Actions SSH) to update production.
# Expects: repo at ~/kalke-auth, prod.env already present, Docker installed.
set -euo pipefail

REPO_DIR="${REPO_DIR:-${HOME}/kalke-auth}"
BRANCH="${BRANCH:-main}"
AWS_REGION="${AWS_REGION:-us-east-1}"
SECRET_ID="${AUTH_SECRET_ID:-kalke/kalke-auth/prod}"

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

# Merge PDE/EBANK M2M keys into Secrets Manager (and keep slim pointers in prod.env).
echo "==> Syncing PDE_* and EBANK_* into Secrets Manager (${SECRET_ID})"
umask 077
export AWS_REGION SECRET_ID
python3 - <<'PY'
import json, os, subprocess
from pathlib import Path

path = Path("prod.env")
region = os.environ.get("AWS_REGION") or "us-east-1"
secret_id = os.environ["SECRET_ID"]

def q(v: str) -> str:
    return "'" + v.replace("'", "'\"'\"'") + "'"

updates: dict[str, str] = {}
for key in (
    "PDE_BASE_URL",
    "PDE_M2M_CLIENT_ID",
    "PDE_M2M_CLIENT_SECRET",
    "PDE_USER_FORWARD_SECRET",
    "EBANK_BASE_URL",
    "EBANK_M2M_CLIENT_ID",
    "EBANK_M2M_CLIENT_SECRET",
    "EBANK_USER_FORWARD_SECRET",
):
    val = os.environ.get(key, "").strip()
    if val:
        updates[key] = val

defaults = {
    "PDE_BASE_URL": "https://pde.kalke.dev",
    "PDE_M2M_CLIENT_ID": "pde-m2m",
    "EBANK_BASE_URL": "https://ebank.kalke.dev",
    "EBANK_M2M_CLIENT_ID": "ebank-m2m",
}

# Seed defaults from existing file when SM blob is still empty of these keys.
file_vals: dict[str, str] = {}
for line in path.read_text().splitlines():
    s = line.strip()
    if not s or s.startswith("#") or "=" not in s:
        continue
    k, v = s.split("=", 1)
    file_vals[k.strip()] = v.strip().strip("'").strip('"')

for key, default in defaults.items():
    if key not in updates and key not in file_vals:
        updates[key] = default

raw_get = subprocess.run(
    [
        "aws", "secretsmanager", "get-secret-value",
        "--region", region, "--secret-id", secret_id,
        "--query", "SecretString", "--output", "text",
    ],
    capture_output=True, text=True,
)
data: dict = {}
if raw_get.returncode == 0 and raw_get.stdout.strip():
    try:
        data = json.loads(raw_get.stdout)
    except json.JSONDecodeError:
        data = {}
if not isinstance(data, dict):
    data = {}

# Prefer existing SM values; fill gaps from local prod.env (one-time migration).
skip = {"AWS_REGION", "SECRET_ID", "KALKE_SECRETS_LOADED", "PLACEHOLDER"}
for k, v in file_vals.items():
    if k in skip:
        continue
    cur = data.get(k)
    if cur in (None, "", "replace-me") or k not in data:
        data[k] = v
data.pop("PLACEHOLDER", None)

for k, v in updates.items():
    data[k] = v
data["AWS_REGION"] = region
data["LOG_FORMAT"] = data.get("LOG_FORMAT") or "json"

if not data.get("DATABASE_URL") and not data.get("KC_DB_URL"):
    raise SystemExit(
        "refusing to publish empty auth secret; populate Secrets Manager or keep a fat prod.env once"
    )

raw = json.dumps(data)
put = subprocess.run(
    [
        "aws", "secretsmanager", "put-secret-value",
        "--region", region, "--secret-id", secret_id, "--secret-string", raw,
    ],
    capture_output=True, text=True,
)
if put.returncode != 0:
    create = subprocess.run(
        [
            "aws", "secretsmanager", "create-secret",
            "--region", region, "--name", secret_id, "--secret-string", raw,
        ],
        capture_output=True, text=True,
    )
    if create.returncode != 0:
        raise SystemExit(
            f"secretsmanager put/create failed: {put.stderr or put.stdout} / "
            f"{create.stderr or create.stdout}"
        )
    print(f"created secret {secret_id}")
else:
    print(f"updated secret {secret_id}")

# Slim bootstrap pointers only (entrypoint loadsecret fills the rest).
path.write_text(
    f"AWS_REGION={q(region)}\n"
    f"SECRET_ID={q(secret_id)}\n"
)
print(f"wrote slim {path}; merged keys:", ", ".join(sorted(updates)) or "(none)")
PY

echo "==> Freeing Docker disk (t3.micro root is tight)"
docker builder prune -af >/dev/null || true
docker image prune -af >/dev/null || true

echo "==> Building and restarting stack"
make aws-up

echo "==> Status"
make aws-ps
