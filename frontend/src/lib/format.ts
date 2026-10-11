const LAMPORTS_PER_SOL = 1_000_000_000;

/** Shorten a base58 address for display, e.g. 9xQe...3fPz */
export function shortenAddress(address: string, chars = 4): string {
  if (address.length <= chars * 2 + 3) return address;
  return `${address.slice(0, chars)}…${address.slice(-chars)}`;
}

/** Convert lamports (integer base units) to a human SOL string, without float error. */
export function lamportsToSol(lamports: string): string {
  let n: bigint;
  try {
    n = BigInt(lamports);
  } catch {
    return "0";
  }
  const whole = n / BigInt(LAMPORTS_PER_SOL);
  const frac = n % BigInt(LAMPORTS_PER_SOL);
  if (frac === 0n) return whole.toString();
  const fracStr = frac.toString().padStart(9, "0").replace(/0+$/, "");
  return `${whole}.${fracStr}`;
}

/** Parse a decimal SOL string to integer lamports. Returns null when invalid. */
export function solToLamports(input: string): bigint | null {
  const s = input.trim();
  if (!/^\d+(\.\d{0,9})?$/.test(s)) return null;
  const [whole = "0", frac = ""] = s.split(".");
  const padded = (frac + "000000000").slice(0, 9);
  try {
    return BigInt(whole) * BigInt(LAMPORTS_PER_SOL) + BigInt(padded);
  } catch {
    return null;
  }
}

export function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

export function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return iso;
  const diff = Date.now() - then;
  const mins = Math.round(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.round(hours / 24);
  return `${days}d ago`;
}
