// Helpers for the cluster/namespace limit forms, where a proto int32 of 0 (or
// absent) means "no limit set" rather than a real zero value.

export function toInt(value: unknown): number | undefined {
  const n = Math.trunc(Number(value));
  return n > 0 ? n : undefined;
}

export function positive(value: number | undefined): number | undefined {
  return value && value > 0 ? value : undefined;
}

/**
 * Whether a request/limit pair counts as limited: either half being set is a
 * constraint, so a form opens on what is actually stored.
 */
export function pairLimited(request: number | undefined, limit: number | undefined): boolean {
  return request !== undefined || limit !== undefined;
}
