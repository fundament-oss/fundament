import { describe, it, expect } from 'vitest';
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
});
