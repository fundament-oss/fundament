import { describe, it, expect, vi } from 'vitest';
import { PluginAuthService, type MintClient, type TokenSnapshot } from './plugin-auth.service';

function inFuture(seconds: number): number {
  return Math.floor(Date.now() / 1000) + seconds;
}

class FakeMintClient implements MintClient {
  callCount = 0;

  nextResp: TokenSnapshot | Error = { token: 't1', expiresAt: inFuture(900) };

  async mint(_c: string, _i: string): Promise<TokenSnapshot> {
    this.callCount += 1;
    if (this.nextResp instanceof Error) throw this.nextResp;
    return this.nextResp;
  }
}

// makeDeferredMint hands the test explicit control over when each mint settles,
// so timer/teardown races can be exercised deterministically. Kept as a factory
// (not a second class) to satisfy the one-class-per-file lint rule.
interface DeferredMint {
  client: MintClient;
  callCount: () => number;
  resolveNext: (snap: TokenSnapshot) => void;
  rejectNext: (err: Error) => void;
}

function makeDeferredMint(): DeferredMint {
  let count = 0;
  const pending: { resolve: (t: TokenSnapshot) => void; reject: (e: Error) => void }[] = [];
  return {
    client: {
      mint(): Promise<TokenSnapshot> {
        count += 1;
        return new Promise<TokenSnapshot>((resolve, reject) => {
          pending.push({ resolve, reject });
        });
      },
    },
    callCount: () => count,
    resolveNext: (snap) => pending.shift()?.resolve(snap),
    rejectNext: (err) => pending.shift()?.reject(err),
  };
}

const flush = (): Promise<void> => Promise.resolve().then(() => undefined);

describe('PluginAuthService', () => {
  it('mints a token and caches it per (cluster, installation)', async () => {
    const client = new FakeMintClient();
    const svc = new PluginAuthService(client);

    const a = await svc.acquire('c1', 'i1');
    const b = await svc.acquire('c1', 'i1');

    expect(a.token).toBe('t1');
    expect(b.token).toBe('t1');
    expect(client.callCount).toBe(1);
  });

  it('mints separate tokens for different installations', async () => {
    const client = new FakeMintClient();
    const svc = new PluginAuthService(client);

    await svc.acquire('c1', 'i1');
    client.nextResp = { token: 't2', expiresAt: inFuture(900) };
    await svc.acquire('c1', 'i2');

    expect(client.callCount).toBe(2);
  });

  it('exposes the latest token as a signal', async () => {
    const client = new FakeMintClient();
    const svc = new PluginAuthService(client);

    const sig = svc.tokenSignal('c1', 'i1');
    expect(sig()).toBeNull();

    await svc.acquire('c1', 'i1');
    expect(sig()?.token).toBe('t1');
  });

  it('rejects acquire and leaves the signal null after mint failure', async () => {
    const client = new FakeMintClient();
    client.nextResp = new Error('mint failed');
    const svc = new PluginAuthService(client);

    await expect(svc.acquire('c1', 'i1')).rejects.toThrow('mint failed');
    expect(svc.tokenSignal('c1', 'i1')()).toBeNull();
  });

  it('deduplicates concurrent acquire calls for the same key', async () => {
    const client = new FakeMintClient();
    const svc = new PluginAuthService(client);

    const [a, b] = await Promise.all([svc.acquire('c1', 'i1'), svc.acquire('c1', 'i1')]);

    expect(a.token).toBe('t1');
    expect(b.token).toBe('t1');
    expect(client.callCount).toBe(1);
  });

  it('does not arm a refresh timer when released while the mint is in flight', async () => {
    vi.useFakeTimers();
    try {
      const d = makeDeferredMint();
      const svc = new PluginAuthService(d.client);

      const p = svc.acquire('c1', 'i1'); // mint in flight
      svc.release('c1', 'i1'); // teardown before it resolves
      d.resolveNext({ token: 't1', expiresAt: inFuture(900) });
      await p;

      // Advance well past any refresh delay: no orphaned timer should re-mint.
      await vi.advanceTimersByTimeAsync(60 * 60 * 1000);
      expect(d.callCount()).toBe(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it('retries a failing refresh with 2s/4s/8s backoff before failing permanently', async () => {
    vi.useFakeTimers();
    try {
      const d = makeDeferredMint();
      const svc = new PluginAuthService(d.client);
      const failed = svc.failedSignal('c1', 'i1');

      const p = svc.acquire('c1', 'i1');
      d.resolveNext({ token: 't1', expiresAt: inFuture(10) }); // short TTL → refresh at ~8s
      await p;
      expect(d.callCount()).toBe(1);

      // Scheduled refresh (0.8 * 10s), then the 2s / 4s / 8s retry backoffs.
      const failNext = async (delayMs: number): Promise<void> => {
        await vi.advanceTimersByTimeAsync(delayMs);
        await flush();
        d.rejectNext(new Error('mint down'));
        await flush();
      };
      await failNext(8_000);
      await failNext(2_000);
      await failNext(4_000);
      await failNext(8_000);

      // 1 initial + 1 scheduled refresh + 3 retries = 5 mint attempts, then
      // permanent failure (the "8s" branch must have fired — with the old
      // off-by-one it gave up after 4).
      expect(d.callCount()).toBe(5);
      expect(failed()).toBe(true);
    } finally {
      vi.useRealTimers();
    }
  });
});
