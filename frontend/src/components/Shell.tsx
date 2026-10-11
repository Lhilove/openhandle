import { useState, type ReactNode } from "react";
import { Link, NavLink, useNavigate } from "react-router-dom";
import { useAuth } from "../lib/auth";
import { useWallet } from "../lib/wallet";
import { shortenAddress } from "../lib/format";
import { SOLANA_CLUSTER } from "../lib/api";

export function Wordmark() {
  return (
    <Link to="/" className="flex items-center gap-2 text-ink" aria-label="OpenHandle home">
      <span
        aria-hidden="true"
        className="grid h-7 w-7 place-items-center rounded-lg bg-accent text-sm font-bold text-accent-ink"
      >
        @
      </span>
      <span className="text-base font-semibold tracking-tight">OpenHandle</span>
    </Link>
  );
}

function ClusterChip() {
  const isMainnet = SOLANA_CLUSTER === "mainnet-beta";
  return (
    <span
      className={isMainnet ? "badge-warning" : "badge"}
      title={isMainnet ? "Live network. Real funds." : "Test network. No real funds."}
    >
      {isMainnet ? "Mainnet" : SOLANA_CLUSTER}
    </span>
  );
}

function MarketingNav() {
  const [open, setOpen] = useState(false);
  const { me } = useAuth();
  return (
    <header className="sticky top-0 z-20 border-b border-line bg-bg/90 backdrop-blur">
      <nav className="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-3 sm:px-6">
        <div className="flex items-center gap-3">
          <Wordmark />
          <ClusterChip />
        </div>
        <div className="hidden items-center gap-6 md:flex">
          <Link to="/#how" className="text-sm font-medium text-muted hover:text-ink">
            How it works
          </Link>
          <Link to="/#security" className="text-sm font-medium text-muted hover:text-ink">
            Security
          </Link>
          <Link to="/docs" className="text-sm font-medium text-muted hover:text-ink">
            Developers
          </Link>
          {me ? (
            <Link to="/app" className="btn-primary">
              Open app
            </Link>
          ) : (
            <>
              <Link to="/signin" className="text-sm font-medium text-muted hover:text-ink">
                Sign in
              </Link>
              <Link to="/claim" className="btn-primary">
                Claim your handle
              </Link>
            </>
          )}
        </div>
        <button
          type="button"
          className="btn-secondary md:hidden"
          aria-expanded={open}
          aria-controls="mobile-menu"
          onClick={() => setOpen((v) => !v)}
        >
          {open ? "Close" : "Menu"}
        </button>
      </nav>
      {open ? (
        <div id="mobile-menu" className="border-t border-line bg-surface md:hidden">
          <div className="mx-auto flex max-w-6xl flex-col gap-1 px-4 py-3">
            <Link to="/#how" onClick={() => setOpen(false)} className="rounded-control px-3 py-2.5 text-sm font-medium hover:bg-bg">
              How it works
            </Link>
            <Link to="/#security" onClick={() => setOpen(false)} className="rounded-control px-3 py-2.5 text-sm font-medium hover:bg-bg">
              Security
            </Link>
            <Link to="/docs" onClick={() => setOpen(false)} className="rounded-control px-3 py-2.5 text-sm font-medium hover:bg-bg">
              Developers
            </Link>
            <div className="my-2 h-px bg-line" />
            {me ? (
              <Link to="/app" onClick={() => setOpen(false)} className="btn-primary">
                Open app
              </Link>
            ) : (
              <>
                <Link to="/signin" onClick={() => setOpen(false)} className="btn-secondary">
                  Sign in
                </Link>
                <Link to="/claim" onClick={() => setOpen(false)} className="btn-primary">
                  Claim your handle
                </Link>
              </>
            )}
          </div>
        </div>
      ) : null}
    </header>
  );
}

function Footer() {
  return (
    <footer className="border-t border-line bg-surface">
      <div className="mx-auto flex max-w-6xl flex-col gap-4 px-4 py-8 text-sm sm:px-6 md:flex-row md:items-center md:justify-between">
        <div className="flex flex-wrap items-center gap-3">
          <Wordmark />
          <span className="muted">Your identity. Your money. Your handle.</span>
        </div>
        <div className="flex flex-wrap gap-5 muted">
          <Link to="/terms" className="hover:text-ink">
            Terms
          </Link>
          <Link to="/privacy" className="hover:text-ink">
            Privacy
          </Link>
          <Link to="/docs#security" className="hover:text-ink">
            Security
          </Link>
          <a
            href="https://github.com/"
            className="hover:text-ink"
            target="_blank"
            rel="noreferrer noopener"
          >
            GitHub
          </a>
        </div>
      </div>
    </footer>
  );
}

export function MarketingShell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-full flex-col">
      <MarketingNav />
      <main className="flex-1">{children}</main>
      <Footer />
    </div>
  );
}

const appTabs = [
  { to: "/app", label: "Overview", end: true },
  { to: "/app/handle", label: "My Handle", end: false },
  { to: "/app/payments", label: "Payments", end: false },
  { to: "/app/identities", label: "Identities", end: false },
  { to: "/app/settings", label: "Settings", end: false },
];

function AccountMenu() {
  const { me, logout } = useAuth();
  const { connected, address, providerName, disconnect } = useWallet();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  return (
    <div className="relative">
      <button
        type="button"
        className="btn-secondary"
        aria-expanded={open}
        aria-haspopup="menu"
        onClick={() => setOpen((v) => !v)}
      >
        {connected && address ? shortenAddress(address) : "Account"}
      </button>
      {open ? (
        <div
          role="menu"
          className="card absolute right-0 z-30 mt-2 w-64 p-2"
          onMouseLeave={() => setOpen(false)}
        >
          <div className="px-3 py-2 text-xs muted">Signed in as</div>
          <div className="truncate px-3 pb-2 text-sm font-medium">{me?.email}</div>
          <div className="my-1 h-px bg-line" />
          <div className="px-3 py-2 text-xs muted">
            Wallet: {connected ? `${providerName ?? "connected"} · ${shortenAddress(address ?? "")}` : "not connected"}
          </div>
          {connected ? (
            <button
              type="button"
              role="menuitem"
              className="w-full rounded-control px-3 py-2 text-left text-sm hover:bg-bg"
              onClick={() => {
                void disconnect();
                setOpen(false);
              }}
            >
              Disconnect wallet
            </button>
          ) : null}
          <button
            type="button"
            role="menuitem"
            className="w-full rounded-control px-3 py-2 text-left text-sm hover:bg-bg"
            onClick={async () => {
              setOpen(false);
              await logout();
              navigate("/");
            }}
          >
            Sign out
          </button>
        </div>
      ) : null}
    </div>
  );
}

export function AppShell({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="flex min-h-full flex-col">
      <header className="border-b border-line bg-surface">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-3 sm:px-6">
          <div className="flex items-center gap-3">
            <Wordmark />
            <ClusterChip />
          </div>
          <div className="flex items-center gap-2">
            <AccountMenu />
            <button
              type="button"
              className="btn-secondary md:hidden"
              aria-expanded={open}
              aria-controls="app-mobile-menu"
              onClick={() => setOpen((v) => !v)}
            >
              {open ? "Close" : "Menu"}
            </button>
          </div>
        </div>
        <div className="hidden border-t border-line md:block">
          <nav className="mx-auto flex max-w-6xl gap-1 px-4 sm:px-6">
            {appTabs.map((t) => (
              <NavLink
                key={t.to}
                to={t.to}
                end={t.end}
                className={({ isActive }) =>
                  `border-b-2 px-3 py-3 text-sm font-medium ${
                    isActive ? "border-accent text-ink" : "border-transparent text-muted hover:text-ink"
                  }`
                }
              >
                {t.label}
              </NavLink>
            ))}
          </nav>
        </div>
      </header>
      {open ? (
        <div id="app-mobile-menu" className="border-b border-line bg-surface md:hidden">
          <nav className="mx-auto flex max-w-6xl flex-col px-4 py-2">
            {appTabs.map((t) => (
              <NavLink
                key={t.to}
                to={t.to}
                end={t.end}
                onClick={() => setOpen(false)}
                className={({ isActive }) =>
                  `rounded-control px-3 py-2.5 text-sm font-medium ${
                    isActive ? "bg-bg text-ink" : "text-muted hover:text-ink"
                  }`
                }
              >
                {t.label}
              </NavLink>
            ))}
          </nav>
        </div>
      ) : null}
      <main className="flex-1">
        <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 sm:py-8">{children}</div>
      </main>
    </div>
  );
}
