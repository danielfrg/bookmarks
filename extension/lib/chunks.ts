/** Characters (UTF-16 code units) of HTML per runtime message. */
export const CHUNK_CHARS = 8 * 1024 * 1024;

/**
 * Split `s` into pieces of at most `size` UTF-16 code units, never between
 * the two halves of a surrogate pair, so each piece is valid on its own and
 * survives JSON serialization. Always returns at least one piece.
 */
export function splitString(s: string, size = CHUNK_CHARS): string[] {
  if (!Number.isInteger(size) || size < 2)
    throw new RangeError(`chunk size must be >= 2, got ${size}`);
  if (s.length <= size) return [s];
  const parts: string[] = [];
  let start = 0;
  while (start < s.length) {
    let end = Math.min(start + size, s.length);
    if (end < s.length) {
      const last = s.charCodeAt(end - 1);
      if (last >= 0xd800 && last <= 0xdbff) end--;
    }
    parts.push(s.slice(start, end));
    start = end;
  }
  return parts;
}

export function joinChunks(parts: string[]): string {
  return parts.join("");
}

// String.fromCharCode.apply takes its arguments on the stack; keep slices small.
const B64_SLICE = 0x8000;

export function bytesToBase64(b: Uint8Array): string {
  const pieces: string[] = [];
  for (let i = 0; i < b.length; i += B64_SLICE) {
    pieces.push(
      String.fromCharCode.apply(null, b.subarray(i, i + B64_SLICE) as unknown as number[]),
    );
  }
  return btoa(pieces.join(""));
}

export function base64ToBytes(s: string): Uint8Array {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

/**
 * Reassembles the capture script's chunk messages. Chunks must arrive in
 * order from index 0 with a constant `total`; anything else throws.
 */
export class ChunkAssembler {
  private parts: string[] = [];
  private total = -1;

  add(index: number, total: number, data: string): void {
    if (!Number.isInteger(total) || total < 1) throw new Error(`invalid chunk total ${total}`);
    if (this.total === -1) this.total = total;
    else if (total !== this.total)
      throw new Error(`chunk total changed: ${this.total} -> ${total}`);
    if (index !== this.parts.length) {
      throw new Error(`chunk out of order: expected ${this.parts.length}, got ${index}`);
    }
    if (typeof data !== "string") throw new Error("chunk data is not a string");
    this.parts.push(data);
  }

  /** The joined HTML; throws unless every chunk arrived. */
  finish(): string {
    if (this.total === -1 || this.parts.length !== this.total) {
      throw new Error(`incomplete capture: ${this.parts.length} of ${this.total} chunks`);
    }
    const html = joinChunks(this.parts);
    this.parts = [];
    return html;
  }
}
