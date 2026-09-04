import { Injectable, inject, signal, DestroyRef } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { METRICS } from '../connect/tokens';
import { GetOrgWorkloadMetricsRequestSchema } from '../generated/v1/metrics_pb';

/** How often to ask, when nobody is watching the metrics page. */
const POLL_MS = 60_000;

/**
 * Whether the metrics backend answers, kept outside the metrics page.
 *
 * The page knows the moment its stream drops, but it tears that stream down when
 * you leave, so the knowledge would leave with it: a badge in the menu would go
 * out while the backend was still down. A cheap unary call on a slow timer keeps
 * the answer available everywhere, and the page reports what it sees so the
 * badge does not wait for the next tick.
 */
@Injectable({ providedIn: 'root' })
export default class MetricsHealthService {
  private metricsClient = inject(METRICS);

  private destroyRef = inject(DestroyRef);

  /** 'unknown' until the first answer: a badge on a guess is worse than none. */
  state = signal<'unknown' | 'ok' | 'down'>('unknown');

  private timer: ReturnType<typeof setInterval> | null = null;

  private checking = false;

  start(): void {
    if (this.timer) return;

    this.check();
    this.timer = setInterval(() => this.check(), POLL_MS);

    // A backgrounded tab gets no timers worth trusting, so ask again as soon as
    // it is looked at.
    const onVisible = () => {
      if (document.visibilityState === 'visible') this.check();
    };
    document.addEventListener('visibilitychange', onVisible);

    this.destroyRef.onDestroy(() => {
      document.removeEventListener('visibilitychange', onVisible);
      if (this.timer) clearInterval(this.timer);
      this.timer = null;
    });
  }

  /** What the metrics page already knows from its own stream. */
  report(healthy: boolean): void {
    this.state.set(healthy ? 'ok' : 'down');
  }

  private async check(): Promise<void> {
    if (this.checking || document.visibilityState === 'hidden') return;
    this.checking = true;
    try {
      await firstValueFrom(
        this.metricsClient.getOrgWorkloadMetrics(create(GetOrgWorkloadMetricsRequestSchema, {})),
      );
      this.state.set('ok');
    } catch {
      this.state.set('down');
    } finally {
      this.checking = false;
    }
  }
}
