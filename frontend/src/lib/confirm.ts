// Deciding what the cluster actually did with a submitted transaction.
//
// This is deliberately plain HTTP JSON-RPC polling, and deliberately not
// `Connection.confirmTransaction`: that resolves by subscribing to the
// signature over a WebSocket. When the socket is unavailable -- a
// Content-Security-Policy whose connect-src has no `wss:` entry, a proxy, a
// flaky network -- the subscription never fires, so the only promise left able
// to settle is the block-height timer. A transfer that landed perfectly well is
// then reported as dropped, which tells the sender a payment failed that
// actually went through. Polling has no such failure mode.
//
// The rule this module enforces: never report "dropped" unless the ledger
// itself cannot produce the transaction. The status index can lag or omit a
// signature the ledger does contain, so a closed blockhash window alone is not
// evidence.

export type TxLanding = "confirmed" | "failed" | "dropped" | "unknown";

export interface LandingResult {
  landing: TxLanding;
  detail?: string;
}

export interface SignatureStatus {
  err?: unknown;
  confirmationStatus?: string;
}

/** The subset of `Connection` this needs, so a fake can stand in for tests. */
export interface SignatureWatcher {
  getSignatureStatus(
    signature: string,
    config?: { searchTransactionHistory?: boolean },
  ): Promise<{ value: SignatureStatus | null } | null>;
  // No commitment argument: the caller's Connection already carries the one it
  // wants ("confirmed" for the wallet connection).
  getBlockHeight(): Promise<number>;
  getTransaction(
    signature: string,
    config?: { commitment?: string; maxSupportedTransactionVersion?: number },
  ): Promise<{ meta?: { err?: unknown } | null } | null>;
}

export const DEFAULT_CONFIRM_TIMEOUT_MS = 90_000;
export const DEFAULT_POLL_INTERVAL_MS = 2_000;

export interface PollOptions {
  timeoutMs?: number;
  pollIntervalMs?: number;
  /** Injectable clock; tests advance it instead of really waiting. */
  now?: () => number;
  sleep?: (ms: number) => Promise<void>;
}

/**
 * Watches a submitted signature until the cluster decides its fate.
 *
 * `lastValidBlockHeight` comes from the blockhash the transaction was built
 * with: once the cluster's block height passes it, the transaction can never
 * land, so that is the point at which "not found" becomes a decision.
 */
export async function pollForLanding(
  watcher: SignatureWatcher,
  signature: string,
  lastValidBlockHeight: number,
  options: PollOptions = {},
): Promise<LandingResult> {
  const timeoutMs = options.timeoutMs ?? DEFAULT_CONFIRM_TIMEOUT_MS;
  const pollIntervalMs = options.pollIntervalMs ?? DEFAULT_POLL_INTERVAL_MS;
  const now = options.now ?? (() => Date.now());
  const sleep = options.sleep ?? ((ms: number) => new Promise<void>((r) => setTimeout(r, ms)));
  const deadline = now() + timeoutMs;

  let heightExceeded = false;
  let lastDetail: string | undefined;

  while (now() < deadline) {
    let status: SignatureStatus | null = null;
    try {
      const res = await watcher.getSignatureStatus(signature, { searchTransactionHistory: true });
      status = res?.value ?? null;
      if (status) {
        if (status.err) {
          return { landing: "failed", detail: "the transfer failed on-chain" };
        }
        if (status.confirmationStatus === "confirmed" || status.confirmationStatus === "finalized") {
          return { landing: "confirmed" };
        }
        // "processed": on the ledger but not confirmed yet. Keep waiting.
      } else if (!heightExceeded) {
        const height = await watcher.getBlockHeight();
        if (height > lastValidBlockHeight) heightExceeded = true;
      }
    } catch (err) {
      // A transient RPC failure says nothing about the transaction: remember it
      // and keep polling rather than inventing an outcome.
      lastDetail = err instanceof Error ? err.message : String(err);
      await sleep(pollIntervalMs);
      continue;
    }

    if (heightExceeded && !status) {
      // The blockhash can no longer be used, so if the transaction is not on the
      // ledger it never will be. Ask the ledger itself, not the status index.
      let landed: { meta?: { err?: unknown } | null } | null;
      try {
        landed = await watcher.getTransaction(signature, {
          commitment: "confirmed",
          maxSupportedTransactionVersion: 0,
        });
      } catch (err) {
        lastDetail = err instanceof Error ? err.message : String(err);
        await sleep(pollIntervalMs);
        continue;
      }
      if (landed) {
        return { landing: landed.meta?.err ? "failed" : "confirmed" };
      }
      return {
        landing: "dropped",
        detail: "the blockhash expired before any node accepted the transaction",
      };
    }

    await sleep(pollIntervalMs);
  }

  return { landing: "unknown", detail: lastDetail ?? "timed out waiting for the cluster" };
}
