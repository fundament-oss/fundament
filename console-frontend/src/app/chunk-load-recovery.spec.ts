import { NavigationError } from '@angular/router';
import { isChunkLoadError, recoverFromChunkLoadError } from './chunk-load-recovery';

const fakeStorage = (): Storage => {
  const items = new Map<string, string>();
  return {
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => {
      items.set(key, value);
    },
    removeItem: (key: string) => {
      items.delete(key);
    },
    clear: () => items.clear(),
    key: () => null,
    get length() {
      return items.size;
    },
  };
};

const navigationError = (error: unknown, url = '/clusters/add'): NavigationError =>
  ({ id: 1, url, error }) as NavigationError;

describe('isChunkLoadError', () => {
  it('detects Chrome dynamic import failures', () => {
    expect(
      isChunkLoadError(
        new TypeError('Failed to fetch dynamically imported module: https://x/chunk-abc.js'),
      ),
    ).toBe(true);
  });

  it('detects Firefox dynamic import failures', () => {
    expect(isChunkLoadError(new TypeError('error loading dynamically imported module'))).toBe(true);
  });

  it('detects Safari dynamic import failures', () => {
    expect(isChunkLoadError(new TypeError('Importing a module script failed.'))).toBe(true);
  });

  it('ignores unrelated errors', () => {
    expect(isChunkLoadError(new Error('route guard rejected'))).toBe(false);
  });

  it('ignores non-Error values', () => {
    expect(isChunkLoadError('Failed to fetch dynamically imported module')).toBe(false);
    expect(isChunkLoadError(undefined)).toBe(false);
  });
});

describe('recoverFromChunkLoadError', () => {
  const chunkError = () =>
    new TypeError('Failed to fetch dynamically imported module: https://x/chunk-abc.js');

  it('does a full page navigation to the intended url on a chunk load error', () => {
    const assign = vi.fn();
    recoverFromChunkLoadError(navigationError(chunkError(), '/clusters/add'), {
      storage: fakeStorage(),
      assign,
      now: () => 1_000_000,
    });
    expect(assign).toHaveBeenCalledExactlyOnceWith('/clusters/add');
  });

  it('does not navigate on unrelated navigation errors', () => {
    const assign = vi.fn();
    recoverFromChunkLoadError(navigationError(new Error('boom')), {
      storage: fakeStorage(),
      assign,
      now: () => 1_000_000,
    });
    expect(assign).not.toHaveBeenCalled();
  });

  it('does not reload again within the loop guard window', () => {
    const assign = vi.fn();
    const storage = fakeStorage();
    recoverFromChunkLoadError(navigationError(chunkError()), {
      storage,
      assign,
      now: () => 1_000_000,
    });
    recoverFromChunkLoadError(navigationError(chunkError()), {
      storage,
      assign,
      now: () => 1_000_000 + 5_000,
    });
    expect(assign).toHaveBeenCalledTimes(1);
  });

  it('reloads again once the loop guard window has passed', () => {
    const assign = vi.fn();
    const storage = fakeStorage();
    recoverFromChunkLoadError(navigationError(chunkError()), {
      storage,
      assign,
      now: () => 1_000_000,
    });
    recoverFromChunkLoadError(navigationError(chunkError()), {
      storage,
      assign,
      now: () => 1_000_000 + 60_000,
    });
    expect(assign).toHaveBeenCalledTimes(2);
  });

  it('still navigates when session storage is unavailable', () => {
    const assign = vi.fn();
    const broken = {
      getItem: () => {
        throw new Error('storage disabled');
      },
      setItem: () => {
        throw new Error('storage disabled');
      },
    } as unknown as Storage;
    recoverFromChunkLoadError(navigationError(chunkError(), '/clusters/add'), {
      storage: broken,
      assign,
      now: () => 1_000_000,
    });
    expect(assign).toHaveBeenCalledExactlyOnceWith('/clusters/add');
  });
});
