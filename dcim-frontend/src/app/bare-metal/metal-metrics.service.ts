import { Injectable, signal } from '@angular/core';
import type { MetalMetrics } from './metal-metrics';

/**
 * What metal-stack says about the estate.
 *
 * Nothing, in the real app: the DCIM frontend has no Prometheus read yet, so
 * the section says metal-stack is not connected. This is the seam where that
 * read lands. A query against the metal-stack Prometheus fills the snapshot,
 * the page and the sidebar badge follow it because it is a signal, and the
 * hand-written types in metal-metrics.ts are already shaped after the series.
 *
 * The demo provides its own, the way it does for the config, for who you are
 * and for rack history.
 */
@Injectable({ providedIn: 'root' })
export default class MetalMetricsService {
  private readonly value = signal<MetalMetrics | null>(null);

  /** The last snapshot, or null while there is none. */
  readonly snapshot = this.value.asReadonly();

  /** Reads the metrics again. A no-op until the Prometheus read lands. */
  readonly refresh = (): void => {
    // Nothing to read: see the class comment.
  };

  /** For a demo build to fill the snapshot. The real implementation sets it
   *  from the answer to its query instead. */
  protected setSnapshot(metrics: MetalMetrics | null): void {
    this.value.set(metrics);
  }
}
