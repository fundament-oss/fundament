/**
 * Polling for steps that wait on Gardener, which takes minutes rather than the
 * 30 s default step timeout.
 */

const POLL_INTERVAL_MS = 15_000;

/**
 * Poll `check` every POLL_INTERVAL_MS until it returns a value, or throw once
 * `deadlineMs` has elapsed. `check` returns `undefined` to keep polling.
 */
export async function pollUntil<T>(
  deadlineMs: number,
  description: string,
  check: () => Promise<T | undefined>,
): Promise<T> {
  const deadline = Date.now() + deadlineMs;
  for (;;) {
    const result = await check();
    if (result !== undefined) {
      return result;
    }
    if (Date.now() >= deadline) {
      throw new Error(
        `Timed out after ${Math.round(deadlineMs / 1000)}s waiting for: ${description}`,
      );
    }
    await new Promise((resolve) => setTimeout(resolve, Math.min(POLL_INTERVAL_MS, deadline - Date.now())));
  }
}
