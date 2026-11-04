import { DestroyRef } from '@angular/core';
import { vi } from 'vitest';
import createInstallPoller from './install-poller';

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

// installPollDelay's pace while an installation is on its way.
const IN_PROGRESS_MS = 5000;

const installing = [{ phase: 'Pending', configInvalid: false }];

describe('createInstallPoller', () => {
  beforeEach(() => vi.useFakeTimers());

  afterEach(() => vi.useRealTimers());

  it('refreshes at the pace the installs need, and stops once none can change', async () => {
    const refresh = vi.fn(async () => {});
    const { destroyRef, destroy } = fakeDestroyRef();
    const poller = createInstallPoller(destroyRef, refresh);

    poller.pace(installing);
    await vi.advanceTimersByTimeAsync(IN_PROGRESS_MS * 2);
    expect(refresh).toHaveBeenCalledTimes(2);

    poller.pace([{ phase: 'Running', configInvalid: false }]);
    await vi.advanceTimersByTimeAsync(IN_PROGRESS_MS * 2);
    expect(refresh).toHaveBeenCalledTimes(2);
    destroy();
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
    createInstallPoller(destroyRef, refresh).pace(installing);

    await vi.advanceTimersByTimeAsync(IN_PROGRESS_MS * 3);
    expect(refresh).toHaveBeenCalledTimes(1);

    finish();
    await vi.advanceTimersByTimeAsync(IN_PROGRESS_MS);
    expect(refresh).toHaveBeenCalledTimes(2);
    destroy();
  });

  it('does not start again once the screen is destroyed', async () => {
    const refresh = vi.fn(async () => {});
    const { destroyRef, destroy } = fakeDestroyRef();
    const poller = createInstallPoller(destroyRef, refresh);

    destroy();
    poller.pace(installing);
    await vi.advanceTimersByTimeAsync(IN_PROGRESS_MS * 2);
    expect(refresh).not.toHaveBeenCalled();
  });
});
