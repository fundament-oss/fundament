import { DestroyRef } from '@angular/core';
import { vi } from 'vitest';
import pollResources, { RESOURCE_POLL_MS } from './resource-poll';

function fakeDestroyRef() {
  const callbacks: (() => void)[] = [];
  const destroyRef = {
    onDestroy: (callback: () => void) => {
      callbacks.push(callback);
      return () => {};
    },
  } as unknown as DestroyRef;
  return { destroyRef, destroy: () => callbacks.forEach((callback) => callback()) };
}

function setVisibility(state: DocumentVisibilityState) {
  vi.spyOn(document, 'visibilityState', 'get').mockReturnValue(state);
  document.dispatchEvent(new Event('visibilitychange'));
}

describe('pollResources', () => {
  beforeEach(() => vi.useFakeTimers());

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('refreshes on every interval until the page is destroyed', async () => {
    const refresh = vi.fn(async () => {});
    const { destroyRef, destroy } = fakeDestroyRef();
    pollResources(destroyRef, refresh);

    await vi.advanceTimersByTimeAsync(RESOURCE_POLL_MS * 2);
    expect(refresh).toHaveBeenCalledTimes(2);

    destroy();
    await vi.advanceTimersByTimeAsync(RESOURCE_POLL_MS * 2);
    expect(refresh).toHaveBeenCalledTimes(2);
  });

  it('waits for a refresh that is still out instead of stacking another', async () => {
    let finish = () => {};
    const refresh = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    const { destroyRef, destroy } = fakeDestroyRef();
    pollResources(destroyRef, refresh);

    await vi.advanceTimersByTimeAsync(RESOURCE_POLL_MS * 3);
    expect(refresh).toHaveBeenCalledTimes(1);

    finish();
    await vi.advanceTimersByTimeAsync(RESOURCE_POLL_MS);
    expect(refresh).toHaveBeenCalledTimes(2);
    destroy();
  });

  it('skips a hidden tab and refreshes as soon as it is looked at again', async () => {
    const refresh = vi.fn(async () => {});
    const { destroyRef, destroy } = fakeDestroyRef();
    pollResources(destroyRef, refresh);

    setVisibility('hidden');
    await vi.advanceTimersByTimeAsync(RESOURCE_POLL_MS * 2);
    expect(refresh).not.toHaveBeenCalled();

    setVisibility('visible');
    await vi.advanceTimersByTimeAsync(0);
    expect(refresh).toHaveBeenCalledTimes(1);
    destroy();
  });
});
