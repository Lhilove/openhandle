import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { Connection, PublicKey, SystemProgram, Transaction } from "@solana/web3.js";

import { pollForLanding, type TxLanding } from "./confirm";

// Re-exported for callers that describe an outcome without polling themselves.
export type { TxLanding };

// Wallet Standard-compatible connection: we detect injected providers (Phantom,
// Solflare) that implement the standard connect/signMessage/signAndSendTransaction
// surface. Private keys never leave the wallet.

export const SOLANA_RPC_URL: string =
  (import.meta.env.VITE_SOLANA_RPC_URL as string | undefined) || "https://api.devnet.solana.com";

interface InjectedProvider {
  publicKey: PublicKey | null;
  isPhantom?: boolean;
  isSolflare?: boolean;
  connect(opts?: { onlyIfTrusted?: boolean }): Promise<{ publicKey: PublicKey }>;
  disconnect(): Promise<void>;
  signMessage(message: Uint8Array, display?: string): Promise<{ signature: Uint8Array }>;
  signAndSendTransaction(transaction: Transaction): Promise<{ signature: string }>;
  on?(event: string, cb: (...args: unknown[]) => void): void;
  off?(event: string, cb: (...args: unknown[]) => void): void;
}

declare global {
  interface Window {
    solana?: InjectedProvider;
    solflare?: InjectedProvider;
    phantom?: { solana?: InjectedProvider };
  }
}

const ALPHABET = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz";
function base58Encode(bytes: Uint8Array): string {
  let zeros = 0;
  while (zeros < bytes.length && bytes[zeros] === 0) zeros++;
  let num = 0n;
  for (const b of bytes) num = num * 256n + BigInt(b);
  let out = "";
  while (num > 0n) {
    const rem = Number(num % 58n);
    num /= 58n;
    out = (ALPHABET[rem] ?? "") + out;
  }
  return "1".repeat(zeros) + out;
}

interface DetectedWallet {
  name: string;
  provider: InjectedProvider;
}

function detectWallets(): DetectedWallet[] {
  if (typeof window === "undefined") return [];
  const out: DetectedWallet[] = [];
  const phantom = window.phantom?.solana ?? (window.solana?.isPhantom ? window.solana : undefined);
  if (phantom) out.push({ name: "Phantom", provider: phantom });
  if (window.solflare) out.push({ name: "Solflare", provider: window.solflare });
  return out;
}

export interface SendOutcome {
  signature: string;
  /**
   * What the cluster says happened to the transaction we just submitted.
   *
   * - confirmed: it is on-chain
   * - failed: it landed but the transfer itself failed
   * - dropped: the network never accepted it (the blockhash expired first), so
   *   no funds moved and the sender can simply try again
   * - unknown: we could not reach the cluster to find out; the API is then the
   *   authority on the final status
   */
  landing: TxLanding;
  detail?: string;
}

export interface WalletContextValue {
  available: { name: string }[];
  address: string | null;
  providerName: string | null;
  connected: boolean;
  connecting: boolean;
  error: string | null;
  cluster: string;
  connection: Connection;
  connect: (name?: string) => Promise<void>;
  disconnect: () => Promise<void>;
  signMessage: (message: string) => Promise<string>;
  sendSol: (to: string, lamports: bigint) => Promise<SendOutcome>;
}

const WalletContext = createContext<WalletContextValue | null>(null);

export function WalletProvider({ children }: { children: ReactNode }) {
  const connection = useMemo(() => new Connection(SOLANA_RPC_URL, "confirmed"), []);
  const [available, setAvailable] = useState<{ name: string }[]>([]);
  const [provider, setProvider] = useState<InjectedProvider | null>(null);
  const [providerName, setProviderName] = useState<string | null>(null);
  const [address, setAddress] = useState<string | null>(null);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const interval = window.setInterval(() => {
      const detected = detectWallets();
      setAvailable(detected.map((w) => ({ name: w.name })));
    }, 1000);
    const detected = detectWallets();
    setAvailable(detected.map((w) => ({ name: w.name })));
    return () => window.clearInterval(interval);
  }, []);

  const connect = useCallback(
    async (name?: string) => {
      setError(null);
      const detected = detectWallets();
      if (detected.length === 0) {
        setError("No Solana wallet was detected. Install Phantom or Solflare to continue.");
        return;
      }
      const target = name ? detected.find((w) => w.name === name) ?? detected[0] : detected[0];
      if (!target) return;
      setConnecting(true);
      try {
        const res = await target.provider.connect();
        setProvider(target.provider);
        setProviderName(target.name);
        setAddress(res.publicKey.toString());
      } catch {
        setError("Wallet connection was rejected or cancelled.");
      } finally {
        setConnecting(false);
      }
    },
    [],
  );

  const disconnect = useCallback(async () => {
    try {
      await provider?.disconnect();
    } catch {
      // ignore disconnect errors
    }
    setProvider(null);
    setProviderName(null);
    setAddress(null);
  }, [provider]);

  const signMessage = useCallback(
    async (message: string): Promise<string> => {
      if (!provider) throw new Error("Connect a wallet before signing.");
      const encoded = new TextEncoder().encode(message);
      const { signature } = await provider.signMessage(encoded, "utf8");
      return base58Encode(signature);
    },
    [provider],
  );

  const sendSol = useCallback(
    async (to: string, lamports: bigint): Promise<SendOutcome> => {
      if (!provider || !provider.publicKey) throw new Error("Connect a wallet before sending.");
      const tx = new Transaction().add(
        SystemProgram.transfer({
          fromPubkey: provider.publicKey,
          toPubkey: new PublicKey(to),
          lamports: Number(lamports),
        }),
      );
      tx.feePayer = provider.publicKey;
      const { blockhash, lastValidBlockHeight } = await connection.getLatestBlockhash("confirmed");
      tx.recentBlockhash = blockhash;
      const { signature } = await provider.signAndSendTransaction(tx);

      // A signature from the wallet only means it was submitted. Find out what
      // the cluster actually did with it -- but only over HTTP, and never guess
      // "lost" without the ledger agreeing.
      const { landing, detail } = await pollForLanding(connection, signature, lastValidBlockHeight);
      return { signature, landing, detail };
    },
    [provider, connection],
  );

  const value: WalletContextValue = {
    available,
    address,
    providerName,
    connected: !!address,
    connecting,
    error,
    cluster: (import.meta.env.VITE_SOLANA_CLUSTER as string | undefined) || "devnet",
    connection,
    connect,
    disconnect,
    signMessage,
    sendSol,
  };

  return <WalletContext.Provider value={value}>{children}</WalletContext.Provider>;
}

export function useWallet(): WalletContextValue {
  const ctx = useContext(WalletContext);
  if (!ctx) throw new Error("useWallet must be used within a WalletProvider");
  return ctx;
}
