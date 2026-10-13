// Regression tests for the transaction-confirmation decision.
//
// The bug these lock in: the client confirmed with Connection.confirmTransaction,
// which settles by subscribing to the signature over a WebSocket. Whenever that
// socket was unavailable -- a Content-Security-Policy whose connect-src had no
// `wss:` entry, a proxy, a flaky network -- the only promise left able to settle
// was the block-height timer, so a transfer that had landed perfectly well was
// reported as "dropped". The sender was then told a payment had failed that
// actually went through.
//
// Run with: npm run test

import { test } from "node:test";
import assert from "node:assert/strict";

import { pollForLanding, type SignatureStatus, type SignatureWatcher } from "./confirm.ts";

const LAST_VALID_BLOCK_HEIGHT = 100;

interface FakeCalls {
  status?: (call: number) => SignatureStatus | null;
  height?: (call: number) => number;
  ledger?: (call: number) => { meta?: { err?: unknown } | null } | null;
}

/** A stand-in cluster. Each method counts its own calls so a test can script a sequence. */
function fakeWatcher(calls: FakeCalls): SignatureWatcher {
  const seen = { status: 0, height: 0, ledger: 0 };
  return {
    async getSignatureStatus() {
      const value = calls.status ? calls.status(seen.status++) : null;
      return { value };
    },
    async getBlockHeight() {
      return calls.height ? calls.height(seen.height++) : LAST_VALID_BLOCK_HEIGHT;
    },
    async getTransaction() {
      return calls.ledger ? calls.ledger(seen.ledger++) : null;
    },
  };
}

/** A virtual clock, so the poll loop runs instantly and deterministically. */
function fakeClock() {
  let elapsed = 0;
  return {
    now: () => elapsed,
    sleep: async (ms: number) => {
      elapsed += ms;
    },
  };
}

function options() {
  return { timeoutMs: 10_000, pollIntervalMs: 1_000, ...fakeClock() };
}

test("a landed transfer the status index has not caught up with is not called dropped", async () => {
  // The reported bug, exactly: the index says "not found", the blockhash window
  // has closed, and yet the ledger does contain the transaction.
  const result = await pollForLanding(
    fakeWatcher({
      status: () => null,
      height: () => LAST_VALID_BLOCK_HEIGHT + 1,
      ledger: () => ({ meta: { err: null } }),
    }),
    "SIGNATURE",
    LAST_VALID_BLOCK_HEIGHT,
    options(),
  );
  assert.equal(result.landing, "confirmed");
});

test("dropped is only reported once the ledger itself cannot produce the transaction", async () => {
  const result = await pollForLanding(
    fakeWatcher({
      status: () => null,
      height: () => LAST_VALID_BLOCK_HEIGHT + 1,
      ledger: () => null,
    }),
    "SIGNATURE",
    LAST_VALID_BLOCK_HEIGHT,
    options(),
  );
  assert.equal(result.landing, "dropped");
  assert.match(String(result.detail), /blockhash expired/);
});

test("a confirmed status settles without reading the ledger", async () => {
  const result = await pollForLanding(
    fakeWatcher({
      status: () => ({ confirmationStatus: "confirmed" }),
      ledger: () => {
        throw new Error("the ledger must not be consulted for a confirmed signature");
      },
    }),
    "SIGNATURE",
    LAST_VALID_BLOCK_HEIGHT,
    options(),
  );
  assert.equal(result.landing, "confirmed");
});

test("a status carrying an error is failed", async () => {
  const result = await pollForLanding(
    fakeWatcher({ status: () => ({ err: { InstructionError: [0, "Custom"] } }) }),
    "SIGNATURE",
    LAST_VALID_BLOCK_HEIGHT,
    options(),
  );
  assert.equal(result.landing, "failed");
});

test("a transaction the ledger has but whose transfer failed is failed, not confirmed", async () => {
  const result = await pollForLanding(
    fakeWatcher({
      status: () => null,
      height: () => LAST_VALID_BLOCK_HEIGHT + 1,
      ledger: () => ({ meta: { err: { InstructionError: [0, "Custom"] } } }),
    }),
    "SIGNATURE",
    LAST_VALID_BLOCK_HEIGHT,
    options(),
  );
  assert.equal(result.landing, "failed");
});

test("an unreachable RPC yields unknown, never dropped", async () => {
  const down = () => {
    throw new Error("rpc down");
  };
  const result = await pollForLanding(
    fakeWatcher({ status: down, height: down, ledger: down }),
    "SIGNATURE",
    LAST_VALID_BLOCK_HEIGHT,
    options(),
  );
  assert.equal(result.landing, "unknown");
  assert.match(String(result.detail), /rpc down/);
});

test("a ledger read that keeps failing never becomes dropped", async () => {
  const result = await pollForLanding(
    fakeWatcher({
      status: () => null,
      height: () => LAST_VALID_BLOCK_HEIGHT + 1,
      ledger: () => {
        throw new Error("ledger unavailable");
      },
    }),
    "SIGNATURE",
    LAST_VALID_BLOCK_HEIGHT,
    options(),
  );
  assert.equal(result.landing, "unknown");
  assert.match(String(result.detail), /ledger unavailable/);
});

test("a processed status is waited out until the cluster confirms it", async () => {
  let slept = 0;
  const result = await pollForLanding(
    fakeWatcher({
      status: (call) => ({ confirmationStatus: call === 0 ? "processed" : "confirmed" }),
    }),
    "SIGNATURE",
    LAST_VALID_BLOCK_HEIGHT,
    {
      timeoutMs: 10_000,
      pollIntervalMs: 1_000,
      now: () => slept,
      sleep: async (ms: number) => {
        slept += ms;
      },
    },
  );
  assert.equal(result.landing, "confirmed");
  assert.ok(slept >= 1_000, "it must have waited before the second status check");
});

test("a transaction that stays processed is unknown, not dropped", async () => {
  // The blockhash window closing says nothing while the signature is visibly on
  // the ledger; only an absent transaction may be called lost.
  const result = await pollForLanding(
    fakeWatcher({
      status: () => ({ confirmationStatus: "processed" }),
      height: () => LAST_VALID_BLOCK_HEIGHT + 1,
    }),
    "SIGNATURE",
    LAST_VALID_BLOCK_HEIGHT,
    options(),
  );
  assert.equal(result.landing, "unknown");
});
