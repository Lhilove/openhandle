import type { ReactNode } from "react";
import { API_BASE, SOLANA_CLUSTER } from "../lib/api";
import { Alert, Badge } from "../components/ui";

function Code({ children }: { children: string }) {
  return (
    <pre className="overflow-x-auto rounded-control border border-line bg-bg p-4 text-xs leading-relaxed">
      <code className="font-mono">{children}</code>
    </pre>
  );
}

function Endpoint({
  method,
  path,
  children,
}: {
  method: string;
  path: string;
  children: ReactNode;
}) {
  return (
    <div className="border-t border-line py-5">
      <div className="flex flex-wrap items-center gap-2">
        <Badge>{method}</Badge>
        <code className="font-mono text-sm">{path}</code>
      </div>
      <div className="mt-3 space-y-3 text-sm muted">{children}</div>
    </div>
  );
}

export default function Docs() {
  return (
    <div className="mx-auto max-w-3xl px-4 py-12 sm:px-6">
      <h1 className="h1">Developer documentation</h1>
      <p className="mt-3 text-base muted">
        OpenHandle exposes a versioned HTTP API. All responses are JSON; errors share one envelope.
      </p>

      <Alert kind="info">
        Base URL in this deployment: <code className="font-mono">{API_BASE}</code>. Network:{" "}
        <strong>{SOLANA_CLUSTER}</strong>. These are example values — replace them with your
        configured deployment URL.
      </Alert>

      <section className="mt-8">
        <h2 className="h2 text-xl">Resolve a handle</h2>
        <Endpoint method="GET" path="/api/v1/handles/{handle}">
          <p>
            Public. Returns only fields the owner has made public. Private or unknown handles both
            return <code className="font-mono">404 HANDLE_NOT_FOUND</code> so private handles are not
            discoverable.
          </p>
          <p>Illustrative response schema (not real data):</p>
          <Code>{`{
  "handle": "love",
  "profile_url": "${API_BASE.replace(/:\d+$/, ":5173")}/@love",
  "display_name": null,
  "bio": null,
  "network": "${SOLANA_CLUSTER}",
  "wallets": [
    {
      "network": "solana",
      "address": "PUBLIC_WALLET_ADDRESS",
      "is_primary": true,
      "ownership_verified": true
    }
  ],
  "identities": []
}`}</Code>
        </Endpoint>
        <Endpoint method="GET" path="/api/v1/handles/availability?handle={handle}">
          <p>
            Public and rate-limited. Returns{" "}
            <code className="font-mono">{`{ "handle", "available", "reason" }`}</code>.
          </p>
        </Endpoint>
      </section>

      <section className="mt-8">
        <h2 className="h2 text-xl">Error format</h2>
        <p className="mt-2 text-sm muted">Every error uses this envelope.</p>
        <Code>{`{
  "error": {
    "code": "HANDLE_NOT_FOUND",
    "message": "No public handle was found."
  }
}`}</Code>
      </section>

      <section className="mt-8">
        <h2 className="h2 text-xl">Authenticated endpoints</h2>
        <p className="mt-2 text-sm muted">
          Sessions use an HttpOnly cookie. Mutating requests require the session CSRF token in the{" "}
          <code className="font-mono">X-CSRF-Token</code> header (returned by{" "}
          <code className="font-mono">GET /api/v1/me</code>).
        </p>
        <Endpoint method="POST" path="/api/v1/auth/register | /login | /logout">
          <p>Email and password accounts. Passwords are hashed with bcrypt.</p>
        </Endpoint>
        <Endpoint method="GET" path="/api/v1/me">
          <p>Current user, handle, wallets, identities and the CSRF token.</p>
        </Endpoint>
        <Endpoint method="POST" path="/api/v1/handles">
          <p>Claim a handle. Uniqueness is enforced by a database constraint.</p>
        </Endpoint>
        <Endpoint method="POST" path="/api/v1/wallet-challenges">
          <p>Create a single-use, short-lived, domain-bound message for a wallet to sign.</p>
        </Endpoint>
        <Endpoint method="POST" path="/api/v1/wallets/verify">
          <p>
            Verify the signature (ed25519) server-side and link the wallet as ownership-verified.
          </p>
        </Endpoint>
        <Endpoint method="POST" path="/api/v1/transactions">
          <p>
            Record a submitted transfer. The recipient is re-resolved from the handle and the
            transaction is verified against the chain when visible. Duplicate signatures are
            idempotent.
          </p>
        </Endpoint>
        <Endpoint method="GET" path="/api/v1/me/transactions?limit=&offset=">
          <p>Paginated history of transactions submitted through OpenHandle only.</p>
        </Endpoint>
      </section>

      <section className="mt-8" id="security">
        <h2 className="h2 text-xl">Rate limits</h2>
        <ul className="mt-2 list-disc space-y-1 pl-5 text-sm muted">
          <li>Login / register: 10 per minute per IP (burst 5)</li>
          <li>Handle availability: 60 per minute per IP</li>
          <li>Wallet challenges: 20 per minute per IP</li>
          <li>Transaction submissions: 30 per minute per IP</li>
          <li>Public resolution: 120 per minute per IP</li>
        </ul>
        <p className="mt-3 text-sm muted">
          Limits are enforced per process. A multi-instance deployment should back them with a shared
          store.
        </p>
      </section>

      <section className="mt-8">
        <h2 className="h2 text-xl">History scope</h2>
        <p className="mt-2 text-sm muted">
          The history endpoint lists only transactions submitted through OpenHandle. It is not an
          exhaustive view of a wallet's on-chain activity.
        </p>
      </section>
    </div>
  );
}
