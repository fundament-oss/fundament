import settledPool from './settled-pool';

describe('settledPool', () => {
  it('returns results in input order, not completion order', async () => {
    // Later ids resolve sooner, so anything that collected results as they
    // landed would come back reversed.
    const delays: Record<string, number> = { a: 30, b: 20, c: 10 };
    const results = await settledPool(['a', 'b', 'c'], 3, async (id) => {
      await new Promise((resolve) => {
        setTimeout(resolve, delays[id]);
      });
      return id.toUpperCase();
    });

    expect(results.map((r) => (r.status === 'fulfilled' ? r.value : r.reason))).toEqual([
      'A',
      'B',
      'C',
    ]);
  });

  it('runs every id exactly once', async () => {
    const ids = Array.from({ length: 25 }, (_, i) => `id-${i}`);
    const seen: string[] = [];

    await settledPool(ids, 6, async (id) => {
      seen.push(id);
      return id;
    });

    expect(seen.slice().sort()).toEqual(ids.slice().sort());
    expect(seen).toHaveLength(ids.length);
  });

  it('never exceeds the concurrency limit', async () => {
    let inFlight = 0;
    let peak = 0;

    await settledPool(
      Array.from({ length: 20 }, (_, i) => String(i)),
      4,
      async (id) => {
        inFlight += 1;
        peak = Math.max(peak, inFlight);
        await new Promise((resolve) => {
          setTimeout(resolve, 1);
        });
        inFlight -= 1;
        return id;
      },
    );

    expect(peak).toBeLessThanOrEqual(4);
  });

  it('keeps going after a rejection and reports it per id', async () => {
    // The bulk actions depend on this: one task failing must not abandon the
    // rest, and the failure has to stay countable so the toast can say so.
    const results = await settledPool(['ok-1', 'boom', 'ok-2'], 2, async (id) => {
      if (id === 'boom') throw new Error('nope');
      return id;
    });

    expect(results.map((r) => r.status)).toEqual(['fulfilled', 'rejected', 'fulfilled']);
    expect(results.filter((r) => r.status === 'rejected')).toHaveLength(1);
  });

  it('resolves to an empty list for no ids without invoking the task', async () => {
    const task = vi.fn();

    await expect(settledPool([], 6, task)).resolves.toEqual([]);
    expect(task).not.toHaveBeenCalled();
  });

  it('still runs every id when the limit is zero, rather than resolving to holes', async () => {
    // A limit of zero would start no workers at all, leaving every slot in the
    // results array an unwritten hole that reads as an undefined settlement.
    const results = await settledPool(['a', 'b'], 0, async (id) => id.toUpperCase());

    expect(results).toEqual([
      { status: 'fulfilled', value: 'A' },
      { status: 'fulfilled', value: 'B' },
    ]);
  });

  it('accepts non-string items, so callers can pool over objects', async () => {
    const results = await settledPool([{ id: 'a' }, { id: 'b' }], 2, async (t) => t.id);

    expect(results.map((r) => (r.status === 'fulfilled' ? r.value : null))).toEqual(['a', 'b']);
  });
});
