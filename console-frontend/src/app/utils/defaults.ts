// Helpers for the container-defaults forms, where a proto int32 of 0 (or
// absent) means "no default set" rather than a real zero value.

/** The largest value the proto int32 fields can carry. */
export const INT32_MAX = 2_147_483_647;

export function toInt(value: unknown): number | undefined {
  const n = Math.trunc(Number(value));
  return n > 0 && n <= INT32_MAX ? n : undefined;
}

export function positive(value: number | undefined): number | undefined {
  return value && value > 0 ? value : undefined;
}

/**
 * Whether a request/limit pair carries a default at all: either half being set
 * is one, so a form opens on what is actually stored.
 */
export function pairSet(request: number | undefined, limit: number | undefined): boolean {
  return request !== undefined || limit !== undefined;
}
