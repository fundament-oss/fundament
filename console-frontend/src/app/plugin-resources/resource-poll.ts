import { DestroyRef } from '@angular/core';

/** Often enough that a Ready condition turning True shows up while you wait
 *  for it, the same pace the plugin pages poll an install at. */
export const RESOURCE_POLL_MS = 5000;

/**
 * Calls `refresh` every RESOURCE_POLL_MS for as long as the page lives, so a
 * plugin resource's status follows the cluster without a reload. A tick is
 * skipped while the tab is hidden or the previous refresh is still out, and a
 * tab that is looked at again refreshes straight away: a backgrounded tab gets
 * no timers worth trusting. `refresh` decides itself whether there is anything
 * to refresh yet, and is expected not to throw.
 */
export default function pollResources(destroyRef: DestroyRef, refresh: () => Promise<void>): void {
  let inFlight = false;
  const tick = async () => {
    if (inFlight || document.visibilityState === 'hidden') return;
    inFlight = true;
    try {
      await refresh();
    } finally {
      inFlight = false;
    }
  };

  const timer = setInterval(tick, RESOURCE_POLL_MS);
  const onVisible = () => {
    if (document.visibilityState === 'visible') tick();
  };
  document.addEventListener('visibilitychange', onVisible);

  destroyRef.onDestroy(() => {
    clearInterval(timer);
    document.removeEventListener('visibilitychange', onVisible);
  });
}
