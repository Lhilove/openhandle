# OpenHandle API

Base URL: your deployment. All responses are JSON and none of them are
cacheable: every response carries `Cache-Control: no-store`, and the API
requires `Content-Type: application/json` bodies.

The API is versioned under `/api/v1`. In the recommended deployment the browser
reaches the API through the app's own origin — Vercel rewrites `/api/*` to the
API origin — so `https://<app>/api/v1/…` and `https://<api>/api/v1/…` are the
same surface. `/healthz` and `/readyz` sit outside `/api` and are not versioned.

Sessions are cookie-based (`oh_session`, `HttpOnly`, `SameSite=Lax`, no `Domain`
attribute, `Secure` in production). Mutating requests must include the session
CSRF token in the `X-CSRF-Token` header. The token is returned by
`GET /api/v1/me` (and by register/login).

If the operator sets `ORIGIN_SECRET`, every request must also carry it in the
`X-Origin-Secret` header (attached by the edge proxy). Requests without it are
rejected with `403 FORBIDDEN` before reaching any handler, except `/healthz`,
which stays open for the container's liveness probe.

## Error format

```json
{
  "error": {
    "code": "HANDLE_NOT_FOUND",
    "message": "No public handle was found."
  }
}
```

Stable codes include: `INVALID_REQUEST`, `UNAUTHORIZED`, `FORBIDDEN`,
`CSRF_FAILED`, `RATE_LIMITED`, `NOT_FOUND`, `HANDLE_NOT_FOUND`, `HANDLE_TAKEN`,
`HANDLE_INVALID`, `EMAIL_TAKEN`, `INVALID_CREDENTIALS`, `CONFLICT`,
`WALLET_INVALID`, `CHALLENGE_INVALID`, `TRANSACTION_INVALID`,
`PROVIDER_NOT_CONFIGURED`, `INTERNAL_ERROR`.

## Operations

| Method | Path | Auth | Notes |
| --- | --- | --- | --- |
| GET | `/healthz` | – | liveness |
| GET | `/readyz` | – | readiness incl. DB, reports RPC reachability |

## Authentication

| Method | Path | Auth | Body |
| --- | --- | --- | --- |
| POST | `/api/v1/auth/register` | – | `{ email, password }` → 201 + session |
| POST | `/api/v1/auth/login` | – | `{ email, password }` → 200 + session |
| POST | `/api/v1/auth/logout` | cookie | invalidates the session |
| GET | `/api/v1/me` | cookie | current user, handle, wallets, identities, `csrf_token` |

Passwords: 8–72 characters. Emails are lowercased and unique.

## Handles

| Method | Path | Auth | Notes |
| --- | --- | --- | --- |
| GET | `/api/v1/handles/availability?handle=love` | – | `{ handle, available, reason }` |
| GET | `/api/v1/handles/{handle}` | – | public resolution |
| POST | `/api/v1/handles` | cookie+CSRF | `{ handle }` → 201 |
| PATCH | `/api/v1/me/handle` | cookie+CSRF | returns 409 (immutable) |
| PATCH | `/api/v1/me/profile` | cookie+CSRF | `{ display_name?, bio?, is_public? }` |

Handle rules: lowercase ASCII, length 3–20, characters `a-z`, `0-9`, single
internal underscores, must start and end alphanumeric, reserved names blocked.

### Public resolution response

Only intentionally public fields are returned. Unknown and private handles both
return `404 HANDLE_NOT_FOUND`.

```json
{
  "handle": "love",
  "profile_url": "https://example.invalid/@love",
  "display_name": null,
  "bio": null,
  "network": "devnet",
  "wallets": [
    { "network": "solana", "address": "PUBLIC_WALLET_ADDRESS", "is_primary": true, "ownership_verified": true }
  ],
  "identities": []
}
```

> This is a schema illustration. `example.invalid` and `PUBLIC_WALLET_ADDRESS`
> are placeholders, not live data.

## Wallets

| Method | Path | Auth | Notes |
| --- | --- | --- | --- |
| GET | `/api/v1/me/wallets` | cookie | owner's wallets |
| POST | `/api/v1/wallet-challenges` | cookie+CSRF | `{ address, network? }` → `{ message, nonce, domain, network, expires_at }` |
| POST | `/api/v1/wallets/verify` | cookie+CSRF | `{ address, nonce, signature, network? }` → verified wallet |
| POST | `/api/v1/wallets` | cookie+CSRF | attach without verification (marked unverified) |
| DELETE | `/api/v1/me/wallets/{id}` | cookie+CSRF | remove (owner-scoped) |

Challenge messages are single-use, expire in 5 minutes, and are bound to the
domain, address and user. Signatures are verified server-side with ed25519.

## Transactions

| Method | Path | Auth | Notes |
| --- | --- | --- | --- |
| POST | `/api/v1/transactions` | cookie+CSRF | record a submitted transfer |
| GET | `/api/v1/me/transactions?limit=&offset=` | cookie | owner's history |
| GET | `/api/v1/transactions/{signature}` | cookie | owner-only |

Request body:

```json
{
  "network": "devnet",
  "signature": "BASE58_TRANSACTION_SIGNATURE",
  "recipient_handle": "love",
  "amount_base_units": "10000000",
  "asset": "SOL",
  "sender_address": "OPTIONAL_SENDER_PUBLIC_KEY"
}
```

The recipient is re-resolved from the handle; a client-supplied address is never
trusted. When the RPC can already see the transaction, it is verified against the
chain (recipient + lamports) and marked `confirmed`. Otherwise it is stored as
`submitted` and confirmed by a background worker. Ingest is idempotent on
`(network, signature)`. `signature` must be a real base58 64-byte signature as
returned by the wallet; anything else is rejected with `TRANSACTION_INVALID`,
because the status RPC fails an entire batch that contains one malformed value.

Statuses:

| Status | Meaning |
| --- | --- |
| `submitted` | accepted for tracking; the network has not confirmed it yet |
| `confirmed` | on-chain and matched against the recorded recipient and amount |
| `failed` | on-chain, but the transfer itself failed |
| `expired` | the network never accepted it — **no funds moved** |

`expired` is the one worth understanding. A Solana blockhash is only valid for
about 90 seconds, so a signature the chain still has not seen after a few minutes
was dropped before it landed: the sender's wallet still holds the funds and the
recipient got nothing. The worker decides this roughly three minutes after
submission (not an hour), because reporting a transfer that never happened as
"pending" misrepresents it. A dropped transaction is not deleted — the attempt
stays in the sender's history as `expired` — and it is still re-checked slowly for
a day, so a transaction that does land late is corrected to `confirmed`.

Because recording a dropped transaction as a payment would be a lie, the client
also confirms with the cluster directly after the wallet returns a signature and
does not register a payment at all when the blockhash expired.

**Scope:** history covers transactions submitted through OpenHandle only.

## Identities

| Method | Path | Auth | Notes |
| --- | --- | --- | --- |
| GET | `/api/v1/me/identities` | cookie | identities + provider configuration |
| GET | `/api/v1/identities/{provider}/start` | cookie | redirects into OAuth (browser) |
| POST | `/api/v1/identities/{provider}/connect` | cookie+CSRF | returns the authorize redirect |
| GET | `/api/v1/identities/{provider}/callback` | cookie | OAuth callback |
| DELETE | `/api/v1/identities/{provider}` | cookie+CSRF | disconnect |

Unconfigured providers return `501 PROVIDER_NOT_CONFIGURED` — never a fake
success.

## Account

| Method | Path | Auth | Notes |
| --- | --- | --- | --- |
| DELETE | `/api/v1/me` | cookie+CSRF | delete account; scrubs data, releases handle |

## Rate limits (per IP, per process)

| Route | Limit |
| --- | --- |
| register / login | 10 / min (burst 5) |
| handle availability | 60 / min |
| wallet challenges | 20 / min |
| transaction submissions | 30 / min |
| public resolution | 120 / min |

429 responses use `RATE_LIMITED`.
