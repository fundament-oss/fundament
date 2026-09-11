import { Injectable, inject, signal, effect, DestroyRef } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { firstValueFrom } from 'rxjs';
import { METRICS } from '../connect/tokens';
import { GetOrgWorkloadMetricsRequestSchema } from '../generated/v1/metrics_pb';
import OrganizationContextService from './organization-context.service';

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

  private organizationContext = inject(OrganizationContextService);

  private destroyRef = inject(DestroyRef);

  /** 'unknown' until the first answer: a badge on a guess is worse than none. */
  state = signal<'unknown' | 'ok' | 'down'>('unknown');

  private timer: ReturnType<typeof setInterval> | null = null;

  private checking = false;

  /**
   * Signing in and having an organization are two moments, and the first comes
   * first: the call carries the Fun-Organization header, which the backend
   * rejects the request without. So the poller starts at sign-in but stays
   * quiet until the organization it would ask about is known.
   */
  private readonly checkOnOrganization = effect(() => {
    const organizationId = this.organizationContext.currentOrganizationId();
    if (organizationId && this.timer) this.check();
  });

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
    // No organization selected yet: asking would be answered with a 400, and a
    // badge reading "down" over a missing header would be a lie. The effect
    // above asks again the moment one is selected.
    if (!this.organizationContext.currentOrganizationId()) return;
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
