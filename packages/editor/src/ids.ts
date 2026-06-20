/**
 * Document-unique id generation for layers and strokes.
 *
 * The document treats ids as opaque, stable, ≤64 chars (DOCUMENT-FORMAT §8).
 * `crypto.randomUUID()` is a global in Node 20+/24 and every modern browser,
 * so no import is needed. "stk_" + a UUID is 4 + 36 = 40 chars, well under 64.
 */
export function newId(): string {
  return `stk_${crypto.randomUUID()}`;
}
