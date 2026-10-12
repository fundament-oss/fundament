import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { buildOf, staleChunkCheck } from './reload-on-stale-chunk';

/**
 * The page reloads for a failed chunk only when the server hands out another
 * build than the tab runs: that is what keeps a chunk missing from the current
 * build from reloading the page forever.
 */

const page = (main: string) =>
  `<!doctype html><html><head><script src="polyfills-P.js" type="module"></script>` +
  `<script src="${main}" type="module"></script></head><body></body></html>`;

function serve(html: string, status = 200) {
  const fetch = vi.fn(async () => new Response(html, { status }));
  vi.stubGlobal('fetch', fetch);
  return fetch;
}

describe('staleChunkCheck', () => {
  let running: HTMLScriptElement;

  beforeEach(() => {
    sessionStorage.clear();
    running = document.createElement('script');
    // Not JavaScript, so the test DOM does not try to load it.
    running.type = 'text/plain';
    running.setAttribute('src', 'main-OLD.js');
    document.head.append(running);
  });

  afterEach(() => {
    running.remove();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('names a page by its hashed main bundle', () => {
    expect(buildOf(document)).toBe('main-OLD.js');
    running.setAttribute('src', 'main.js');
    expect(buildOf(document)).toBeUndefined();
  });

  it('reloads when the server hands out another build', async () => {
    const fetch = serve(page('main-NEW.js'));
    const reload = vi.fn();

    await staleChunkCheck()(reload);

    expect(fetch).toHaveBeenCalledWith(document.baseURI, { cache: 'no-store' });
    expect(reload).toHaveBeenCalledOnce();
  });

  it('leaves the page alone when the server hands out the same build', async () => {
    serve(page('main-OLD.js'));
    const reload = vi.fn();

    await staleChunkCheck()(reload);

    expect(reload).not.toHaveBeenCalled();
  });

  it('leaves the page alone when the server cannot say', async () => {
    const reload = vi.fn();

    serve('Bad gateway', 502);
    await staleChunkCheck()(reload);
    vi.stubGlobal('fetch', vi.fn(async () => Promise.reject(new TypeError('Failed to fetch'))));
    await staleChunkCheck()(reload);

    expect(reload).not.toHaveBeenCalled();
  });

  it('does nothing under ng serve, whose bundle has no hash', async () => {
    running.setAttribute('src', 'main.js');
    const fetch = serve(page('main-NEW.js'));

    await staleChunkCheck()(vi.fn());

    expect(fetch).not.toHaveBeenCalled();
  });

  it('reloads away from a build once at most, so a rollout cannot bounce the tab', async () => {
    serve(page('main-NEW.js'));
    const reload = vi.fn();

    await staleChunkCheck()(reload);
    await staleChunkCheck()(reload);

    expect(reload).toHaveBeenCalledOnce();
  });

  it('asks the server once per ten seconds at most', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    const fetch = serve(page('main-OLD.js'));
    const check = staleChunkCheck();

    await check(vi.fn());
    await check(vi.fn());
    expect(fetch).toHaveBeenCalledOnce();

    vi.advanceTimersByTime(10_000);
    await check(vi.fn());
    expect(fetch).toHaveBeenCalledTimes(2);
  });
});
