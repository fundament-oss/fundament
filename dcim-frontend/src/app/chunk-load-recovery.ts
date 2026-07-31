import { NavigationError } from '@angular/router';

// Browser-specific messages for a failed dynamic import() (Chrome, Firefox,
// Safari respectively). Thrown when a lazy route chunk no longer exists on the
// server, i.e. a deploy replaced the content-hashed bundles while this tab was
// still running the previous version of the app.
const CHUNK_LOAD_ERROR_PATTERN =
  /Failed to fetch dynamically imported module|error loading dynamically imported module|Importing a module script failed/i;

const RELOADED_AT_KEY = 'chunk-load-recovery:reloaded-at';

// Allow at most one forced reload per window, so a persistently broken server
// results in a normal navigation error instead of a reload loop.
const RELOAD_LOOP_WINDOW_MS = 30_000;

export const isChunkLoadError = (error: unknown): boolean =>
  error instanceof Error && CHUNK_LOAD_ERROR_PATTERN.test(error.message);

interface RecoveryDeps {
  storage: Storage;
  assign: (url: string) => void;
  now: () => number;
}

// Recover from a navigation that failed because its lazy chunk is gone: leave
// the stale app via a full page load of the url the user was navigating to.
// This only ever replaces a navigation the user already initiated, so no
// in-page state is lost that the route change would not have discarded anyway.
export const recoverFromChunkLoadError = (
  event: NavigationError,
  deps?: Partial<RecoveryDeps>,
): void => {
  const { storage, assign, now }: RecoveryDeps = {
    storage: window.sessionStorage,
    assign: (url) => window.location.assign(url),
    now: () => Date.now(),
    ...deps,
  };

  if (!isChunkLoadError(event.error)) {
    return;
  }

  try {
    const reloadedAt = Number(storage.getItem(RELOADED_AT_KEY) ?? 0);
    if (now() - reloadedAt < RELOAD_LOOP_WINDOW_MS) {
      return;
    }
    storage.setItem(RELOADED_AT_KEY, String(now()));
  } catch {
    // Storage being unavailable should not prevent recovery; without the loop
    // guard the worst case is periodic reloads while the server stays broken.
  }

  assign(event.url);
};
