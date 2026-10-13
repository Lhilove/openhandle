# Deployment guide

## The shape of the deployment

```
        browser
           |  https://<project>.vercel.app           (one origin, first-party cookies)
           v
   +-------------------+        rewrite /api/* and /healthz
   |  Vercel (edge)    | ------------------------------------------+
   |  static React app |                                           |
   +-------------------+                                           v
                                                       +----------------------+
                                                       |  Oracle Cloud VM     |
                                                       |  Caddy (TLS, ports   |
                                                       |   80/443)            |
                                                       |        |             |
                                                       |        v             |
                                                       |  api (Go, :8080)     |
                                                       |        |             |
                                                       |        v             |
                                                       |  postgres (private)  |
                                                       +----------------------+
```

The client is static assets, so it goes on Vercel's free tier. The API needs a
long-running process, a database and background workers, so it runs on a VM.

**Why the browser talks only to Vercel.** Vercel proxies `/api/*` to the API
origin (see `frontend/vercel.json`). The session cookie therefore belongs to the
Vercel host and is **first-party**. The alternative — pointing the client
straight at `https://api.…` — makes every API request cross-site, which needs
`SameSite=None` cookies that Safari blocks outright and Firefox partitions. The
proxy keeps sign-in working in every browser.

Two consequences of proxying are handled in this repository:

* **Edge caching.** Vercel honours the origin's cache headers on external
  rewrites. Every API response carries `Cache-Control: no-store` (`httpx.NoStore`)
  so one user's data can never be served to another, and `frontend/vercel.json`
  disables rewrite caching for `/api/*` as well.
* **Client IP.** Vercel overwrites `X-Forwarded-For` with the visitor's address,
  so the stack sets `CLIENT_IP_MODE=forwarded` and rate limiting keys on the real
  visitor instead of on the proxy. See "Locking the origin down" for how to make
  that spoof-proof.

## What you need

| Needed | Notes |
| --- | --- |
| GitHub account | the repository (this one) |
| Vercel account | free tier; the client and its free `*.vercel.app` domain |
| Oracle Cloud account | free tier ARM/x86 VM with a public IP, Ubuntu or Oracle Linux |
| Domain | optional. The API origin can use a free name (see below) |

## Step 1 — Push the repository

```bash
gh repo create <you>/openhandle --public --source . --push
# or: git remote add origin git@github.com:<you>/openhandle.git && git push -u origin main
```

## Step 2 — The server (one command)

SSH into the VM and run:

```bash
sudo apt-get update && sudo apt-get install -y git   # or: sudo dnf install -y git
git clone https://github.com/<you>/openhandle.git
cd openhandle
sudo bash deploy/bootstrap.sh
```

The script, in order:

1. verifies the host and detects the package manager and architecture;
2. installs Docker with the Compose v2 plugin (asking before it falls back to
   Docker's own installer);
3. opens TCP 80 and 443 in `ufw`, `firewalld` or the Oracle iptables rules and
   persists them;
4. detects the public IP, asks a few questions and defaults the API hostname;
5. generates `deploy/.env` with fresh `SESSION_SECRET` and database password
   (mode 600) and writes the API origin;
6. builds the image and starts Postgres + API + Caddy, then waits for the API to
   answer over HTTPS;
7. prints the Vercel steps.

Re-running it is safe: it keeps the existing secrets and database name, and only
rebuilds.

Preview everything without changing the host with
`sudo DRY_RUN=1 bash deploy/bootstrap.sh --yes`.

### The API hostname

The API origin needs a real hostname with a valid certificate, because the
browser fetches it over HTTPS. You do not have to buy a domain:

* **`sslip.io` (default, no signup).** The script proposes
  `openhandle-api-<dashed-ip>.sslip.io`, which resolves to your IP automatically.
* **DuckDNS (free, yours to keep).** Create `something.duckdns.org`, point it at
  the VM's IP, then run the bootstrap with `API_DOMAIN=something.duckdns.org`.
  Better if you intend to move the IP later.
* **Your own domain.** `API_DOMAIN=api.example.com` with an `A` record at your IP.

Caddy requests a Let's Encrypt certificate on first start, so **ports 80 and 443
must be open** — both in the instance firewall (the script does that) and in the
VCN Security List:

> Oracle Cloud console → Networking → Virtual Cloud Networks → your VCN →
> Security Lists → Default Security List → Add Ingress Rules:
> source `0.0.0.0/0`, TCP `80`, then repeat for `443`.

The instance firewall cannot do this part; if you skip it the only symptom is
that Caddy never obtains a certificate.

## Step 3 — The client (Vercel)

1. **Add New… → Project → Import** your repository.
2. **Root Directory: `frontend`** (that is where `vercel.json` lives). Vercel
   detects Vite, and `npm ci` / `npm run build` come from `vercel.json`.
3. Deploy. The build needs **no environment variables**: with
   `VITE_API_BASE_URL` unset, the client calls the API on its own origin and
   Vercel proxies `/api/*`.
4. **Point the proxy at your server.** In `frontend/vercel.json` replace the
   `__API_ORIGIN__` placeholder with your API hostname (no scheme):

   ```bash
   sed -i 's|__API_ORIGIN__|openhandle-api-203-0-113-7.sslip.io|' frontend/vercel.json
   git commit -am "Point the client at the API origin" && git push
   ```

   Until you do, `/api/*` requests fail: the placeholder is not a valid
   destination host, which is deliberate so the mistake cannot go unnoticed.

Vercel redeploys on push. Preview deployments use the same rules; if you want a
preview URL to reach the API, add that origin to `CORS_ALLOWED_ORIGINS` in
`deploy/.env` on the server and restart the API container.

## Step 4 — Verify

From your own machine:

```bash
bash deploy/verify.sh https://<project>.vercel.app
bash deploy/verify.sh https://<project>.vercel.app --create-test-account
```

The first run is read-only: liveness through the proxy, API routing, a rejected
login, and the `no-store` header. The second also registers a throwaway account,
proves the session cookie survives the proxy round trip (`/api/v1/me` with the
cookie), signs out, and deletes the account again.

## Cookies, TLS and proxies

| Setting | Default | Meaning |
| --- | --- | --- |
| `COOKIE_SAMESITE` | `lax` | correct for this topology, where the browser only talks to the Vercel origin |
| `COOKIE_SAMESITE=none` | — | only for a genuinely cross-site API; requires `https` base URLs and browsers still apply third-party cookie rules |
| `CLIENT_IP_MODE` | `remote` | use the socket peer, honouring `TRUSTED_PROXIES` |
| `CLIENT_IP_MODE=forwarded` | set by the bootstrap | take the left-most `X-Forwarded-For` value (what Vercel sets) |
| `TRUSTED_PROXIES` | empty | CIDRs whose `X-Forwarded-For` may be trusted in `remote` mode |
| `ORIGIN_SECRET` | empty | optional; see below |

The session cookie is always `HttpOnly`, host-only (no `Domain` attribute, which
`*.vercel.app` could not accept anyway) and marked `Secure` in production.

## Locking the origin down (optional)

With `CLIENT_IP_MODE=forwarded` the API believes the left-most
`X-Forwarded-For` value. Anyone who can reach the API directly can therefore pick
its own rate-limit bucket. That is not a privilege escalation — requests are
still authenticated — but it is weaker than it should be.

Vercel can attach a secret request header to every rewrite, which lets the
origin reject anything that did not come through Vercel:

1. Generate a value: `openssl rand -hex 32`.
2. Add it as `ORIGIN_SECRET` in the Vercel project environment (Production *and*
   Preview) and as `ORIGIN_SECRET` in `deploy/.env` on the server.
3. Vercel's request-header transforms require the legacy `routes` format, which
   cannot be combined with `rewrites`/`headers`. Replace the whole of
   `frontend/vercel.json` with:

   ```json
   {
     "$schema": "https://openapi.vercel.sh/vercel.json",
     "routes": [
       {
         "src": "/api/(.*)",
         "dest": "https://__API_ORIGIN__/api/$1",
         "transforms": [
           {
             "type": "request.headers",
             "op": "set",
             "target": { "key": "x-origin-secret" },
             "args": "$ORIGIN_SECRET",
             "env": ["ORIGIN_SECRET"]
           }
         ]
       },
       { "handle": "filesystem" },
       { "src": "/(.*)", "dest": "/index.html" }
     ]
   }
   ```

4. Restart the API (`docker compose … up -d api`). Requests without the header
   now get `403`; `/healthz` stays open so the container probe works.

The API side of this is implemented and covered by tests (`ORIGIN_SECRET` +
`CLIENT_IP_MODE=forwarded`, config validation refuses mismatched combinations).
**The Vercel half follows Vercel's documented `routes`/`transforms` format but
has not been deployed in this environment — validate it on a preview deployment
before relying on it.**

## Configuration reference

Server (`deploy/.env`, see `deploy/.env.prod.example`):

| Variable | Required | Notes |
| --- | --- | --- |
| `APP_ENV` | yes | set to `production` by the compose file |
| `APP_BASE_URL` | yes | the Vercel URL; used in profile and redirect URLs |
| `API_BASE_URL` | yes | `https://$API_DOMAIN` |
| `DATABASE_URL` | yes | assembled by the compose file for the private network |
| `SESSION_SECRET` | yes | ≥32 chars; rotating it signs everyone out |
| `CORS_ALLOWED_ORIGINS` | yes | exact origins, comma separated, never `*` |
| `SOLANA_CLUSTER` | yes | `devnet` \| `testnet` \| `mainnet-beta` |
| `SOLANA_RPC_URL` | yes | public endpoints are rate-limited; use a provider for real traffic |
| `CLIENT_IP_MODE` | recommended | `forwarded` behind the Vercel proxy |
| `ORIGIN_SECRET` | optional | see above; requires `forwarded` |
| `GITHUB_CLIENT_ID/SECRET` | optional | enables GitHub identity; callback is `<Vercel URL>/api/v1/identities/github/callback` |
| `LOG_LEVEL` | optional | `debug` \| `info` \| `warn` \| `error` |
| `HTTP_PORT` / `HTTPS_PORT` | optional | only to smoke-test on a host where 80/443 are taken |

Frontend (build-time, public — never secrets): `VITE_API_BASE_URL`,
`VITE_SOLANA_CLUSTER`, `VITE_SOLANA_RPC_URL`. Leave `VITE_API_BASE_URL` unset to
use the same-origin proxy; set it only if you deliberately go cross-site. If you
change `VITE_SOLANA_RPC_URL`, add that origin to the `connect-src` list in the
CSP in `frontend/vercel.json`.

## Operating the stack

```bash
cd /opt/openhandle                                   # or wherever you cloned it
# deploy/.env is root-only (it holds the secrets), so reading it -- and therefore
# every command below -- needs sudo. bootstrap.sh also puts your user in the
# docker group, so `docker ps` works unprivileged once you log in again.
COMPOSE="sudo docker compose -f deploy/docker-compose.prod.yml -p openhandle --env-file deploy/.env"

$COMPOSE ps                       # container and health status
$COMPOSE logs -f api              # structured JSON logs, request_id on every line
$COMPOSE restart api              # e.g. after editing deploy/.env
$COMPOSE up -d --build            # after git pull
$COMPOSE exec db psql -U openhandle -d openhandle
```

### A payment that never confirms

`GET /api/v1/me/transactions` reports each payment as `submitted`, `confirmed`,
`failed` or `expired`. `expired` means the network never accepted the
signature — the sender's wallet still holds the funds and the recipient received
nothing. To check a specific one against the chain:

```bash
$COMPOSE exec db psql -U openhandle -d openhandle -c \
  "SELECT signature, status, amount_base_units, submitted_at FROM transactions
    WHERE status <> 'confirmed' ORDER BY submitted_at DESC LIMIT 10"

# Is that signature on-chain at all?
curl -s https://api.devnet.solana.com -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"getSignatureStatuses","params":[["SIGNATURE"],{"searchTransactionHistory":true}]}'
```

A `null` entry means it never landed: the sender should send again (a fresh
transaction is built every attempt, with a new blockhash). If the API says
`submitted` for a signature `getTransaction` can see, the worker has not caught
up yet — it re-checks every 15 seconds and corrects the record.

* **Update:** `git pull && sudo bash deploy/bootstrap.sh --yes`, or the
  `$COMPOSE up -d --build` above.
* **Backups:** `sudo bash deploy/backup.sh` writes a compressed dump to
  `/var/backups/openhandle` and prunes old ones; it reads `deploy/.env` and runs
  `docker exec`, so schedule it as root (see the header of that file for a cron
  line). Copy dumps off the machine — a backup on the same disk is not a backup.
* **Rollback:** `git checkout <previous-tag>` then `up -d --build`. Database
  migrations are forward-only; restore from a dump if you must go back.
* **Certificates:** Caddy renews automatically. `$COMPOSE logs caddy` shows the
  ACME activity.

## Pre-launch checklist

- [ ] Real legal review of [TERMS.md](../TERMS.md) and [PRIVACY.md](../PRIVACY.md)
- [ ] `SESSION_SECRET` and the database password generated on the server, mode 600
- [ ] `deploy/verify.sh <app-origin> --create-test-account` passes
- [ ] Automated backups configured **and a restore actually rehearsed**
- [ ] Repository license chosen before distributing
- [ ] Mainnet RPC provider chosen, funded and monitored (before real funds)
- [ ] GitHub OAuth app created, callback registered, flow tested end to end
- [ ] Dependency scanning enabled in CI (the workflow builds and tests; add a
      scanner for the supply chain)
- [ ] Security contact address and disclosure process published

## External accounts / credentials needed

Features that cannot be exercised without credentials:

1. **GitHub OAuth** — a GitHub OAuth App (`GITHUB_CLIENT_ID`,
   `GITHUB_CLIENT_SECRET`), callback `<APP_BASE_URL>/api/v1/identities/github/callback`.
2. **Reddit** — declared in config, not implemented in this release.
3. **Production Solana RPC** — a provider endpoint and key.
4. **Your own domain** — optional; a free `sslip.io`/DuckDNS name is enough to run.

## What was verified, and what was not

Verified locally by running this repository's production compose stack on a
development machine, and by serving the built client through a local stand-in
for the Vercel edge that applies `frontend/vercel.json` verbatim:

* the image builds and starts as a non-root user; migrations apply; `db` and
  `api` report healthy;
* TLS termination through Caddy, `/healthz` and a real API call served over
  HTTPS on port 443;
* `db` and `api` publish **no** host ports — only Caddy is reachable;
* `ORIGIN_SECRET` returns 403 without the header, 200 with it, and leaves
  `/healthz` open for the container probe;
* the session cookie is `HttpOnly; Secure; SameSite=Lax`, host-only, and a
  register → `/me` → delete round trip works through the proxy;
* `Cache-Control: no-store` on API responses;
* the built client under the real CSP: the SPA fallback serves deep links such
  as `/@handle` on a fresh load, no CSP violation is reported, and a sign-up in
  a real browser through the proxy is still signed in after a full page reload
  (the cookie is first-party).

Not verified here: the Vercel half (the project was not deployable from this
environment) and a **public** Let's Encrypt certificate, since the local run used
Caddy's internal CA. Both are exercised by the steps in Step 3 and Step 4 on the
first real deployment.
