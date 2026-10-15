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

## Stage 9 — Payments that the network never accepts
- [x] A signature the chain still cannot see after the blockhash window is
      reported as `expired` ("Not sent") within ~3 minutes instead of sitting at
      "Pending" for an hour. The record is re-checked slowly for a day, so a
      transaction that lands late is still corrected to `confirmed`.
- [x] The client confirms with the cluster after the wallet returns a signature
      and refuses to record a payment whose blockhash already expired: no
      phantom "pending" payment for a transfer that never happened.
- [x] Signatures are validated as base58 64-byte values on ingest, and the
      worker falls back to one-by-one status checks, so a single malformed
      signature cannot stall confirmation for every pending payment.
- [x] Tests: fake-RPC tests for the drop decision and the late-arrival recovery,
      plus a rejection test for malformed signatures. Exercised end to end
      against devnet and in a browser here.

## Stage 10 — One payment, two sides, one status
- [x] A payment is visible to both parties. History was scoped to the account
      that submitted the transaction, so a recipient had **no record of a tip at
      all**: the delivery was invisible to the person who was paid even though
      the sender's history showed it. `/api/v1/me/transactions` now returns the
      payments a user sent *and* the ones paid to them, on one timeline, each
      with `direction` and the counterparty (handle when public, address
      always). `GET /api/v1/transactions/{signature}` is readable by either
      party. One row, two views: the sides cannot report different statuses.
- [x] An unreachable RPC endpoint no longer becomes a chain verdict. The old
      worker discarded the error from its chain lookup and fell through to
      `expired`, so a provider outage (a rejected API key answers `401`)
      reported a payment that had **already finalized on mainnet** as
      "Not sent" — telling the sender their money had never left the wallet.
      A response the endpoint never gave is now `unknown`, not `missing`: the
      record stays `submitted` and is re-checked (`GET /readyz` reports
      `"rpc": false`, the worker logs why). Verified against a real finalized
      mainnet payment that had been reported `expired`.
- [x] Expired records are re-checked for a week rather than a day, so a wrong
      verdict heals by itself once the endpoint recovers.
- [x] Tests: an endpoint that answers with `401` leaves records pending; a
      failed chain lookup past the blockhash window does not become `expired`
      (this test fails against the old code with exactly the reported symptom);
      sent and received history, attribution by wallet address, and no leak of a
      counterparty handle its owner has hidden.

## Documented limitations
- Social login/verification is **not** faked. Without OAuth credentials the UI
  shows "Not configured"; no verified badge is ever shown without a real
  provider verification.
- Balance display is intentionally **omitted** (no reliable balance indexer
  wired). The dashboard does not render a balance as if verified.
- Transaction history covers the payments **sent and received through
  OpenHandle**; it is not an exhaustive on-chain history, so a transfer made
  outside OpenHandle does not appear.
- Confirmation requires a reachable RPC endpoint. While `/readyz` reports
  `"rpc": false` nothing is confirmed and payments stay `submitted` — honest, but
  not final until the endpoint works again.
- Devnet is the default cluster. Mainnet must be opted into explicitly.
