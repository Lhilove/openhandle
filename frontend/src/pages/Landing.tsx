import { Link } from "react-router-dom";
import { Badge } from "../components/ui";
import { SOLANA_CLUSTER } from "../lib/api";

function ExampleProfileCard() {
  return (
    <div className="card w-full max-w-sm p-5" aria-label="Illustrative example profile">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium uppercase tracking-wide muted">Example profile</span>
        <Badge>Illustrative</Badge>
      </div>
      <div className="mt-4 flex items-center gap-3">
        <span
          aria-hidden="true"
          className="grid h-11 w-11 place-items-center rounded-full bg-accent/10 text-base font-semibold text-accent"
        >
          @
        </span>
        <div>
          <div className="text-lg font-semibold">@love</div>
          <div className="text-sm muted">Ada — building in public</div>
        </div>
      </div>
      <div className="mt-4 space-y-2 rounded-control border border-line bg-bg p-3 font-mono text-xs">
        <div className="flex justify-between gap-3">
          <span className="muted">Network</span>
          <span>solana · {SOLANA_CLUSTER}</span>
        </div>
        <div className="flex justify-between gap-3">
          <span className="muted">Wallet</span>
          <span>7xKX…9Qm2</span>
        </div>
        <div className="flex justify-between gap-3">
          <span className="muted">Asset</span>
          <span>SOL</span>
        </div>
      </div>
      <p className="mt-3 text-xs muted">
        Sample data only. Real profiles show only wallets and verified accounts the owner has chosen
        to make public.
      </p>
    </div>
  );
}

const steps = [
  {
    n: "1",
    title: "Claim",
    body: "Choose a human-readable handle. It is unique, enforced by the database, and yours to share.",
  },
  {
    n: "2",
    title: "Connect",
    body: "Link a wallet you already control. You sign a message to prove ownership — OpenHandle never touches your keys.",
  },
  {
    n: "3",
    title: "Receive",
    body: "Share @yourhandle. Senders open your profile, see your address, and pay from their own wallet.",
  },
];

const faqs = [
  {
    q: "Does OpenHandle take a fee?",
    a: "No application fee is charged in this release. Blockchain network fees still apply and are paid by the sender's wallet. We do not claim that payments are free.",
  },
  {
    q: "Do you hold my money or keys?",
    a: "No. OpenHandle never generates, collects, or stores private keys or seed phrases, and it is not a custodian. It resolves a handle to a wallet address you control.",
  },
  {
    q: "What does a verified social account prove?",
    a: "Only that you control that account, confirmed through the provider's official API. It is not government identity verification, and we never show a badge for a username you simply typed in.",
  },
  {
    q: "Can I change or lose my handle?",
    a: "In this release handles are immutable. Your handle stays yours; if you delete your account the handle can be released.",
  },
  {
    q: "Are blockchain payments reversible?",
    a: "Generally no. Once a transaction is confirmed on-chain it cannot be undone. Always confirm the recipient, network, asset and amount before signing.",
  },
];

export default function Landing() {
  return (
    <div>
      {/* Hero */}
      <section className="mx-auto grid max-w-6xl gap-10 px-4 py-12 sm:px-6 sm:py-16 lg:grid-cols-2 lg:items-center">
        <div>
          <Badge>Open source · {SOLANA_CLUSTER}</Badge>
          <h1 className="h1 mt-4">Your identity. Your money. Your handle.</h1>
          <p className="mt-4 max-w-prose text-base text-muted">
            OpenHandle maps a handle you choose to a self-custody wallet you already own. Share one
            readable name, and anyone can send you crypto without copying a long address.
          </p>
          <div className="mt-6 flex flex-wrap gap-3">
            <Link to="/claim" className="btn-primary">
              Claim your handle
            </Link>
            <a href="#how" className="btn-secondary">
              How it works
            </a>
          </div>
          <p className="mt-4 text-xs muted">
            Self-custody by design. No seed phrases. No custody. Network fees may apply and are paid
            by the sender's wallet.
          </p>
        </div>
        <div className="flex justify-center lg:justify-end">
          <ExampleProfileCard />
        </div>
      </section>

      {/* Steps */}
      <section id="how" className="border-y border-line bg-surface">
        <div className="mx-auto max-w-6xl px-4 py-14 sm:px-6">
          <h2 className="h2">Claim, connect, receive</h2>
          <p className="mt-2 max-w-prose text-sm muted">
            Three steps. No blockchain expertise required.
          </p>
          <div className="mt-8 grid gap-5 md:grid-cols-3">
            {steps.map((s) => (
              <div key={s.n} className="card p-5">
                <span
                  aria-hidden="true"
                  className="grid h-8 w-8 place-items-center rounded-full bg-accent/10 text-sm font-semibold text-accent"
                >
                  {s.n}
                </span>
                <h3 className="mt-3 text-base font-semibold">{s.title}</h3>
                <p className="mt-1.5 text-sm muted">{s.body}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Security */}
      <section id="security" className="mx-auto max-w-6xl px-4 py-14 sm:px-6">
        <h2 className="h2">Security and honesty</h2>
        <div className="mt-6 grid gap-5 md:grid-cols-2">
          <div className="card p-5">
            <h3 className="text-base font-semibold">Your keys never leave your wallet</h3>
            <p className="mt-1.5 text-sm muted">
              OpenHandle is not a wallet and not a custodian. We store a public address only. Wallet
              ownership is confirmed by having your wallet sign a short, single-use message that the
              server verifies cryptographically.
            </p>
          </div>
          <div className="card p-5">
            <h3 className="text-base font-semibold">What verification does and does not mean</h3>
            <p className="mt-1.5 text-sm muted">
              &ldquo;Wallet verified&rdquo; means a signature proved control of that address.
              &ldquo;Identity verified&rdquo; means a provider's official API confirmed control of a
              social account. Neither is legal or government identity verification.
            </p>
          </div>
          <div className="card p-5">
            <h3 className="text-base font-semibold">Real status, never simulated</h3>
            <p className="mt-1.5 text-sm muted">
              Transaction status comes from the Solana network. We never mark a payment confirmed
              because a browser said so, and we never invent balances or history.
            </p>
          </div>
          <div className="card p-5">
            <h3 className="text-base font-semibold">Irreversible by nature</h3>
            <p className="mt-1.5 text-sm muted">
              On-chain transfers are generally final. We show the recipient, network, asset and
              amount before you sign, and warn that payments cannot be reversed.
            </p>
          </div>
        </div>
      </section>

      {/* Supported networks */}
      <section className="border-y border-line bg-surface">
        <div className="mx-auto max-w-6xl px-4 py-14 sm:px-6">
          <h2 className="h2">Supported networks</h2>
          <p className="mt-2 max-w-prose text-sm muted">
            We list only what is actually implemented. Nothing here is aspirational.
          </p>
          <div className="mt-6 grid gap-4 sm:grid-cols-2">
            <div className="card p-5">
              <div className="flex items-center justify-between">
                <h3 className="text-base font-semibold">Solana — native SOL</h3>
                <Badge kind="success">Implemented</Badge>
              </div>
              <p className="mt-2 text-sm muted">
                Handle resolution, wallet ownership verification and native SOL transfers.
                Configured cluster: <span className="font-medium text-ink">{SOLANA_CLUSTER}</span>.
              </p>
            </div>
            <div className="card p-5">
              <div className="flex items-center justify-between">
                <h3 className="text-base font-semibold">SPL tokens (e.g. USDC)</h3>
                <Badge kind="warning">Not yet</Badge>
              </div>
              <p className="mt-2 text-sm muted">
                Token transfers are not enabled in this release. We will not display a token option
                that cannot be sent safely.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* Developers */}
      <section className="mx-auto max-w-6xl px-4 py-14 sm:px-6">
        <div className="card flex flex-col gap-4 p-6 md:flex-row md:items-center md:justify-between">
          <div>
            <h2 className="text-xl font-semibold">Open source and developer-friendly</h2>
            <p className="mt-1.5 max-w-prose text-sm muted">
              Resolve any handle to its public wallet with one documented, versioned endpoint.
              Self-host the whole stack.
            </p>
          </div>
          <div className="flex gap-3">
            <Link to="/docs" className="btn-primary">
              Read the API docs
            </Link>
            <a
              href="https://github.com/"
              target="_blank"
              rel="noreferrer noopener"
              className="btn-secondary"
            >
              View on GitHub
            </a>
          </div>
        </div>
      </section>

      {/* FAQ */}
      <section className="border-t border-line bg-surface">
        <div className="mx-auto max-w-3xl px-4 py-14 sm:px-6">
          <h2 className="h2">Questions</h2>
          <div className="mt-6 divide-y divide-line">
            {faqs.map((f) => (
              <details key={f.q} className="group py-4">
                <summary className="cursor-pointer list-none text-sm font-semibold marker:content-none">
                  <span className="mr-2 text-accent" aria-hidden="true">
                    +
                  </span>
                  {f.q}
                </summary>
                <p className="mt-2 pl-5 text-sm muted">{f.a}</p>
              </details>
            ))}
          </div>
        </div>
      </section>
    </div>
  );
}
