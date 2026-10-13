#!/usr/bin/env bash
#
# OpenHandle — one-shot production bootstrap for a single Linux VM
# (written for Oracle Cloud free tier, works on any Debian/Ubuntu/Oracle
# Linux/RHEL host).
#
#   git clone https://github.com/Lhilove/openhandle.git
#   cd openhandle
#   sudo bash deploy/bootstrap.sh
#
# It installs Docker, opens the firewall for HTTP/HTTPS, generates secrets,
# builds the API image and starts Postgres + API + Caddy.
#
# Run it with no arguments to answer a few prompts, or set the values below as
# environment variables (or --yes to accept the detected defaults):
#
#   APP_BASE_URL       your Vercel URL               (default https://openhandle.vercel.app)
#   API_DOMAIN         hostname for the API origin   (default <label>-<dashed-ip>.sslip.io)
#   SOLANA_CLUSTER     devnet | testnet | mainnet-beta (default devnet)
#   SOLANA_RPC_URL     RPC endpoint                  (default: the cluster's public RPC)
#   GITHUB_CLIENT_ID / GITHUB_CLIENT_SECRET          (optional, enables GitHub identity)
#   ORIGIN_SECRET      optional; see docs/DEPLOYMENT.md before enabling
#
# Preview without changing anything:  sudo DRY_RUN=1 bash deploy/bootstrap.sh --yes
#
set -euo pipefail

REPO_SLUG="${REPO_SLUG:-Lhilove/openhandle}"
REPO_URL="${REPO_URL:-https://github.com/${REPO_SLUG}.git}"
INSTALL_DIR="${INSTALL_DIR:-/opt/openhandle}"
COMPOSE_FILE="deploy/docker-compose.prod.yml"
ENV_FILE="deploy/.env"
PROJECT_NAME="openhandle"

ASSUME_YES="${ASSUME_YES:-0}"
DRY_RUN="${DRY_RUN:-0}"

APP_BASE_URL="${APP_BASE_URL:-}"
API_DOMAIN="${API_DOMAIN:-}"
SOLANA_CLUSTER="${SOLANA_CLUSTER:-devnet}"
SOLANA_RPC_URL="${SOLANA_RPC_URL:-}"
GITHUB_CLIENT_ID="${GITHUB_CLIENT_ID:-}"
GITHUB_CLIENT_SECRET="${GITHUB_CLIENT_SECRET:-}"
ORIGIN_SECRET="${ORIGIN_SECRET:-}"

for arg in "$@"; do
  case "$arg" in
    -y|--yes) ASSUME_YES=1 ;;
    --dry-run) DRY_RUN=1 ;;
    -h|--help) sed -n '2,31p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) printf 'unknown argument: %s\n' "$arg" >&2; exit 2 ;;
  esac
done

readonly C_RESET='\033[0m' C_BOLD='\033[1m' C_DIM='\033[2m' C_YELLOW='\033[33m'

bold() { printf '%b%s%b\n' "$C_BOLD" "$1" "$C_RESET"; }
note() { printf '  %b%s%b\n' "$C_DIM" "$1" "$C_RESET"; }
warn() { printf '%bwarning:%b %s\n' "$C_YELLOW" "$C_RESET" "$1" >&2; }
die() { printf 'error: %s\n' "$1" >&2; exit 1; }

# run executes a command, or only prints it in dry-run mode.
run() {
  if [ "$DRY_RUN" = "1" ]; then
    printf '  %bdry-run:%b %s\n' "$C_DIM" "$C_RESET" "$*"
    return 0
  fi
  "$@"
}

have() { command -v "$1" >/dev/null 2>&1; }

compose() {
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" -p "$PROJECT_NAME" "$@"
}

# ---------------------------------------------------------------- preflight --

step_preflight() {
  bold "1/8  Checking the host"
  if [ "$DRY_RUN" != "1" ] && [ "$(id -u)" -ne 0 ]; then
    die "run this with sudo (it installs packages and edits firewall rules)"
  fi
  if [ "$(uname -s)" != "Linux" ]; then
    warn "this script targets Linux; dry-run only on other systems"
  fi

  PKG=""
  if have apt-get; then PKG=apt
  elif have dnf; then PKG=dnf
  elif have yum; then PKG=yum
  elif [ "$DRY_RUN" = "1" ]; then
    PKG=none
    warn "no apt/dnf/yum on this host; continuing because this is a dry run"
  else
    die "no supported package manager found (apt/dnf/yum)"
  fi

  ARCH="$(uname -m)"
  if [ "$PKG" != "none" ]; then
    note "package manager: $PKG   architecture: $ARCH"
  fi
  note "mem: $(awk '/MemTotal/ {printf "%.1f GB", $2/1048576}' /proc/meminfo 2>/dev/null || echo unknown)"
  case "$ARCH" in
    x86_64|aarch64) ;;
    *) warn "unusual architecture $ARCH: check that the images have a matching build" ;;
  esac
}

step_repo() {
  bold "2/8  Locating the repository"
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  if [ -f "$SCRIPT_DIR/../$COMPOSE_FILE" ]; then
    REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
    note "using the checkout at $REPO_DIR"
  else
    REPO_DIR="$INSTALL_DIR"
    note "no checkout next to this script; will clone into $REPO_DIR"
    run install -d -m 0750 "$INSTALL_DIR"
    if [ -d "$REPO_DIR/.git" ]; then
      run git -C "$REPO_DIR" pull --ff-only
    else
      run git clone --depth 1 "$REPO_URL" "$REPO_DIR" ||
        die "could not clone $REPO_URL — the repository must be reachable"
    fi
  fi
  cd "$REPO_DIR"
  [ -f "$COMPOSE_FILE" ] || die "$COMPOSE_FILE not found in $REPO_DIR"
}

# ensure_docker_group lets the invoking user drive Docker without sudo, which is
# what every command in docs/DEPLOYMENT.md and deploy/backup.sh assumes.
# Without it `docker compose ps` answers "permission denied while trying to
# connect to the docker API", which is a confusing dead end right after a
# successful install. Group membership only takes effect on a new login.
ensure_docker_group() {
  local user="${SUDO_USER:-}"
  [ -n "$user" ] || return 0
  [ "$user" != "root" ] || return 0
  if id -nG "$user" 2>/dev/null | tr ' ' '\n' | grep -qx docker; then
    note "$user can already run docker"
    return 0
  fi
  run usermod -aG docker "$user"
  warn "$user was added to the docker group — log out and back in for it to take effect"
}

step_docker() {
  bold "3/8  Docker"
  if have docker && docker compose version >/dev/null 2>&1; then
    note "docker $(docker version --format '{{.Server.Version}}' 2>/dev/null || echo present) with the compose v2 plugin"
    ensure_docker_group
    return
  fi

  note "installing docker from the distribution packages"
  case "$PKG" in
    apt)
      run apt-get update -y
      run apt-get install -y ca-certificates curl git openssl
      run apt-get install -y docker.io || true
      run apt-get install -y docker-compose-v2 || run apt-get install -y docker-compose-plugin || true
      ;;
    dnf|yum)
      run "$PKG" install -y ca-certificates curl git openssl
      run "$PKG" install -y docker-engine || run "$PKG" install -y docker || run "$PKG" install -y moby-engine || true
      run "$PKG" install -y docker-compose-plugin || true
      ;;
  esac

  run systemctl enable --now docker || true

  if [ "$DRY_RUN" != "1" ]; then
    for _ in $(seq 1 30); do
      docker info >/dev/null 2>&1 && break
      sleep 2
    done
  fi

  if ! (have docker && docker compose version >/dev/null 2>&1); then
    printf '\nDocker Compose v2 is still missing. The official installer from\n'
    printf 'https://get.docker.com adds both the daemon and the compose plugin.\n'
    if [ "$ASSUME_YES" != "1" ] && [ "$DRY_RUN" != "1" ] && [ -t 0 ]; then
      printf 'Download and run it now? [y/N] '
      read -r reply </dev/tty || reply=""
      case "$reply" in [yY]*) ;; *) die "install Docker + compose v2 and re-run" ;; esac
    elif [ "$ASSUME_YES" != "1" ]; then
      die "install Docker + compose v2 (or pass --yes to run the official installer) and re-run"
    fi
    run sh -c 'curl -fsSL https://get.docker.com | sh'
  fi

  ensure_docker_group
}

step_firewall() {
  bold "4/8  Firewall (ports 80 and 443 must be reachable for certificates)"
  if have ufw && ufw status 2>/dev/null | grep -qi '^Status: active'; then
    note "ufw is active"
    run ufw allow 80/tcp
    run ufw allow 443/tcp
  fi
  if have firewall-cmd && firewall-cmd --state >/dev/null 2>&1; then
    note "firewalld is active"
    run firewall-cmd --permanent --add-service=http
    run firewall-cmd --permanent --add-service=https
    run firewall-cmd --reload
  fi
  if have iptables; then
    # Oracle's images ship an INPUT chain that REJECTs inbound traffic. Insert
    # the accept rules ahead of that rule, and persist them.
    for port in 80 443; do
      if iptables -C INPUT -p tcp --dport "$port" -j ACCEPT >/dev/null 2>&1; then
        note "port $port already accepted by iptables"
        continue
      fi
      rule_line="$(iptables -L INPUT --line-numbers -n 2>/dev/null | awk '/REJECT|DROP/ {print $1; exit}' || true)"
      if [ -n "$rule_line" ]; then
        note "inserting an ACCEPT rule for port $port before iptables rule $rule_line"
        run iptables -I INPUT "$rule_line" -p tcp --dport "$port" -j ACCEPT
      else
        run iptables -A INPUT -p tcp --dport "$port" -j ACCEPT
      fi
    done
    if have netfilter-persistent; then
      run netfilter-persistent save
    elif [ -d /etc/iptables ]; then
      run sh -c 'iptables-save > /etc/iptables/rules.v4'
    elif have service && service iptables status >/dev/null 2>&1; then
      run service iptables save
    else
      warn "could not persist the iptables rules; they may not survive a reboot"
    fi
  fi
  printf '\n'
  warn "Oracle Cloud: the instance firewall above is not enough. Add ingress rules"
  warn "for TCP 80 and 443 in the VCN Security List (or Network Security Group):"
  warn "  Networking -> Virtual Cloud Networks -> your VCN -> Security Lists ->"
  warn "  Default Security List -> Add Ingress Rules (Source 0.0.0.0/0, TCP 80, TCP 443)"
}

# ------------------------------------------------------------------ config --

detect_public_ip() {
  local ip=""
  for url in https://api.ipify.org https://ifconfig.me/ip https://icanhazip.com; do
    ip="$(curl -fsS --max-time 5 "$url" 2>/dev/null | tr -d '[:space:]')" || true
    case "$ip" in ''|*[!0-9.]*) continue ;; esac
    printf '%s' "$ip"
    return 0
  done
  # Oracle metadata service, when outbound HTTP is blocked.
  ip="$(curl -fsS --max-time 3 -H 'Authorization: Bearer Oracle' \
        http://169.254.169.254/opc/v2/vnics/ 2>/dev/null \
        | grep -o '"publicIp":"[^"]*"' | head -1 | cut -d'"' -f4)" || true
  case "$ip" in ''|*[!0-9.]*) return 1 ;; esac
  printf '%s' "$ip"
}

# ask VAR "prompt" "default" — an already-set environment value always wins.
ask() {
  local var="$1" prompt="$2" default="$3" current answer
  current="${!var:-}"
  if [ -n "$current" ]; then return 0; fi
  if [ "$ASSUME_YES" = "1" ] || [ "$DRY_RUN" = "1" ] || [ ! -t 0 ]; then
    printf -v "$var" '%s' "$default"
    note "using $var=$default"
    return 0
  fi
  printf '%s [%s]: ' "$prompt" "$default"
  read -r answer </dev/tty || answer=""
  [ -n "$answer" ] || answer="$default"
  printf -v "$var" '%s' "$answer"
}

gen_secret() {
  if have openssl; then
    openssl rand -hex 32
  else
    od -An -tx1 -N32 /dev/urandom | tr -d ' \n'
  fi
}

# env_value FILE KEY — read one key without sourcing the file. Sourcing would
# execute any command substitution an editor (or an attacker) left in it.
env_value() {
  [ -f "$1" ] || return 0
  sed -n "s/^$2=//p" "$1" | tail -1
}

step_config() {
  bold "5/8  Configuration"

  PUBLIC_IP="$(detect_public_ip)" || PUBLIC_IP=""
  if [ -n "$PUBLIC_IP" ]; then
    note "public address: $PUBLIC_IP"
  else
    warn "could not detect the public address; set API_DOMAIN yourself"
  fi

  ask APP_BASE_URL "Public URL of your Vercel deployment" "https://openhandle.vercel.app"
  case "$APP_BASE_URL" in
    https://*) ;;
    *) die "APP_BASE_URL must be https (browsers block Secure cookies on http)" ;;
  esac
  APP_BASE_URL="${APP_BASE_URL%/}"

  default_domain=""
  if [ -n "$PUBLIC_IP" ]; then
    default_domain="openhandle-api-$(printf '%s' "$PUBLIC_IP" | tr '.' '-').sslip.io"
  fi
  ask API_DOMAIN "Hostname for the API (no domain of your own? keep the sslip.io default)" "$default_domain"
  [ -n "$API_DOMAIN" ] || die "API_DOMAIN is required"
  API_DOMAIN="${API_DOMAIN#https://}"; API_DOMAIN="${API_DOMAIN#http://}"; API_DOMAIN="${API_DOMAIN%%/*}"

  ask SOLANA_CLUSTER "Solana cluster (devnet/testnet/mainnet-beta)" "devnet"
  default_rpc="https://api.${SOLANA_CLUSTER}.solana.com"
  ask SOLANA_RPC_URL "Solana RPC endpoint" "$default_rpc"

  ask GITHUB_CLIENT_ID "GitHub OAuth app client id (optional, Enter to skip)" ""
  if [ -n "$GITHUB_CLIENT_ID" ]; then
    ask GITHUB_CLIENT_SECRET "GitHub OAuth app client secret" ""
  fi

  note "app:  $APP_BASE_URL"
  note "api:  https://$API_DOMAIN"
  if [ "$SOLANA_CLUSTER" = "mainnet-beta" ]; then
    warn "SOLANA_CLUSTER=mainnet-beta: this stack will handle real funds"
  fi
}

step_env_file() {
  bold "6/8  Writing $ENV_FILE"

  # Carry the generated secrets and database identity over from an earlier run,
  # so re-running does not sign everyone out or orphan the data volume. Every
  # other value comes from this run (step 5).
  if [ -f "$ENV_FILE" ]; then
    note "existing $ENV_FILE found; reusing its secrets and database name"
    SESSION_SECRET="${SESSION_SECRET:-$(env_value "$ENV_FILE" SESSION_SECRET)}"
    POSTGRES_PASSWORD="$(env_value "$ENV_FILE" POSTGRES_PASSWORD)"
    POSTGRES_USER="$(env_value "$ENV_FILE" POSTGRES_USER)"
    POSTGRES_DB="$(env_value "$ENV_FILE" POSTGRES_DB)"
  fi
  SESSION_SECRET="${SESSION_SECRET:-$(gen_secret)}"
  POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-$(gen_secret)}"
  POSTGRES_USER="${POSTGRES_USER:-openhandle}"
  POSTGRES_DB="${POSTGRES_DB:-openhandle}"
  [ -n "$POSTGRES_PASSWORD" ] || die "could not obtain a database password"

  if [ "$DRY_RUN" = "1" ]; then
    note "dry-run: not writing $ENV_FILE"
    return
  fi

  umask 077
  cat > "$ENV_FILE" <<EOF
# Written by deploy/bootstrap.sh on $(date -u '+%Y-%m-%dT%H:%M:%SZ'). Keep this file private.

APP_BASE_URL=$APP_BASE_URL
API_BASE_URL=https://$API_DOMAIN
CORS_ALLOWED_ORIGINS=$APP_BASE_URL

API_DOMAIN=$API_DOMAIN

POSTGRES_USER=$POSTGRES_USER
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_DB=$POSTGRES_DB

SESSION_SECRET=$SESSION_SECRET

CLIENT_IP_MODE=forwarded
COOKIE_SAMESITE=lax
ORIGIN_SECRET=$ORIGIN_SECRET

SOLANA_CLUSTER=$SOLANA_CLUSTER
SOLANA_RPC_URL=$SOLANA_RPC_URL

GITHUB_CLIENT_ID=$GITHUB_CLIENT_ID
GITHUB_CLIENT_SECRET=$GITHUB_CLIENT_SECRET

LOG_LEVEL=info
EOF
  chmod 600 "$ENV_FILE"
  note "wrote $ENV_FILE (mode 600)"
}

# -------------------------------------------------------------------- start --

step_start() {
  bold "7/8  Building and starting the stack"
  run compose up -d --build
  if [ "$DRY_RUN" = "1" ]; then return; fi

  printf '  '
  for _ in $(seq 1 60); do
    printf '.'
    # --resolve makes this a real TLS check against the domain's certificate
    # while connecting straight to the local Caddy container.
    if curl -fsS --max-time 3 --resolve "$API_DOMAIN:443:127.0.0.1" \
         "https://$API_DOMAIN/healthz" >/dev/null 2>&1; then
      printf '\n'
      note "API answering over HTTPS at https://$API_DOMAIN"
      return
    fi
    sleep 2
  done
  printf '\n'
  warn "the API did not answer over HTTPS within ~2 minutes. Most likely causes:"
  warn "  * DNS for $API_DOMAIN does not point at this server"
  warn "  * TCP 80/443 are still closed in the Oracle VCN Security List"
  warn "  * Caddy could not obtain a certificate, or its config is invalid"
  printf '\n'
  compose ps || true
  printf '\n'
  # A Caddyfile parse error is reported by the config adapter, which does not
  # reach the container log when the process exits before it starts serving.
  printf 'caddy config check:\n'
  compose exec -T caddy caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile 2>&1 | tail -5 || true
  printf '\napi config check:\n'
  compose logs --tail=10 api || true
  printf '\ncaddy log:\n'
  compose logs --tail=15 caddy || true
  die "stack started but not healthy"
}

step_next() {
  bold "8/8  Done — finish the client side"
  cat <<EOF

The API is live at:   https://$API_DOMAIN
Health check:         curl -fsS https://$API_DOMAIN/healthz

1. Deploy the client on Vercel
     * Import github.com/$REPO_SLUG
     * Root Directory: frontend     (the project's vercel.json lives there)
     * Framework Preset: Vite, build command and output directory come from it
     * No environment variables are needed: the client calls the API on its own
       origin and Vercel proxies /api/* to this server.

2. Point the client at this API (one line, then commit and push):
     sed -i 's|__API_ORIGIN__|$API_DOMAIN|' frontend/vercel.json
   The deployed app keeps its session cookie first-party that way, which is what
   makes sign-in work in every browser.

3. Verify the whole path from your own machine:
     ./deploy/verify.sh $APP_BASE_URL

Re-running this script is safe: it keeps existing secrets and just rebuilds.
Logs:    cd $REPO_DIR && sudo docker compose --env-file $ENV_FILE -f $COMPOSE_FILE -p $PROJECT_NAME logs -f
         (deploy/$ENV_FILE is root-only, so these commands need sudo unless you
          copy the values into your own environment)
Update:  git pull && sudo bash deploy/bootstrap.sh --yes

EOF
}

main() {
  step_preflight
  step_repo
  step_docker
  step_firewall
  step_config
  step_env_file
  step_start
  step_next
}

main
