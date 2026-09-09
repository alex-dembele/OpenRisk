#!/usr/bin/env bash
# OpenRisk — one-command self-host installer.
#
# Tested on a clean Ubuntu 24.04 and Debian 12 VM (see the monthly CI job
# .github/workflows/selfhost-install.yml, which provisions a fresh runner and
# runs this end-to-end). It is idempotent: re-running keeps your existing .env
# and keys.
#
#   curl -fsSL https://raw.githubusercontent.com/opendefender/OpenRisk/master/scripts/install.sh | bash
# or, from a clone:
#   ./scripts/install.sh
set -euo pipefail

REPO_URL="${OPENRISK_REPO_URL:-https://github.com/opendefender/OpenRisk.git}"
BRANCH="${OPENRISK_BRANCH:-master}"

log()  { printf '\033[1;36m[openrisk]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[openrisk]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[openrisk]\033[0m %s\n' "$*" >&2; exit 1; }

# --- 1. Prerequisites --------------------------------------------------------
command -v docker >/dev/null 2>&1 || die "Docker is not installed. See https://docs.docker.com/engine/install/"
if docker compose version >/dev/null 2>&1; then
  DC="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  DC="docker-compose"
else
  die "Docker Compose plugin is not installed. See https://docs.docker.com/compose/install/"
fi
docker info >/dev/null 2>&1 || die "Cannot talk to the Docker daemon (is it running / do you have permission?)."

# --- 2. Locate the repo (clone if piped from curl) ---------------------------
if [ -f "deploy/selfhost/docker-compose.yml" ]; then
  ROOT="$(pwd)"
elif [ -f "$(dirname "$0")/../deploy/selfhost/docker-compose.yml" ]; then
  ROOT="$(cd "$(dirname "$0")/.." && pwd)"
else
  command -v git >/dev/null 2>&1 || die "git is required to fetch OpenRisk."
  ROOT="${OPENRISK_DIR:-$HOME/openrisk}"
  if [ ! -d "$ROOT/.git" ]; then
    log "Cloning OpenRisk into $ROOT ..."
    git clone --depth 1 --branch "$BRANCH" "$REPO_URL" "$ROOT"
  fi
fi
cd "$ROOT/deploy/selfhost"
log "Deployment directory: $(pwd)"

# --- 3. Secret generation helpers -------------------------------------------
rand_hex() { openssl rand -hex "${1:-32}"; }
rand_b64_32() { openssl rand -base64 32; }
command -v openssl >/dev/null 2>&1 || die "openssl is required to generate secrets."

# --- 4. RSA keypair for RS256 JWTs ------------------------------------------
mkdir -p secrets
if [ ! -f secrets/private.pem ]; then
  log "Generating RS256 keypair ..."
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out secrets/private.pem 2>/dev/null
  openssl rsa -in secrets/private.pem -pubout -out secrets/public.pem 2>/dev/null
  chmod 600 secrets/private.pem
else
  log "RSA keypair already present — keeping it."
fi

# --- 5. .env with strong random secrets -------------------------------------
if [ ! -f .env ]; then
  log "Creating .env with generated secrets ..."
  cp .env.example .env
  # Fill the empty required fields with generated values (portable sed).
  set_kv() { # key value
    if grep -q "^$1=" .env; then
      # Use a temp file so any char in $2 is safe.
      awk -v k="$1" -v v="$2" 'BEGIN{FS=OFS="="} $1==k{$0=k"="v} {print}' .env > .env.tmp && mv .env.tmp .env
    else
      printf '%s=%s\n' "$1" "$2" >> .env
    fi
  }
  set_kv DB_PASSWORD "$(rand_hex 24)"
  set_kv MFA_ENCRYPTION_KEY "$(rand_hex 32)"       # 32 bytes hex → 64 chars; backend takes 32 bytes
  set_kv SCANNER_CREDENTIAL_KEY "$(rand_b64_32)"
  set_kv AUDIT_EXPORT_KEY "$(rand_b64_32)"
  chmod 600 .env
else
  log ".env already present — keeping your configuration."
fi

# --- 6. Bring the stack up ---------------------------------------------------
log "Building and starting the stack (this can take a few minutes on first run) ..."
$DC up -d --build

# --- 7. Wait for backend health ---------------------------------------------
PORT="$(grep -E '^BACKEND_PORT=' .env | cut -d= -f2)"; PORT="${PORT:-8080}"
FPORT="$(grep -E '^FRONTEND_PORT=' .env | cut -d= -f2)"; FPORT="${FPORT:-3000}"
log "Waiting for the backend to become healthy on :$PORT ..."
HEALTHY=0
for _ in $(seq 1 60); do
  if curl -fsS "http://localhost:${PORT}/api/v1/health" >/dev/null 2>&1; then
    HEALTHY=1
    break
  fi
  sleep 3
done
if [ "$HEALTHY" -ne 1 ]; then
  warn "Backend did not report healthy in time. Inspect logs with:"
  warn "  (cd deploy/selfhost && $DC logs backend)"
  exit 1
fi
log "Backend is healthy."

# --- 8. The first administrator ----------------------------------------------
# An install that ends at "the stack is up" is not finished: the operator still
# has to work out how to get in. This creates the first account and prints its
# credentials (#328).
#
# It goes through the product's OWN public registration endpoint rather than a
# SQL insert, so the first account is created exactly the way every other account
# is and cannot drift from the real registration path. That endpoint also creates
# the organisation and makes this user its root member.
#
# The password is generated here, printed ONCE, and written to no file. If you
# lose it, use the app's password reset, or delete the user and re-run.
ADMIN_EMAIL="${OPENRISK_ADMIN_EMAIL:-admin@openrisk.local}"
ADMIN_USERNAME="${OPENRISK_ADMIN_USERNAME:-admin}"
ADMIN_NAME="${OPENRISK_ADMIN_NAME:-OpenRisk Administrator}"
ADMIN_ORG="${OPENRISK_ORG_NAME:-My Organization}"

# 32 alphanumeric characters from OpenSSL's CSPRNG (the backend requires >= 12).
# Deliberately not `tr -dc < /dev/urandom | head -c`: `head` closing the pipe
# SIGPIPEs its producer, which trips the `set -o pipefail` above.
gen_password() {
  local raw
  raw="$(openssl rand -base64 64 | tr -d '\n+/=')"
  printf '%s' "${raw:0:32}"
}

json_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

if [ "${OPENRISK_SKIP_ADMIN:-0}" = "1" ]; then
  log "OPENRISK_SKIP_ADMIN=1 — not creating an administrator."
  ADMIN_STATE="skipped"
else
  ADMIN_PASSWORD="$(gen_password)"
  log "Creating the first administrator ..."
  # The response body carries the new user and organisation; it is discarded
  # rather than written anywhere.
  REGISTER_STATUS="$(curl -s -o /dev/null -w '%{http_code}' \
    -X POST "http://localhost:${PORT}/api/v1/auth/register" \
    -H 'Content-Type: application/json' \
    -d "$(printf '{"email":"%s","username":"%s","password":"%s","full_name":"%s","company_name":"%s"}' \
          "$(json_escape "$ADMIN_EMAIL")" \
          "$(json_escape "$ADMIN_USERNAME")" \
          "$ADMIN_PASSWORD" \
          "$(json_escape "$ADMIN_NAME")" \
          "$(json_escape "$ADMIN_ORG")")" || true)"

  case "$REGISTER_STATUS" in
    201)     ADMIN_STATE="created" ;;
    409)     ADMIN_STATE="exists" ;;
    *)       ADMIN_STATE="failed" ;;
  esac
fi

# --- 9. Tell the operator what they have ------------------------------------
log "✅ OpenRisk is up."
log "   • App:  http://localhost:${FPORT}"
log "   • API:  http://localhost:${PORT}/api/v1"
log "   • Logs: (cd deploy/selfhost && $DC logs -f)"

case "$ADMIN_STATE" in
  created)
    printf '\n'
    log "Sign in with:"
    log "   • Email:    ${ADMIN_EMAIL}"
    log "   • Password: ${ADMIN_PASSWORD}"
    printf '\n'
    warn "This password is shown once and stored nowhere. Save it now."
    ;;
  exists)
    log "An account for ${ADMIN_EMAIL} already exists — keeping it, password unchanged."
    ;;
  skipped)
    log "No administrator was created. Register the first account at http://localhost:${FPORT}."
    ;;
  *)
    warn "Could not create the first administrator (HTTP ${REGISTER_STATUS:-no response})."
    warn "The stack is up: register the first account at http://localhost:${FPORT}."
    ;;
esac

exit 0
