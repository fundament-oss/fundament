/**
 * Runs `task` over every id with at most `limit` requests in flight, resolving
 * like Promise.allSettled: results come back in input order and a rejection
 * never short-circuits the rest.
 */
export default async function settledPool<T>(
  ids: string[],
  limit: number,
  task: (id: string) => Promise<T>,
): Promise<PromiseSettledResult<T>[]> {
  const results = new Array<PromiseSettledResult<T>>(ids.length);
  let next = 0;

  const worker = async (): Promise<void> => {
    // Claiming the index and advancing the cursor happens synchronously before
    // the await, so concurrent workers never take the same id.
    for (;;) {
      const i = next;
      next += 1;
      if (i >= ids.length) return;
      try {
        // Sequential within a worker is the point: parallelism comes from
        // running `limit` workers, which is what bounds the requests in flight.
        // eslint-disable-next-line no-await-in-loop
        results[i] = { status: 'fulfilled', value: await task(ids[i]) };
      } catch (reason) {
        results[i] = { status: 'rejected', reason };
      }
    }
  };

  await Promise.all(Array.from({ length: Math.min(limit, ids.length) }, worker));
  return results;
}
