// Helpers for the cluster/namespace limit forms, where a proto int32 of 0 (or
// absent) means "no limit set" rather than a real zero value.

export function toInt(value: unknown): number | undefined {
  const n = Math.trunc(Number(value));
  return n > 0 ? n : undefined;
}

export function positive(value: number | undefined): number | undefined {
  return value && value > 0 ? value : undefined;
}
