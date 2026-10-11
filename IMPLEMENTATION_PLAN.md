# OpenHandle — Implementation Plan

> Status legend: [x] done · [~] partial / documented limitation · [ ] not started

This file is the working checklist from the build spec (§3, §16). The repository
started empty, so the recommended stack (React + TypeScript + Vite + Tailwind
frontend, Go + Gin + PostgreSQL backend) was adopted as-is.

## Stage 1 — Inspect and scaffold
- [x] Inspect repository (empty, no git). Recorded stack decision.
- [x] Repository layout: `backend/`, `frontend/`, `docs/`.
- [x] Docker Compose for PostgreSQL.
- [x] `.env.example` with placeholders only.
- [x] Design tokens + shared layout (frontend).
- [x] Health `/healthz` and readiness `/readyz`.

## Stage 2 — Core interface
- [x] Landing page, responsive navigation.
- [x] Auth screens (register / login).
- [x] Dashboard shell.
- [x] Public profile `/@:handle`.
- [x] Handle-claim flow with loading / error / empty states.

## Stage 3 — Real backend
- [x] Auth + session cookies (HttpOnly, SameSite=Lax, CSRF token).
- [x] `users` and `handles` migrations.
- [x] Handle availability + claim (DB unique constraint, tx, race-safe).
- [x] Public resolution `GET /api/v1/handles/{handle}` (public fields only).
- [x] Profile update + privacy visibility controls.
- [x] Backend authorization tests (cross-user access).

## Stage 4 — Wallet integration
- [x] Wallet connect / disconnect (frontend adapter).
- [x] Single-use, short-lived, domain-bound challenge.
- [x] Server-side ed25519 signature verification.
- [x] Wallet ownership status surfaced distinctly from "connected".
- [x] Devnet/mainnet separation via `SOLANA_CLUSTER`.

## Stage 5 — Payments
- [x] Recipient resolution from handle.
- [x] Client-side transaction build + wallet signing (native SOL).
- [x] Transaction signature persistence (unique network+signature).
- [x] Confirmation polling with backoff; accurate status states.
- [x] History page from real records (scoped: only OpenHandle-submitted txs).
- [~] Automated devnet transfer test — requires keys/faucet; documented skip.

## Stage 6 — Social identities and docs
- [~] Provider interface + official OAuth scaffolding; providers ship
      **disabled** ("Not configured") because no OAuth credentials are present.
- [x] API docs, security docs, privacy/terms drafts, deployment guide.

## Stage 7 — Verification
- [x] gofmt / go vet / go test.
- [x] ESLint / tsc / vite build.
- [x] Manual review of privacy + authorization boundaries.
- [x] No fake data outside explicit demo mode.

## Stage 8 — Deployment (client on Vercel, API on a VM)
- [x] Production stack: `deploy/Dockerfile`, `deploy/docker-compose.prod.yml`
      (postgres + api + caddy), `deploy/Caddyfile` with automatic TLS.
- [x] `deploy/bootstrap.sh`: Docker install, firewall rules (ufw/firewalld/Oracle
      iptables), generated secrets, `deploy/.env`, build, start, diagnostics.
- [x] `deploy/verify.sh` (end-to-end check of a live deployment) and
      `deploy/backup.sh` (compressed dumps with retention).
- [x] Same-origin proxying so cookies stay first-party: `CLIENT_IP_MODE`,
      `httpx.NoStore`, `frontend/vercel.json` rewrites and security headers.
- [x] Optional origin lockdown (`ORIGIN_SECRET`) with config validation.
- [x] CI: backend tests against PostgreSQL, frontend build, image build,
      shell syntax and a bootstrap dry run.
- [x] Verified locally: image build, migrations, healthchecks, TLS through
      Caddy, no published database/API ports, origin-secret gate, cookie
      attributes and a session round trip through the proxy.
- [ ] Not verified here: the Vercel deployment itself and a public Let's Encrypt
      certificate (the local run used Caddy's internal CA). See
      `docs/DEPLOYMENT.md` for the two steps that exercise them.

## Documented limitations
- Social login/verification is **not** faked. Without OAuth credentials the UI
  shows "Not configured"; no verified badge is ever shown without a real
  provider verification.
- Balance display is intentionally **omitted** (no reliable balance indexer
  wired). The dashboard does not render a balance as if verified.
- Transaction history covers only transactions **submitted through OpenHandle**;
  it is not an exhaustive on-chain history.
- Devnet is the default cluster. Mainnet must be opted into explicitly.
