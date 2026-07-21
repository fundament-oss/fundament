/**
 * Runs `task` over every item with at most `limit` requests in flight, resolving
 * like Promise.allSettled: results come back in input order and a rejection
 * never short-circuits the rest.
 */
export default async function settledPool<I, T>(
  items: readonly I[],
  limit: number,
  task: (item: I) => Promise<T>,
): Promise<PromiseSettledResult<T>[]> {
  const results = new Array<PromiseSettledResult<T>>(items.length);
  let next = 0;

  const worker = async (): Promise<void> => {
    // Claiming the index and advancing the cursor happens synchronously before
    // the await, so concurrent workers never take the same item.
    for (;;) {
      const i = next;
      next += 1;
      if (i >= items.length) return;
      try {
        // Sequential within a worker is the point: parallelism comes from
        // running `limit` workers, which is what bounds the requests in flight.
        // eslint-disable-next-line no-await-in-loop
        results[i] = { status: 'fulfilled', value: await task(items[i]) };
      } catch (reason) {
        results[i] = { status: 'rejected', reason };
      }
    }
  };

  // Floored at one worker: a limit of zero (or less) would start nothing at
  // all, and `results` would resolve as a sparse array of holes that every
  // caller would then read as undefined settlements. Degrade to running the
  // items one at a time rather than silently dropping all of them.
  const workers = Math.max(1, Math.min(limit, items.length));
  await Promise.all(Array.from({ length: workers }, worker));
  return results;
}
