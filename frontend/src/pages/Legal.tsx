import { Alert } from "../components/ui";

export default function Legal({ kind }: { kind: "terms" | "privacy" }) {
  const isTerms = kind === "terms";
  return (
    <div className="mx-auto max-w-3xl px-4 py-12 sm:px-6">
      <Alert kind="warning">
        <strong>Draft document.</strong> This is a plain-language draft for a pre-launch product. It
        has not been reviewed by a lawyer and is not legal advice. Consult qualified professionals
        before any public launch.
      </Alert>

      <h1 className="h1 mt-6">{isTerms ? "Terms of Use (draft)" : "Privacy Policy (draft)"}</h1>
      <p className="mt-3 text-sm muted">Last updated: draft, not yet in force.</p>

      {isTerms ? (
        <div className="mt-6 space-y-5 text-sm leading-relaxed">
          <section>
            <h2 className="text-base font-semibold">What OpenHandle is</h2>
            <p className="mt-1 muted">
              OpenHandle maps a handle to a public wallet address. It is not a wallet, a custodian, a
              money transmitter, an exchange, or a broker. It never holds funds and never handles
              private keys or seed phrases.
            </p>
          </section>
          <section>
            <h2 className="text-base font-semibold">Your responsibilities</h2>
            <p className="mt-1 muted">
              You are responsible for your account, your wallet, and any transaction you sign.
              Blockchain transactions are generally irreversible. Always confirm the recipient,
              network, asset and amount before signing. Network fees may apply and are paid by the
              sender's wallet.
            </p>
          </section>
          <section>
            <h2 className="text-base font-semibold">Handles</h2>
            <p className="mt-1 muted">
              Handles are unique and immutable in this release. We may reclaim handles that violate
              these terms or impersonate others. A handle is not a trademark and carries no claim of
              trademark clearance.
            </p>
          </section>
          <section>
            <h2 className="text-base font-semibold">No guarantees</h2>
            <p className="mt-1 muted">
              The service is provided as-is, without warranties. We do not guarantee uptime, that
              payments are free, or that any network or RPC provider remains available.
            </p>
          </section>
          <section>
            <h2 className="text-base font-semibold">Not affiliated</h2>
            <p className="mt-1 muted">
              OpenHandle is not affiliated with, endorsed by, or sponsored by any wallet provider,
              social network, or blockchain foundation, including but not limited to Phantom,
              Solflare, GitHub, Reddit, Discord, or the Solana Foundation.
            </p>
          </section>
        </div>
      ) : (
        <div className="mt-6 space-y-5 text-sm leading-relaxed">
          <section>
            <h2 className="text-base font-semibold">What we store</h2>
            <p className="mt-1 muted">
              An email address and a password hash; your handle and any display name or bio you
              provide; public wallet addresses you connect; and records of transactions you submit
              through OpenHandle. We do not store private keys, seed phrases, or social provider
              access tokens.
            </p>
          </section>
          <section>
            <h2 className="text-base font-semibold">What becomes public</h2>
            <p className="mt-1 muted">
              If your profile is public, your handle, display name, bio, connected public wallet
              addresses, and any verified social accounts are visible to anyone. Public wallet
              addresses and on-chain transactions are inherently public on the blockchain and are
              outside our control.
            </p>
          </section>
          <section>
            <h2 className="text-base font-semibold">What we never do</h2>
            <p className="mt-1 muted">
              We never collect seed phrases or private keys. We never scrape private profile data. We
              never show a verification badge without a real provider verification.
            </p>
          </section>
          <section>
            <h2 className="text-base font-semibold">Your controls</h2>
            <p className="mt-1 muted">
              You can make your profile private, disconnect wallets, disconnect social accounts, and
              request account deletion. Deletion removes your account data, but on-chain records are
              permanent and cannot be deleted by us.
            </p>
          </section>
          <section>
            <h2 className="text-base font-semibold">No anonymity claim</h2>
            <p className="mt-1 muted">
              This service is not advertised as anonymous, fully decentralized, or compliant with
              every jurisdiction. Regulatory obligations may apply in your jurisdiction; review them
              with a professional.
            </p>
          </section>
        </div>
      )}
    </div>
  );
}
