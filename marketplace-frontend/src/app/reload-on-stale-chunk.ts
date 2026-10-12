// Kept identical in console-frontend, dcim-frontend and marketplace-frontend:
// a change here belongs in the other two as well.
import { isPlatformBrowser, LocationStrategy } from '@angular/common';
import {
  type EnvironmentProviders,
  ErrorHandler,
  inject,
  Injectable,
  makeEnvironmentProviders,
  PLATFORM_ID,
  provideEnvironmentInitializer,
} from '@angular/core';
import { NavigationError, Router } from '@angular/router';

// The build a page runs, named by its hashed main bundle. Undefined under
// `ng serve`, whose bundle carries no hash.
const MAIN_BUNDLE = /(?:^|\/)main-[^/]+\.js$/;

export const buildOf = (page: Document): string | undefined =>
  Array.from(page.scripts, (script) => script.getAttribute('src') ?? '').find((src) =>
    MAIN_BUNDLE.test(src),
  );

// The build the server hands out now. Every route answers with the app's page,
// so the base URL does.
async function deployedBuild(): Promise<string | undefined> {
  const response = await fetch(document.baseURI, { cache: 'no-store' });
  if (!response.ok) {
    return undefined;
  }
  return buildOf(new DOMParser().parseFromString(await response.text(), 'text/html'));
}

// The builds this tab has reloaded away from. During a rollout old and new pods
// serve side by side, so a reload can land on the build it just left; leaving
// each build at most once keeps the tab from bouncing between the two.
const LEFT_BUILDS = 'stale-chunk-reload-left';

const leftBefore = (build: string): boolean => {
  try {
    const left = (sessionStorage.getItem(LEFT_BUILDS) ?? '').split(' ').filter(Boolean);
    if (left.includes(build)) {
      return true;
    }
    sessionStorage.setItem(LEFT_BUILDS, [...left, build].slice(-10).join(' '));
    return false;
  } catch {
    // Without storage, the comparison with the deployed build still keeps a
    // missing chunk from reloading the page forever.
    return false;
  }
};

// One look at the server per interval at most: a failed navigation also reaches
// the ErrorHandler, and a bug that throws on every change detection would
// otherwise fetch the page over and over.
const CHECK_INTERVAL_MS = 10_000;

/**
 * Calls `reload` only when the server hands out a different build than this
 * tab runs. A chunk missing from the current build is a bug a reload cannot
 * fix, and reloading for it would never end.
 */
export const staleChunkCheck = () => {
  let checkedAt = -Infinity;
  return async (reload: () => void): Promise<void> => {
    const running = buildOf(document);
    if (!running || Date.now() - checkedAt < CHECK_INTERVAL_MS) {
      return;
    }
    checkedAt = Date.now();
    try {
      const deployed = await deployedBuild();
      if (deployed && deployed !== running && !leftBefore(running)) {
        reload();
      }
    } catch {
      // No answer from the server, so nothing says the build has changed.
    }
  };
};

const reloadIfStale = staleChunkCheck();

// What a chunk that failed to load looks like. A dynamic import rejects with a
// TypeError in every browser, whatever the wording. A @defer block without an
// @error block reports NG0750 instead and drops the TypeError. Either can also
// be an ordinary bug: staleChunkCheck tells the two apart.
const DEFER_LOADING_FAILED = -750;

const mayBeMissingChunk = (error: unknown): boolean =>
  error instanceof TypeError ||
  (typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    error.code === DEFER_LOADING_FAILED);

// @defer blocks and plain import() calls: reload the page as it is.
@Injectable()
class StaleChunkErrorHandler extends ErrorHandler {
  private readonly browser = isPlatformBrowser(inject(PLATFORM_ID));

  override handleError(error: unknown): void {
    super.handleError(error);
    if (this.browser && mayBeMissingChunk(error)) {
      reloadIfStale(() => window.location.reload());
    }
  }
}

/**
 * A redeploy replaces the hashed chunk files, so a tab still running the
 * previous build can no longer load a lazy route, a @defer block or anything
 * else it imports on demand: the file it asks for is gone. Instead of leaving
 * the tab broken, load the page fresh, which picks up the current build.
 *
 * A failed navigation reloads at the URL it was headed for. Under hash routing
 * that URL differs only in its hash, which the browser does not reload for, so
 * this is for path routing only.
 */
const provideStaleChunkReload = (): EnvironmentProviders =>
  makeEnvironmentProviders([
    { provide: ErrorHandler, useClass: StaleChunkErrorHandler },
    provideEnvironmentInitializer(() => {
      if (!isPlatformBrowser(inject(PLATFORM_ID))) {
        return;
      }
      const locationStrategy = inject(LocationStrategy);
      inject(Router).events.subscribe((event) => {
        if (event instanceof NavigationError && mayBeMissingChunk(event.error)) {
          reloadIfStale(() =>
            window.location.assign(locationStrategy.prepareExternalUrl(event.url)),
          );
        }
      });
    }),
  ]);

export default provideStaleChunkReload;
