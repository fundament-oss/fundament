import { DestroyRef } from '@angular/core';
import { installPollDelay } from './plugin-install-status';

export interface InstallPoller {
  /** Starts, re-paces or stops the poll to suit the installs shown. */
  pace(installs: readonly { phase: string; configInvalid: boolean }[]): void;
  /** Stops the poll until the next pace. */
  stop(): void;
}

/**
 * Calls `refresh` at the pace installPollDelay gives the installs a screen
 * shows: quickly while one is on its way up or down, slower while one is only
 * Degraded or Failed, and not at all once none can change on its own. Owns the
 * timer, and stops for good when the screen is destroyed, so a refresh that
 * returns after that cannot start it again. A tick is skipped while the
 * previous refresh is still out: a cluster that is slow to answer must not
 * stack reads, nor have an older one land over a newer one. `refresh` is
 * expected not to throw.
 */
export default function createInstallPoller(
  destroyRef: DestroyRef,
  refresh: () => Promise<void>,
): InstallPoller {
  let timer: ReturnType<typeof setInterval> | null = null;
  // The pace the timer runs at, so a change of pace restarts it.
  let delay: number | null = null;
  let destroyed = false;
  let inFlight = false;

  const tick = async () => {
    if (inFlight) return;
    inFlight = true;
    try {
      await refresh();
    } finally {
      inFlight = false;
    }
  };

  const stop = () => {
    if (timer) {
      clearInterval(timer);
      timer = null;
    }
    delay = null;
  };

  destroyRef.onDestroy(() => {
    destroyed = true;
    stop();
  });

  return {
    pace(installs) {
      if (destroyed) return;
      const next = installPollDelay(installs);
      if (next === delay) return;
      stop();
      if (next === null) return;
      delay = next;
      timer = setInterval(tick, next);
    },
    stop,
  };
}
