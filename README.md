# OpenHandle

**Your identity. Your money. Your handle.**

OpenHandle lets a person claim a human-readable handle, connect a self-custody
Solana wallet, prove they control it, and share a public profile that resolves
the handle to that wallet address. A sender opens `/@handle`, sees the address,
and pays from their own wallet.

OpenHandle is **not a wallet and not a custodian**. It never generates, collects
or stores private keys or seed phrases, and it never signs transactions on a
user's behalf.

---

## Repository layout

```
.
├── backend/                 Go + Gin API
│   ├── cmd/api/             server entrypoint, demo seed
│   └── internal/
│       ├── api/             HTTP handlers, DTOs, router, integration tests
│       ├── config/          env loading + validation, .env loader
│       ├── db/              pgx pool + embedded SQL migrations
│       ├── httpx/           errors, middleware (auth, CSRF, CORS, rate limit)
│       ├── identity/        pluggable OAuth identity providers (GitHub)
│       ├── models/          domain structs
│       ├── rpc/             minimal Solana JSON-RPC client
│       ├── store/           parameterized SQL data access
│       ├── validate/        handle/email/password/address validation
│       └── wallet/          ed25519 signature verification
├── frontend/                React + TypeScript + Vite + Tailwind
│   └── src/
│       ├── components/      layout shells + UI primitives
│       ├── lib/             API client, auth + wallet contexts, schemas, format
│       └── pages/           landing, auth, claim, dashboard, profile, send, …
├── deploy/                  production stack for a single VM
│   ├── Dockerfile           multi-stage build of the API image
│   ├── docker-compose.prod.yml  postgres + api + caddy
│   ├── Caddyfile            TLS and reverse proxy for the API origin
│   ├── bootstrap.sh         one-shot: Docker, firewall, secrets, start
│   ├── backup.sh            compressed database dumps with retention
│   └── verify.sh            end-to-end check of a live deployment
├── .github/workflows/ci.yml tests, build and image build
├── docs/                    API.md, DEPLOYMENT.md
├── docker-compose.yml       PostgreSQL for local development
├── SECURITY.md              threat model + reporting
├── PRIVACY.md / TERMS.md    draft policy documents (require review)
└── IMPLEMENTATION_PLAN.md   build checklist and status
```

## Architecture

- **Backend:** Go, Gin, PostgreSQL (pgx). Migrations are embedded and applied at
  startup. Sessions are server-side rows; the cookie holds a random token whose
  SHA-256 hash is stored. Mutating requests require a per-session CSRF token in
  the `X-CSRF-Token` header. Sensitive endpoints are rate-limited per IP.
- **Frontend:** React + TypeScript (strict) + Vite + Tailwind. Talks to the API
  with `credentials: "include"`. Wallet connection uses injected, Wallet
  Standard-compatible providers (Phantom, Solflare).
- **Chain:** Solana via a small JSON-RPC client (no heavyweight SDK on the
  server). Transaction status and transfer details are read from the chain using
  `getSignatureStatuses` and `getTransaction` (jsonParsed).

## Quick start (local development)

### Prerequisites

- Go 1.24+
- Node 20+
- Docker (for PostgreSQL) — or an existing PostgreSQL 14+

### 1. Database

```bash
docker compose up -d          # Postgres on localhost:5433
```

> The host port is `5433` to avoid clashing with a default 5432 install. Override
> with `POSTGRES_PORT=5432 docker compose up -d`.

### 2. Backend

```bash
cd backend
cp ../.env.example ../.env    # then edit values (see below)
go run ./cmd/api              # applies migrations and serves on :8080
```

Key `.env` values:

```dotenv
APP_ENV=development
APP_BASE_URL=http://localhost:5173
API_BASE_URL=http://localhost:8080
DATABASE_URL=postgres://openhandle:openhandle@localhost:5433/openhandle?sslmode=disable
SESSION_SECRET=replace-with-a-long-random-secret-at-least-32-chars
CORS_ALLOWED_ORIGINS=http://localhost:5173
SOLANA_CLUSTER=devnet
SOLANA_RPC_URL=https://api.devnet.solana.com
```

If port 8080 is taken, set `PORT` (e.g. `PORT=8090`) and point the frontend
`VITE_API_BASE_URL` at it. Health: `GET /healthz` · readiness: `GET /readyz`.

### 3. Frontend

```bash
cd frontend
npm install
cp .env.example .env.local    # VITE_API_BASE_URL=http://localhost:8080 (or your PORT)
npm run dev                   # http://localhost:5173
```

### Optional: demo seed (development only)

```bash
SEED_DEMO=true go run ./cmd/api
```

Creates `demo@example.com` / `demo-password-123` with handle `demo`. It creates
**no** wallets, balances, identities or transactions — nothing fabricated.

## Testing

```bash
# Backend unit tests (no database required)
cd backend && go test ./internal/validate/... ./internal/wallet/...

# Backend integration tests (require PostgreSQL)
TEST_DATABASE_URL="postgres://openhandle:openhandle@localhost:5433/openhandle?sslmode=disable" \
  go test ./...

# Frontend
cd frontend
npm run typecheck
npm run lint
npm run build
```

CI runs the same commands plus a production image build on every push — see
[.github/workflows/ci.yml](.github/workflows/ci.yml).

Integration tests **skip explicitly** when `TEST_DATABASE_URL` is unset — they
never silently pass. They truncate their tables, so point them at a test
database, not one you care about.

## Deploying

The client goes on Vercel (free tier) and the API on a single VM
(Oracle Cloud free tier works). Vercel proxies `/api/*` to the API, so the
browser only ever talks to one origin and the session cookie stays first-party —
which is what keeps sign-in working in Safari and Firefox. An optional
`ORIGIN_SECRET` lets the API reject anything that did not come through the proxy.

```bash
# on the server
sudo bash deploy/bootstrap.sh          # Docker, firewall, secrets, up

# from anywhere, once the client is deployed
bash deploy/verify.sh https://<project>.vercel.app --create-test-account
```

Full walkthrough, configuration reference and troubleshooting:
[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Feature status

| Area | Status | Notes |
| --- | --- | --- |
| Email/password auth | ✅ Working | bcrypt, server-side sessions, CSRF |
| Handle claim + availability | ✅ Working | DB-enforced uniqueness, race-safe |
| Public profile `/@handle` | ✅ Working | public fields only; private → 404 |
| Wallet ownership challenge | ✅ Working | single-use, domain-bound, ed25519 |
| Native SOL transfer | ✅ Working | signed in the user's wallet; verified on-chain |
| Transaction history | ✅ Working | OpenHandle-submitted transactions only |
| Account deletion | ✅ Working | data scrubbed, handle released |
| Social identity (GitHub OAuth) | ⚠️ Needs credentials | disabled → "Not configured" until configured |
| Production deployment | ✅ Scripted and verified | compose + Caddy + TLS, see docs/DEPLOYMENT.md |
| Edge-proxy client IP + no-store | ✅ Working | `CLIENT_IP_MODE`, `httpx.NoStore` |
| SPL token transfers | ❌ Not implemented | not shown in the UI |
| Balance display | ❌ Deliberately omitted | no indexer configured; not shown as zero |

## Known limitations

- **Social login is not enabled.** Email/password is the only auth. GitHub OAuth
  is implemented (`internal/identity/github.go`) but ships disabled until
  `GITHUB_CLIENT_ID`/`GITHUB_CLIENT_SECRET` are set. It has **not been exercised
  end-to-end** in this environment (no credentials), so treat it as unverified.
- **Handles are immutable.** There is no rename flow in this release.
- **History scope.** Only transactions submitted through OpenHandle appear.
  It is not a full view of a wallet's on-chain activity.
- **Balance is not shown**, by design, rather than showing an unverified zero.
- **Rate limiting is per process.** A multi-instance deployment needs a shared
  store (Redis, etc.). Behind an edge proxy, limits are only as trustworthy as
  the origin's reachability: set `ORIGIN_SECRET` (see
  [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)) so a direct caller cannot pick the
  `X-Forwarded-For` value its limit is keyed on.
- **The client and API must share an origin** for cookies to be first-party.
  The deployment does that with a Vercel rewrite. Pointing the client directly
  at a different domain needs `COOKIE_SAMESITE=none`, and Safari will then drop
  the session cookie entirely.
- **Devnet by default.** `SOLANA_CLUSTER` must be set to `mainnet-beta`
  explicitly; the UI labels the active cluster everywhere.
- The frontend bundle includes `@solana/web3.js` (~570 KB); route-level code
  splitting is a future improvement.

## Documentation

- [docs/API.md](docs/API.md) — endpoints, errors, rate limits
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) — deploying to Vercel + a VM, cookie/TLS behaviour, troubleshooting
- [SECURITY.md](SECURITY.md) — threat model and reporting
- [PRIVACY.md](PRIVACY.md) / [TERMS.md](TERMS.md) — drafts requiring legal review
- [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) — build checklist

## License

No license has been chosen yet. Add one before distributing.
