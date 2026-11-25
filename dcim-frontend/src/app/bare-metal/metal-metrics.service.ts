import { inject, Injectable, signal } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';
import { firstValueFrom } from 'rxjs';
import { METAL_CLIENT } from '../../connect/tokens';
import connectErrorMessage from '../../connect/error';
import type { MetalMetrics } from './metal-metrics';
import metalFromProto from './metal-from-proto';

/** Where the read stands. `not-configured`: this environment has no
 *  metal-stack metrics. `unavailable`: it has, but they cannot be read now. */
export type MetalStatus = 'loading' | 'ok' | 'not-configured' | 'unavailable';

/**
 * What metal-stack says about the estate, read through dcim-api, which queries
 * the store metal-stack's exporters are scraped into. The page and the sidebar
 * badge follow it because it is a signal.
 *
 * The demo provides its own, the way it does for the config, for who you are
 * and for rack history.
 */
@Injectable({ providedIn: 'root' })
export default class MetalMetricsService {
  private readonly client = inject(METAL_CLIENT);

  private readonly value = signal<MetalMetrics | null>(null);

  private readonly state = signal<MetalStatus>('loading');

  /** The last snapshot, or null while there is none. A failed read keeps it:
   *  the page says when it was collected. */
  readonly snapshot = this.value.asReadonly();

  readonly status = this.state.asReadonly();

  private pending = false;

  /** Reads the metrics again, unless a read is already on its way. The data
   *  changes every minute or two, so a second read in flight adds nothing. */
  refresh(): void {
    if (this.pending) return;
    this.pending = true;
    firstValueFrom(this.client.getMetalSnapshot({}))
      .then((res) => {
        this.value.set(metalFromProto(res));
        this.state.set('ok');
      })
      .catch((err: unknown) => {
        const code = ConnectError.from(err).code;
        this.state.set(code === Code.FailedPrecondition ? 'not-configured' : 'unavailable');
        if (code !== Code.FailedPrecondition) {
          // eslint-disable-next-line no-console
          console.error(connectErrorMessage(err));
        }
      })
      .finally(() => {
        this.pending = false;
      });
  }

  /** For a demo build to fill the snapshot. */
  protected setSnapshot(metrics: MetalMetrics | null, status: MetalStatus): void {
    this.value.set(metrics);
    this.state.set(status);
  }
}
