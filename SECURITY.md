# Security

OpenHandle is financial-adjacent software. We treat security as a first-class
requirement and prefer a smaller, genuinely safe surface over broad, unverified
features.

## Reporting a vulnerability

Do not open a public issue for a security problem. Report privately to the
project maintainers (replace this address with a real one before launch):
`security@example.invalid`.

Please include: a description, reproduction steps, affected version/commit, and
impact. We aim to acknowledge within 72 hours. No bug-bounty program exists yet.

## Design guarantees

- OpenHandle never generates, requests, collects, transmits or stores private
  keys or seed phrases.
- The backend never signs a transaction on a user's behalf and never holds or
  escrows funds.
- A public address is stored; that is all.
- Wallet ownership is only marked "verified" after the server cryptographically
  verifies a signed challenge with `crypto/ed25519`.
- Transaction status is only "confirmed" when the configured Solana RPC reports
  the transaction confirmed **and** the parsed on-chain transfer matches the
  recorded recipient and amount. A frontend "success" flag is never trusted.
- Social accounts are only marked verified after the provider's official API
  confirms control. Usernames typed by users are never treated as proof.

## Controls implemented

- Passwords hashed with bcrypt; login responses are timing-equalised for unknown
  emails to resist user enumeration.
- Sessions: server-side records; only a SHA-256 hash of the cookie token is
  stored. Cookies are `HttpOnly`, `SameSite=Lax`, and `Secure` in production.
- CSRF: mutating requests require the per-session token in `X-CSRF-Token`.
- Object-level authorization on every user-owned resource (wallets, transactions,
  profile edits) via owner-scoped queries.
- Parameterized SQL only; no string-built queries.
- Server-side validation mirroring the client, plus database constraints for
  uniqueness and one-primary-wallet rules.
- Rate limiting on register/login, handle availability, wallet challenges,
  transaction submission and public resolution.
- Security headers (`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`,
  a restrictive CSP, `Permissions-Policy`, HSTS in production).
- CORS uses an explicit origin allow-list; wildcard origins are rejected at
  startup and never combined with credentials.
- Client IPs come from `RemoteAddr` unless `TRUSTED_PROXIES` is configured, so a
  spoofed `X-Forwarded-For` cannot bypass rate limits by default.
- Wallet challenges are single-use, short-lived (5 min) and bound to the domain,
  wallet address and user; consumption is atomic to prevent replay.
- OAuth `state` is HMAC-signed and compared in constant time; least-privilege
  scopes are requested and access tokens are not retained.
- Errors returned to clients are generic; internal errors and stack traces are
  logged server-side only, and secrets are never logged.

## Threat model

1. **Handle squatting and impersonation.** Handles are first-come-first-served
   and immutable; a database constraint prevents duplicates under concurrency.
   A handle is not proof of identity, and a handle that happens to match a
   username elsewhere does not imply ownership of that account. Reserved
   system names are blocked. Reported impersonation is a manual moderation
   action (no automated reporting flow yet).
2. **Account takeover.** Mitigated by bcrypt, server-side sessions, CSRF tokens,
   HttpOnly cookies, and login rate limiting. Not mitigated by MFA (not
   implemented) or email verification (not implemented) — both are gaps.
3. **Handle reassignment and stale payment links.** Handles are immutable in
   this release, so reassignment cannot occur while an account is active.
   Deletion releases a handle; a released handle could later be claimed by
   someone else, so old links could point to a different owner. The Send flow
   re-resolves the recipient immediately before building a transaction and shows
   the full destination address, and aborts if the address changed since review.
4. **Wallet-signature replay.** Challenges are single-use (atomic consume),
   expire after 5 minutes, and are bound to domain, address and user. Signature
   verification is delegated to `crypto/ed25519`.
5. **Fake or spoofed payment confirmation.** Status comes from the RPC, and
   confirmation re-checks that the on-chain transfer matches recipient and
   amount. The server ignores any client claim of success.
6. **Malicious frontend or compromised dependency.** CSP is restrictive; secrets
   live only on the server; the API re-validates and re-authorizes everything,
   so a hostile client cannot escalate. Dependency hygiene is manual — no
   automated scanning pipeline yet.
7. **RPC/indexer outage or stale data.** The RPC client is isolated behind a
   small interface. RPC failures leave records pending rather than fabricating
   success; status transitions are conservative (a confirmed transaction is not
   moved to failed, and vice versa).
8. **OAuth token compromise.** The GitHub flow discards tokens after fetching the
   profile; no tokens are stored. `state` is signed and validated.
9. **Spam, enumeration, denial of service.** IP rate limits on sensitive routes;
   public resolution returns 404 for both unknown and private handles so private
   handles cannot be enumerated. Per-process limits are a weakness for
   multi-instance deployments.
10. **Privacy leakage from public wallet/social links.** Public resolution
    returns only intentionally public fields and never internal ids or email.
    Public wallet addresses and on-chain history are inherently public.

## Residual risks

- No email verification, MFA, or account-recovery flow.
- No automated dependency scanning or SAST in CI.
- Rate limiting and challenge storage are in-process.
- GitHub OAuth is implemented but unverified in this environment.
- Deleted-account tombstones retain a scrubbed row for referential integrity;
  the email is anonymised but a row remains.

## Operational notes

- Rotate `SESSION_SECRET` carefully — rotating it invalidates all sessions.
- Take regular database backups and store them encrypted; document and test
  restore procedures.
- Keep dependencies updated and monitor RPC provider status and quotas.
