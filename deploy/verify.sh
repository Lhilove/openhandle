#!/usr/bin/env bash
#
# Verify a deployment end to end, from the outside.
#
#   bash deploy/verify.sh https://your-project.vercel.app
#   bash deploy/verify.sh https://your-project.vercel.app --create-test-account
#
# The default checks are read-only. --create-test-account additionally registers
# a throwaway account, proves the session cookie survives the trip through the
# edge proxy, then signs out and deletes the account again.
#
set -euo pipefail

APP="${1:-}"
CREATE_ACCOUNT=0
for arg in "$@"; do
  case "$arg" in
    --create-test-account) CREATE_ACCOUNT=1 ;;
  esac
done

if [ -z "$APP" ] || [ "$APP" = "-h" ] || [ "$APP" = "--help" ]; then
  sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
fi
APP="${APP%/}"

C_RED='\033[31m' C_GREEN='\033[32m' C_BOLD='\033[1m' C_RESET='\033[0m'
FAILED=0

pass() { printf '  %bok%b   %s\n' "$C_GREEN" "$C_RESET" "$1"; }
fail() { printf '  %bFAIL%b %s\n' "$C_RED" "$C_RESET" "$1"; FAILED=$((FAILED + 1)); }
section() { printf '\n%b%s%b\n' "$C_BOLD" "$1" "$C_RESET"; }

# http METHOD PATH [curl args...] -> writes the body to $BODY_FILE, the response
# headers to $HEADERS_FILE and the status code to $STATUS.
BODY_FILE="$(mktemp)"
HEADERS_FILE="$(mktemp)"
trap 'rm -f "$BODY_FILE" "$HEADERS_FILE"' EXIT

http() {
  local method="$1" path="$2"; shift 2
  STATUS="$(curl -sS -o "$BODY_FILE" -D "$HEADERS_FILE" -w '%{http_code}' \
              -X "$method" "$APP$path" "$@" 2>/dev/null || echo 000)"
}

printf '%bOpenHandle deployment check%b  ->  %s\n' "$C_BOLD" "$C_RESET" "$APP"

section "1. Liveness through the edge proxy"
http GET /healthz
if [ "$STATUS" = "200" ] && grep -q '"status":"ok"' "$BODY_FILE"; then
  pass "/healthz answered 200 (the proxy reaches the API)"
else
  fail "/healthz returned $STATUS — check the Vercel rewrite destination and that the API is running"
fi

section "2. API path reaches the origin"
# A syntactically valid handle that should not exist, so the API answers with the
# handler's own 404 rather than a validation error.
PARAM_HANDLE="verify_probe_$$"
http GET "/api/v1/handles/$PARAM_HANDLE"
if [ "$STATUS" = "404" ] && grep -q '"error"' "$BODY_FILE"; then
  pass "/api/v1/* is proxied to the API (404 for an unknown handle, as expected)"
else
  fail "/api/v1/* returned $STATUS — 200 means the SPA is swallowing API paths, 000 means nothing answered"
fi

section "3. Credentials are rejected without leaking a session"
EMAIL="verify-$$-$(date +%s)@example.invalid"
http POST /api/v1/auth/login -H 'Content-Type: application/json' \
     -d "{\"email\":\"$EMAIL\",\"password\":\"not-the-right-password\"}"
COOKIE_LINES="$(grep -ci '^set-cookie:' "$HEADERS_FILE" || true)"
if [ "$STATUS" = "401" ] && [ "$COOKIE_LINES" = "0" ]; then
  pass "wrong password -> 401 and no session cookie"
else
  fail "expected 401 with no Set-Cookie, got $STATUS and $COOKIE_LINES cookie header(s)"
fi

section "4. Security headers on the API"
if grep -qi '^cache-control:.*no-store' "$HEADERS_FILE"; then
  pass "API responses are marked no-store (an edge cache will not keep them)"
else
  fail "API response is missing Cache-Control: no-store — with Vercel's rewrite caching enabled that could serve one user's data to another"
fi

if [ "$CREATE_ACCOUNT" = "1" ]; then
  section "5. Full session round trip (creates and deletes a throwaway account)"
  http POST /api/v1/auth/register -H 'Content-Type: application/json' \
       -d "{\"email\":\"$EMAIL\",\"password\":\"verify-account-password-123\"}"
  if [ "$STATUS" = "201" ]; then
    pass "registered $EMAIL"
  else
    fail "registration returned $STATUS: $(head -c 200 "$BODY_FILE")"
  fi

  SET_COOKIE="$(grep -i '^set-cookie: oh_session' "$HEADERS_FILE" || true)"
  if printf '%s' "$SET_COOKIE" | grep -qi 'HttpOnly'; then
    pass "session cookie is HttpOnly"
  else
    fail "session cookie is not HttpOnly: $SET_COOKIE"
  fi
  if printf '%s' "$SET_COOKIE" | grep -qi 'SameSite=None'; then
    printf '  note  cookie is SameSite=None: the browser calls the API cross-site. Safari blocks those cookies.\n'
  fi

  # The real test: does the browser-visible origin accept the cookie back?
  CSRF="$(sed -n 's/.*"csrf_token":"\([^"]*\)".*/\1/p' "$BODY_FILE" | head -1)"
  COOKIE_VALUE="$(printf '%s' "$SET_COOKIE" | sed -n 's/.*oh_session=\([^;]*\).*/\1/p' | head -1)"

  http GET /api/v1/me -H "Cookie: oh_session=$COOKIE_VALUE"
  if [ "$STATUS" = "200" ] && grep -q "$EMAIL" "$BODY_FILE"; then
    pass "the session cookie is sent back and resolves to the right account (login works through the proxy)"
  else
    fail "/api/v1/me returned $STATUS with the cookie — sign-in will not persist for users"
  fi

  http POST /api/v1/auth/logout -H "Cookie: oh_session=$COOKIE_VALUE" -H "X-CSRF-Token: $CSRF"
  [ "$STATUS" = "200" ] && pass "logout accepted" || fail "logout returned $STATUS"

  http GET /api/v1/me -H "Cookie: oh_session=$COOKIE_VALUE"
  [ "$STATUS" = "401" ] && pass "session is gone after logout" || fail "/api/v1/me returned $STATUS after logout"

  # Clean up: delete the account we just made (this also releases the handle).
  http POST /api/v1/auth/login -H 'Content-Type: application/json' \
       -d "{\"email\":\"$EMAIL\",\"password\":\"verify-account-password-123\"}"
  COOKIE_VALUE="$(grep -i '^set-cookie: oh_session' "$HEADERS_FILE" | sed -n 's/.*oh_session=\([^;]*\).*/\1/p' | head -1)"
  CSRF="$(sed -n 's/.*"csrf_token":"\([^"]*\)".*/\1/p' "$BODY_FILE" | head -1)"
  http DELETE /api/v1/me -H "Cookie: oh_session=$COOKIE_VALUE" -H "X-CSRF-Token: $CSRF"
  [ "$STATUS" = "200" ] && pass "throwaway account deleted" || fail "account cleanup returned $STATUS"
else
  section "5. Full session round trip"
  printf '  skipped (pass --create-test-account to create and delete a throwaway account)\n'
fi

printf '\n'
if [ "$FAILED" -eq 0 ]; then
  printf '%bAll checks passed.%b\n' "$C_GREEN" "$C_RESET"
else
  printf '%b%d check(s) failed.%b\n' "$C_RED" "$FAILED" "$C_RESET"
  exit 1
fi
