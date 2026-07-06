import { Inject, Injectable, InjectionToken, Optional, inject, signal, type Signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { TOKEN } from '../../connect/tokens';

export interface TokenSnapshot {
  token: string;
  expiresAt: number; // unix seconds
}

export interface MintClient {
  mint(clusterId: string, installationId: string): Promise<TokenSnapshot>;
}

// MintClient is a TypeScript interface, so Angular's DI can't resolve it by
// type — it needs a runtime token. Tests bypass DI entirely by passing the
// fake to `new PluginAuthService(fake)`; in Angular the `@Inject(MINT_CLIENT)`
// slot is unfilled by default (no provider), and the constructor builds the
// production client from the injected TokenService.
export const MINT_CLIENT = new InjectionToken<MintClient>('PluginAuth.MintClient');

const REFRESH_RATIO = 0.8;

const RETRY_DELAY_MS = 2_000;

const RETRY_BACKOFF_MAX_MS = 30_000;

const MAX_RETRIES = 3;

const MIN_REFRESH_MS = 1_000;

const CACHE_MIN_REMAINING_MS = 60_000;

function makeKey(clusterId: string, installationId: string): string {
  return `${clusterId}::${installationId}`;
}

@Injectable({ providedIn: 'root' })
export class PluginAuthService {
  private readonly client: MintClient;

  private readonly signals = new Map<string, ReturnType<typeof signal<TokenSnapshot | null>>>();

  private readonly cache = new Map<string, TokenSnapshot>();

  private readonly inFlight = new Map<string, Promise<TokenSnapshot>>();

  private readonly timers = new Map<string, ReturnType<typeof setTimeout>>();

  // eslint-disable-next-line @angular-eslint/prefer-inject
  constructor(@Optional() @Inject(MINT_CLIENT) overrideClient?: MintClient) {
    if (overrideClient) {
      this.client = overrideClient;
    } else {
      const tokenClient = inject(TOKEN);
      this.client = {
        mint(clusterId: string, installationId: string): Promise<TokenSnapshot> {
          return firstValueFrom(tokenClient.mintPluginToken({ clusterId, installationId })).then(
            (resp) => ({
              token: resp.accessToken,
              expiresAt: Math.floor(Date.now() / 1000) + Number(resp.expiresIn),
            }),
          );
        },
      };
    }
  }

  tokenSignal(clusterId: string, installationId: string): Signal<TokenSnapshot | null> {
    const key = makeKey(clusterId, installationId);
    if (!this.signals.has(key)) {
      this.signals.set(key, signal<TokenSnapshot | null>(null));
    }
    return this.signals.get(key)!.asReadonly();
  }

  async acquire(clusterId: string, installationId: string): Promise<TokenSnapshot> {
    const key = makeKey(clusterId, installationId);

    // Return cached value if still valid (> 60s remaining)
    const cached = this.cache.get(key);
    if (cached && cached.expiresAt * 1000 > Date.now() + CACHE_MIN_REMAINING_MS) {
      return cached;
    }

    // Deduplicate concurrent callers
    const inFlight = this.inFlight.get(key);
    if (inFlight) {
      return inFlight;
    }

    const promise = this.client.mint(clusterId, installationId).then(
      (snapshot) => {
        this.cache.set(key, snapshot);
        this.getOrCreateSignal(key).set(snapshot);
        this.scheduleRefresh(clusterId, installationId, snapshot, 0);
        this.inFlight.delete(key);
        return snapshot;
      },
      (err: unknown) => {
        this.inFlight.delete(key);
        throw err;
      },
    );

    this.inFlight.set(key, promise);
    return promise;
  }

  release(clusterId: string, installationId: string): void {
    const key = makeKey(clusterId, installationId);
    const timer = this.timers.get(key);
    if (timer !== undefined) {
      clearTimeout(timer);
      this.timers.delete(key);
    }
  }

  private getOrCreateSignal(key: string): ReturnType<typeof signal<TokenSnapshot | null>> {
    if (!this.signals.has(key)) {
      this.signals.set(key, signal<TokenSnapshot | null>(null));
    }
    return this.signals.get(key)!;
  }

  private scheduleRefresh(
    clusterId: string,
    installationId: string,
    snapshot: TokenSnapshot,
    retryCount: number,
  ): void {
    const key = makeKey(clusterId, installationId);

    // Clear any existing timer
    const existing = this.timers.get(key);
    if (existing !== undefined) {
      clearTimeout(existing);
    }

    const nowSec = Math.floor(Date.now() / 1000);
    const remainingMs = (snapshot.expiresAt - nowSec) * 1000;
    const delayMs = Math.max(MIN_REFRESH_MS, remainingMs * REFRESH_RATIO);

    const timer = setTimeout(() => {
      this.doRefresh(clusterId, installationId, retryCount);
    }, delayMs);

    this.timers.set(key, timer);
  }

  private doRefresh(clusterId: string, installationId: string, failureCount: number): void {
    const key = makeKey(clusterId, installationId);

    this.client.mint(clusterId, installationId).then(
      (snapshot) => {
        this.cache.set(key, snapshot);
        this.getOrCreateSignal(key).set(snapshot);
        this.scheduleRefresh(clusterId, installationId, snapshot, 0);
      },
      () => {
        const nextFailureCount = failureCount + 1;
        if (nextFailureCount >= MAX_RETRIES) {
          // Persistent failure — signal null so iframe learns auth failed
          this.cache.delete(key);
          this.getOrCreateSignal(key).set(null);
          this.timers.delete(key);
        } else {
          // Exponential backoff: 2s, 4s, 8s, capped at 30s
          const backoffMs = Math.min(RETRY_DELAY_MS * 2 ** failureCount, RETRY_BACKOFF_MAX_MS);
          const timer = setTimeout(() => {
            this.doRefresh(clusterId, installationId, nextFailureCount);
          }, backoffMs);
          this.timers.set(key, timer);
        }
      },
    );
  }
}
