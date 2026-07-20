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

  // failed[key] flips true only after MAX_RETRIES mint failures. The iframe
  // effect uses it to distinguish "still awaiting initial mint" (both signals
  // null) from "mint permanently failed" (token null AND failed true) — the
  // former must NOT post fundament:auth-failed since a successful mint may
  // still be in flight.
  private readonly failed = new Map<string, ReturnType<typeof signal<boolean>>>();

  private readonly cache = new Map<string, TokenSnapshot>();

  private readonly inFlight = new Map<string, Promise<TokenSnapshot>>();

  private readonly timers = new Map<string, ReturnType<typeof setTimeout>>();

  // epochs[key] bumps on every release(key). A mint captures the epoch when it
  // starts and, on resolve, skips re-populating the maps / re-arming the timer
  // if it changed — otherwise a mint that resolves after release() resurrects a
  // refresh timer for a destroyed iframe. Never deleted: a missing entry would
  // default to 0 and defeat the guard.
  private readonly epochs = new Map<string, number>();

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

  // failedSignal returns a Signal<boolean> that flips true only after
  // MAX_RETRIES mint failures. Consumers should treat (token null AND
  // failed false) as "in flight, do nothing yet".
  failedSignal(clusterId: string, installationId: string): Signal<boolean> {
    const key = makeKey(clusterId, installationId);
    if (!this.failed.has(key)) {
      this.failed.set(key, signal<boolean>(false));
    }
    return this.failed.get(key)!.asReadonly();
  }

  private getOrCreateFailedSignal(key: string): ReturnType<typeof signal<boolean>> {
    if (!this.failed.has(key)) {
      this.failed.set(key, signal<boolean>(false));
    }
    return this.failed.get(key)!;
  }

  async acquire(clusterId: string, installationId: string): Promise<TokenSnapshot> {
    const key = makeKey(clusterId, installationId);

    // Return cached value if still valid (> 60s remaining)
    const cached = this.cache.get(key);
    if (cached && cached.expiresAt * 1000 > Date.now() + CACHE_MIN_REMAINING_MS) {
      return cached;
    }

    // Deduplicate concurrent callers (including an in-flight refresh mint).
    const inFlight = this.inFlight.get(key);
    if (inFlight) {
      return inFlight;
    }

    const epoch = this.epochOf(key);
    const promise = this.client.mint(clusterId, installationId).then(
      (snapshot) => {
        this.inFlight.delete(key);
        // released mid-mint (see epochs) — hand back the token but don't
        // repopulate state for a torn-down tuple.
        if (this.epochOf(key) !== epoch) return snapshot;
        this.cache.set(key, snapshot);
        this.getOrCreateSignal(key).set(snapshot);
        this.getOrCreateFailedSignal(key).set(false);
        this.scheduleRefresh(clusterId, installationId, snapshot, 0);
        return snapshot;
      },
      (err: unknown) => {
        this.inFlight.delete(key);
        // Initial-mint failures don't flip failedSignal — the caller can retry
        // via acquire() again. Only the doRefresh MAX_RETRIES path (below)
        // marks auth as permanently failed.
        throw err;
      },
    );

    this.inFlight.set(key, promise);
    return promise;
  }

  private epochOf(key: string): number {
    return this.epochs.get(key) ?? 0;
  }

  release(clusterId: string, installationId: string): void {
    const key = makeKey(clusterId, installationId);
    // Bump the epoch before clearing so an in-flight mint sees it and bails.
    this.epochs.set(key, this.epochOf(key) + 1);
    const timer = this.timers.get(key);
    if (timer !== undefined) {
      clearTimeout(timer);
      this.timers.delete(key);
    }
    // Drop the maps too. Without this, long-lived console sessions leak an
    // entry per (cluster, install) tuple ever visited, AND a subsequent
    // acquire() for the same tuple returns the stale cached snapshot (whose
    // refresh timer was just cancelled), so tokens silently expire without
    // being refreshed. Clearing cache/signals/inFlight ensures the next
    // acquire mints fresh and re-arms the refresh timer.
    this.cache.delete(key);
    this.signals.delete(key);
    this.failed.delete(key);
    this.inFlight.delete(key);
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
    const epoch = this.epochOf(key);

    // Register the refresh mint in inFlight so a concurrent acquire() (e.g. a
    // 401-driven requestRefresh) dedups onto it instead of starting a second
    // mint that races to set cache/signal/timer.
    const mintPromise = this.client.mint(clusterId, installationId);
    this.inFlight.set(key, mintPromise);

    mintPromise.then(
      (snapshot) => {
        if (this.inFlight.get(key) === mintPromise) this.inFlight.delete(key);
        // released while minting — don't repopulate or re-arm.
        if (this.epochOf(key) !== epoch) return;
        this.cache.set(key, snapshot);
        this.getOrCreateSignal(key).set(snapshot);
        this.getOrCreateFailedSignal(key).set(false);
        this.scheduleRefresh(clusterId, installationId, snapshot, 0);
      },
      () => {
        if (this.inFlight.get(key) === mintPromise) this.inFlight.delete(key);
        if (this.epochOf(key) !== epoch) return;
        const nextFailureCount = failureCount + 1;
        if (nextFailureCount > MAX_RETRIES) {
          // Persistent failure — mark auth as failed and clear the token.
          // The iframe effect posts fundament:auth-failed only when
          // failedSignal is true, distinguishing this from the initial
          // in-flight state.
          this.cache.delete(key);
          this.getOrCreateSignal(key).set(null);
          this.getOrCreateFailedSignal(key).set(true);
          this.timers.delete(key);
        } else {
          // Exponential backoff: 2s, 4s, 8s, capped at 30s.
          const backoffMs = Math.min(RETRY_DELAY_MS * 2 ** failureCount, RETRY_BACKOFF_MAX_MS);
          const timer = setTimeout(() => {
            this.doRefresh(clusterId, installationId, nextFailureCount);
          }, backoffMs);
          this.timers.set(key, timer);
        }
      },
    );
  }

  // requestRefresh is invoked when a plugin fetch returns 401 — meaning the
  // token in play has been rejected upstream. Cancels the pending timer
  // (which was scheduled off the token's expiresAt) and forces an immediate
  // out-of-band refresh. Returns the promise so callers that need to await
  // the new token can do so.
  requestRefresh(clusterId: string, installationId: string): Promise<TokenSnapshot> {
    const key = makeKey(clusterId, installationId);
    const timer = this.timers.get(key);
    if (timer !== undefined) {
      clearTimeout(timer);
      this.timers.delete(key);
    }
    // Reset failed so any waiting effect sees the retry attempt. The next
    // MAX_RETRIES failures via doRefresh will re-flip it if the mint really
    // is broken.
    this.getOrCreateFailedSignal(key).set(false);
    // Clear the cache so acquire() cannot return the just-rejected token.
    this.cache.delete(key);
    return this.acquire(clusterId, installationId);
  }
}
