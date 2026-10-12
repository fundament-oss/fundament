import { LocationStrategy } from '@angular/common';
import { inject } from '@angular/core';
import { type NavigationError, withNavigationErrorHandler } from '@angular/router';

// How Chromium, Firefox and Safari word a dynamic import that failed to load.
const FAILED_IMPORT = /dynamically imported module|Importing a module script failed/i;

// When this tab last reloaded for a stale chunk. A chunk that is missing for any
// other reason would otherwise reload the page forever.
const RELOADED_AT = 'reloaded-for-stale-chunk-at';
const RELOAD_COOLDOWN_MS = 10_000;

const reloadedRecently = (): boolean => {
  try {
    const now = Date.now();
    const recently = now - Number(sessionStorage.getItem(RELOADED_AT)) < RELOAD_COOLDOWN_MS;
    sessionStorage.setItem(RELOADED_AT, String(now));
    return recently;
  } catch {
    // Without storage there is no telling a first attempt from a loop.
    return true;
  }
};

/**
 * A redeploy replaces the hashed chunk files, so a tab still running the
 * previous build can no longer load a lazy route: the chunk it asks for is
 * gone. Instead of leaving the navigation dead, load its URL fresh, which
 * picks up the current build.
 *
 * Path routing only: under hash routing the new URL differs only in its hash,
 * which the browser does not reload for.
 */
const reloadOnStaleChunk = () =>
  withNavigationErrorHandler((navigationError: NavigationError) => {
    if (!FAILED_IMPORT.test(String(navigationError.error)) || reloadedRecently()) {
      return;
    }
    window.location.assign(inject(LocationStrategy).prepareExternalUrl(navigationError.url));
  });

export default reloadOnStaleChunk;
