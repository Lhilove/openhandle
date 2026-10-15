// Typed API client. Uses cookie-based sessions (credentials: "include") and
// echoes the session CSRF token on mutating requests. The CSRF token is held in
// memory only -- never in localStorage.

const configuredApiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.replace(/\/$/, "");

// In production an unset value means "same origin": the deployed app reaches
// the API through the reverse proxy on its own host (see the edge proxy rules
// in vercel.json). Defaulting to a localhost URL in a build would point every
// visitor's browser at their own machine.
//
export const API_BASE: string = configuredApiBase || (import.meta.env.DEV ? "http://localhost:8090" : "");

export const SOLANA_CLUSTER: string =
  (import.meta.env.VITE_SOLANA_CLUSTER as string | undefined) || "devnet";

let csrfToken = "";

export function setCsrfToken(token: string | undefined | null): void {
  csrfToken = token ?? "";
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

interface RequestOptions {
  method?: string;
  body?: unknown;
  signal?: AbortSignal;
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const method = opts.method ?? "GET";
  const headers: Record<string, string> = { Accept: "application/json" };
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";
  if (method !== "GET" && method !== "HEAD" && csrfToken) {
    headers["X-CSRF-Token"] = csrfToken;
  }

  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      method,
      headers,
      credentials: "include",
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      signal: opts.signal,
    });
  } catch {
    throw new ApiError(0, "NETWORK_ERROR", "Could not reach the server. Check your connection.");
  }

  const text = await res.text();
  const data = text ? safeParse(text) : null;

  if (!res.ok) {
    const err = (data as { error?: { code?: string; message?: string } } | null)?.error;
    throw new ApiError(res.status, err?.code ?? "ERROR", err?.message ?? `Request failed (${res.status}).`);
  }
  return data as T;
}

function safeParse(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

// ---- types ----

export interface HandlePrivate {
  id: string;
  handle: string;
  display_name: string | null;
  bio: string | null;
  is_public: boolean;
  public_url: string;
}

export interface WalletPrivate {
  id: string;
  network: string;
  address: string;
  is_primary: boolean;
  ownership_verified: boolean;
  ownership_verified_at?: string;
}

export interface Identity {
  provider: string;
  username: string;
  verified: boolean;
}

export interface Me {
  email: string;
  created_at: string;
  csrf_token: string;
  handle: HandlePrivate | null;
  wallets: WalletPrivate[];
  identities: Identity[];
}

export interface PublicWallet {
  network: string;
  address: string;
  is_primary: boolean;
  ownership_verified: boolean;
}

export interface PublicProfile {
  handle: string;
  profile_url: string;
  display_name: string | null;
  bio: string | null;
  network: string;
  wallets: PublicWallet[];
  identities: Identity[];
}

export interface Tx {
  id: string;
  network: string;
  signature: string;
  sender_address: string | null;
  recipient_address: string;
  amount_base_units: string;
  asset_identifier: string;
  status: "submitted" | "confirmed" | "failed" | "expired";
  slot: number | null;
  submitted_at: string;
  confirmed_at: string | null;
  /** Which side of the payment the signed-in account is on. */
  direction: "sent" | "received";
  /** The other party's public handle, when it is public and active. */
  counterparty_handle: string | null;
  counterparty_address: string;
}

export interface TransactionList {
  transactions: Tx[];
  total: number;
  limit: number;
  offset: number;
  scope: string;
}

export interface WalletChallenge {
  message: string;
  nonce: string;
  domain: string;
  network: string;
  expires_at: string;
}

export interface Availability {
  handle: string;
  available: boolean;
  reason: string;
}

export interface ProviderStatus {
  provider: string;
  configured: boolean;
}

export interface IdentityList {
  identities: Identity[];
  providers: ProviderStatus[];
  note: string;
}

// ---- endpoints ----

export const api = {
  register: (email: string, password: string) =>
    request<Me>("/api/v1/auth/register", { method: "POST", body: { email, password } }),
  login: (email: string, password: string) =>
    request<Me>("/api/v1/auth/login", { method: "POST", body: { email, password } }),
  logout: () => request<{ ok: boolean }>("/api/v1/auth/logout", { method: "POST" }),
  me: () => request<Me>("/api/v1/me"),

  availability: (handle: string) =>
    request<Availability>(`/api/v1/handles/availability?handle=${encodeURIComponent(handle)}`),
  claimHandle: (handle: string) =>
    request<HandlePrivate>("/api/v1/handles", { method: "POST", body: { handle } }),
  resolveHandle: (handle: string) => request<PublicProfile>(`/api/v1/handles/${encodeURIComponent(handle)}`),
  updateProfile: (body: { display_name?: string; bio?: string; is_public?: boolean }) =>
    request<HandlePrivate>("/api/v1/me/profile", { method: "PATCH", body }),

  listWallets: () => request<{ wallets: WalletPrivate[] }>("/api/v1/me/wallets"),
  createChallenge: (address: string, network = "solana") =>
    request<WalletChallenge>("/api/v1/wallet-challenges", { method: "POST", body: { address, network } }),
  verifyWallet: (address: string, nonce: string, signature: string, network = "solana") =>
    request<WalletPrivate>("/api/v1/wallets/verify", {
      method: "POST",
      body: { address, nonce, signature, network },
    }),
  deleteWallet: (id: string) => request<{ ok: boolean }>(`/api/v1/me/wallets/${id}`, { method: "DELETE" }),

  createTransaction: (body: {
    signature: string;
    recipient_handle: string;
    amount_base_units: string;
    asset?: string;
    sender_address?: string;
  }) => request<Tx>("/api/v1/transactions", { method: "POST", body }),
  listTransactions: (limit = 20, offset = 0) =>
    request<TransactionList>(`/api/v1/me/transactions?limit=${limit}&offset=${offset}`),
  getTransaction: (signature: string) =>
    request<Tx>(`/api/v1/transactions/${encodeURIComponent(signature)}`),

  deleteAccount: () => request<{ ok: boolean; note: string }>("/api/v1/me", { method: "DELETE" }),

  listIdentities: () => request<IdentityList>("/api/v1/me/identities"),
  disconnectIdentity: (provider: string) =>
    request<{ ok: boolean }>(`/api/v1/identities/${provider}`, { method: "DELETE" }),
  identityStartUrl: (provider: string) => `${API_BASE}/api/v1/identities/${provider}/start`,
};
