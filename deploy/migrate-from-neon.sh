#!/usr/bin/env bash
# Dump Neon (or any source URL) into the Docker Postgres on this EC2.
#
# Usage (on the instance, or via workflow_dispatch):
#   bash deploy/migrate-from-neon.sh              # refuse if local DB already has data
#   bash deploy/migrate-from-neon.sh --if-empty   # no-op when keycloak schema has tables
#   bash deploy/migrate-from-neon.sh --force      # overwrite local data
#   bash deploy/migrate-from-neon.sh --ensure-password
#
# Source URL resolution (first match):
#   NEON_DATABASE_URL env, then Secrets Manager NEON_DATABASE_URL / DATABASE_URL / KC_DB_URL
# Pooler hosts (*-pooler.*) are rewritten to the Neon direct host for pg_dump.
set -euo pipefail

IF_EMPTY=0
FORCE=0
ENSURE_ONLY=0
for arg in "$@"; do
  case "$arg" in
    --if-empty) IF_EMPTY=1 ;;
    --force) FORCE=1 ;;
    --ensure-password) ENSURE_ONLY=1 ;;
    -h|--help)
      sed -n '2,16p' "$0"
      exit 0
      ;;
    *)
      echo "unknown argument: $arg" >&2
      exit 2
      ;;
  esac
done

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ ! -f prod.env ]]; then
  echo "prod.env missing in ${ROOT}" >&2
  exit 1
fi

ensure_postgres_password() {
  if grep -qE '^POSTGRES_PASSWORD=' prod.env; then
    return 0
  fi
  local pw
  pw="$(openssl rand -hex 16)"
  printf "\nPOSTGRES_PASSWORD='%s'\n" "$pw" >> prod.env
  echo "generated POSTGRES_PASSWORD and appended to prod.env"
}

ensure_postgres_password
if [[ "$ENSURE_ONLY" == 1 ]]; then
  exit 0
fi

COMPOSE=(docker compose -f docker-compose.aws.yml --env-file prod.env)
PG_USER="$(awk -F= '/^POSTGRES_USER=/{sub(/^[^=]*=/,""); gsub(/^['\''"]+|['\''"]+$/,""); print; exit}' prod.env)"
PG_DB="$(awk -F= '/^POSTGRES_DB=/{sub(/^[^=]*=/,""); gsub(/^['\''"]+|['\''"]+$/,""); print; exit}' prod.env)"
PG_USER="${PG_USER:-kalke}"
PG_DB="${PG_DB:-kalke}"

echo "==> Starting local postgres"
"${COMPOSE[@]}" up -d postgres --wait

local_table_count() {
  "${COMPOSE[@]}" exec -T postgres \
    psql -U "$PG_USER" -d "$PG_DB" -Atc \
    "SELECT count(*) FROM information_schema.tables WHERE table_schema IN ('keycloak','app') AND table_type='BASE TABLE'"
}

count="$(local_table_count)"
if [[ "$count" -gt 0 && "$FORCE" != 1 ]]; then
  if [[ "$IF_EMPTY" == 1 ]]; then
    echo "local postgres already has ${count} tables in keycloak/app; skip migrate"
    exit 0
  fi
  echo "refusing to overwrite local postgres (${count} tables). Pass --force to replace." >&2
  exit 1
fi

REGION="${AWS_REGION:-us-east-1}"
SECRET_ID="${AUTH_SECRET_ID:-kalke/kalke-auth/prod}"
if [[ -z "${AWS_REGION:-}" ]]; then
  REGION="$(awk -F= '/^AWS_REGION=/{sub(/^[^=]*=/,""); gsub(/^['\''"]+|['\''"]+$/,""); print; exit}' prod.env || true)"
  REGION="${REGION:-us-east-1}"
fi
if grep -qE '^SECRET_ID=' prod.env; then
  SECRET_ID="$(awk -F= '/^SECRET_ID=/{sub(/^[^=]*=/,""); gsub(/^['\''"]+|['\''"]+$/,""); print; exit}' prod.env)"
fi

DUMP_DIR="$(mktemp -d /tmp/kalke-neon-migrate.XXXXXX)"
chmod 700 "$DUMP_DIR"
cleanup() { rm -rf "$DUMP_DIR"; }
trap cleanup EXIT

echo "==> Resolving Neon source URL"
export AWS_REGION="$REGION"
export SECRET_ID
export DUMP_DIR
export IF_EMPTY
python3 - <<'PY'
import json, os, subprocess, sys
from urllib.parse import parse_qsl, urlencode, urlparse, urlunparse

def parse_env_file(path):
    vals = {}
    try:
        text = open(path).read()
    except OSError:
        return vals
    for line in text.splitlines():
        s = line.strip()
        if not s or s.startswith("#") or "=" not in s:
            continue
        k, v = s.split("=", 1)
        vals[k.strip()] = v.strip().strip("'").strip('"')
    return vals

def sm_blob():
    sid = os.environ.get("SECRET_ID") or ""
    region = os.environ.get("AWS_REGION") or "us-east-1"
    if not sid:
        return {}
    p = subprocess.run(
        [
            "aws", "secretsmanager", "get-secret-value",
            "--region", region, "--secret-id", sid,
            "--query", "SecretString", "--output", "text",
        ],
        capture_output=True, text=True,
    )
    if p.returncode != 0 or not p.stdout.strip():
        return {}
    try:
        data = json.loads(p.stdout)
    except json.JSONDecodeError:
        return {}
    return data if isinstance(data, dict) else {}

def jdbc_to_postgres(jdbc, user, password):
    raw = jdbc
    if raw.startswith("jdbc:"):
        raw = raw[5:]
    if raw.startswith("postgresql://") and "://" in raw:
        # postgresql://host/db?...  (JDBC has no userinfo)
        rest = raw[len("postgresql://"):]
        hostpath, _, query = rest.partition("?")
        host, _, db = hostpath.partition("/")
        user_q = user or "kalke"
        pw_q = password or ""
        q = query or "sslmode=require"
        return f"postgres://{user_q}:{pw_q}@{host}/{db}?{q}"
    return raw

def to_direct(url: str) -> str:
    p = urlparse(url)
    host = p.hostname or ""
    host = host.replace("-pooler", "")
    # Drop currentSchema (libpq ignores it; keep sslmode).
    q = []
    ssl_true = False
    for k, v in parse_qsl(p.query, keep_blank_values=True):
        kl = k.lower()
        if kl == "ssl":
            ssl_true = v.lower() in ("1", "true", "require")
            continue
        if kl == "currentschema":
            continue
        q.append((k, v))
    if ssl_true or not any(k.lower() == "sslmode" for k, _ in q):
        if not any(k.lower() == "sslmode" for k, _ in q):
            q.append(("sslmode", "require"))
    netloc = p.netloc
    if p.hostname:
        auth = ""
        if p.username is not None:
            auth = p.username
            if p.password is not None:
                auth += ":" + p.password
            auth += "@"
        port = f":{p.port}" if p.port else ""
        netloc = f"{auth}{host}{port}"
    return urlunparse((p.scheme or "postgres", netloc, p.path, p.params, urlencode(q), p.fragment))

file_vals = parse_env_file("prod.env")
data = sm_blob()
user = (
    os.environ.get("KC_DB_USERNAME")
    or data.get("KC_DB_USERNAME")
    or file_vals.get("KC_DB_USERNAME")
    or ""
)
password = (
    os.environ.get("KC_DB_PASSWORD")
    or data.get("KC_DB_PASSWORD")
    or file_vals.get("KC_DB_PASSWORD")
    or ""
)
candidates = [
    os.environ.get("NEON_DATABASE_URL") or "",
    data.get("NEON_DATABASE_URL") or "",
    data.get("DATABASE_URL") or "",
    os.environ.get("DATABASE_URL") or "",
    file_vals.get("NEON_DATABASE_URL") or "",
    file_vals.get("DATABASE_URL") or "",
]
url = ""
for c in candidates:
    c = (c or "").strip()
    if not c or "://postgres:5432" in c:
        continue
    if "postgres" in c.split(":")[0] or "neon.tech" in c:
        url = c
        break
if not url:
    jdbc = (data.get("KC_DB_URL") or file_vals.get("KC_DB_URL") or os.environ.get("KC_DB_URL") or "").strip()
    if jdbc:
        url = jdbc_to_postgres(jdbc, user, password)

if not url:
    if os.environ.get("IF_EMPTY") == "1":
        print("no Neon source URL; skip (--if-empty)")
        sys.exit(0)
    print("no Neon/source DATABASE_URL found in env or Secrets Manager", file=sys.stderr)
    sys.exit(1)
if "postgres" in url and "://postgres:5432" in url:
    print("source URL already points at local Docker postgres; nothing to dump", file=sys.stderr)
    sys.exit(0)

direct = to_direct(url)
open(os.path.join(os.environ["DUMP_DIR"], "source.url"), "w").write(direct)
print("resolved source host")
PY

if [[ ! -s "${DUMP_DIR}/source.url" ]]; then
  echo "no dump source (already on local Docker postgres, or URL unresolved)"
  exit 0
fi

# Pass the URL via file so it never lands in `ps` argv.
echo "==> Dumping source database"
docker run --rm \
  --network kalke-auth_default \
  -v "${DUMP_DIR}:/dump" \
  postgres:18-alpine \
  sh -c 'pg_dump --dbname="$(cat /dump/source.url)" --format=custom --no-owner --no-acl --file=/dump/neon.dump'

if [[ ! -s "${DUMP_DIR}/neon.dump" ]]; then
  echo "pg_dump produced an empty dump" >&2
  exit 1
fi

if [[ "$count" -gt 0 && "$FORCE" == 1 ]]; then
  echo "==> Dropping local schemas (--force)"
  "${COMPOSE[@]}" exec -T postgres psql -U "$PG_USER" -d "$PG_DB" -v ON_ERROR_STOP=1 <<SQL
DROP SCHEMA IF EXISTS keycloak CASCADE;
DROP SCHEMA IF EXISTS app CASCADE;
CREATE SCHEMA keycloak;
CREATE SCHEMA app;
SQL
fi

echo "==> Restoring into local postgres"
docker run --rm \
  --network kalke-auth_default \
  -v "${DUMP_DIR}:/dump" \
  -e PGPASSWORD="$(awk -F= '/^POSTGRES_PASSWORD=/{sub(/^[^=]*=/,""); gsub(/^['\''"]+|['\''"]+$/,""); print; exit}' prod.env)" \
  postgres:18-alpine \
  pg_restore --host=postgres --username="$PG_USER" --dbname="$PG_DB" \
    --no-owner --no-acl --verbose /dump/neon.dump || true

after="$(local_table_count)"
if [[ "$after" -lt 1 ]]; then
  echo "restore produced no keycloak/app tables" >&2
  exit 1
fi

echo "==> Verifying"
"${COMPOSE[@]}" exec -T postgres psql -U "$PG_USER" -d "$PG_DB" -v ON_ERROR_STOP=1 <<'SQL'
SELECT nspname AS schema, count(*) AS tables
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r' AND n.nspname IN ('keycloak', 'app', 'public')
GROUP BY 1
ORDER BY 1;
SQL

echo "migrated ${after} tables into local postgres"
