import { afterEach, describe, expect, it, vi } from 'vitest';
import { TestBed } from '@angular/core/testing';
import { DefaultUrlSerializer, Navigation, Router } from '@angular/router';
import { Code, ConnectError, UnaryRequest } from '@connectrpc/connect';
import { staffSessionInterceptor } from './admin';
import StaffSessionService from '../app/admin-review/staff-session.service';

const serializer = new DefaultUrlSerializer();

interface Options {
  /** The page the last finished navigation landed on. */
  url: string;
  /** Where a navigation still running is going, if one is. */
  navigatingTo?: string;
}

function setup({ url, navigatingTo }: Options) {
  const navigate = vi.fn().mockResolvedValue(true);
  const navigation = navigatingTo
    ? ({ extractedUrl: serializer.parse(navigatingTo) } as Navigation)
    : null;
  const session = { hasSessionSurface: () => true, sessionEnded: vi.fn() };

  TestBed.configureTestingModule({
    providers: [
      {
        provide: Router,
        useValue: {
          url,
          navigate,
          currentNavigation: () => navigation,
          serializeUrl: (tree: Parameters<DefaultUrlSerializer['serialize']>[0]) =>
            serializer.serialize(tree),
        },
      },
      { provide: StaffSessionService, useValue: session },
    ],
  });
  const interceptor = TestBed.runInInjectionContext(() => staffSessionInterceptor());
  const call = interceptor(async () => {
    throw new ConnectError('no session', Code.Unauthenticated);
  });
  const fail = () => call({} as UnaryRequest);
  return { navigate, session, fail };
}

describe('staffSessionInterceptor', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('sends a refused reviewer to the login, marked as a refusal', async () => {
    const { navigate, session, fail } = setup({ url: '/admin/submissions/42' });

    // The caller still gets the error, whatever else happens.
    await expect(fail()).rejects.toMatchObject({ code: Code.Unauthenticated });

    expect(session.sessionEnded).toHaveBeenCalledOnce();
    expect(navigate).toHaveBeenCalledWith(['/login'], {
      queryParams: { returnUrl: '/admin/submissions/42', refused: 1 },
    });
  });

  // router.url is still the page before while the navigation runs.
  it('returns to the page being opened, not the one being left', async () => {
    const { navigate, fail } = setup({ url: '/admin', navigatingTo: '/admin/submissions/42' });

    await expect(fail()).rejects.toMatchObject({ code: Code.Unauthenticated });

    expect(navigate).toHaveBeenCalledWith(['/login'], {
      queryParams: { returnUrl: '/admin/submissions/42', refused: 1 },
    });
  });

  // Several calls failing at once: the first one's navigation to the login is
  // left to finish rather than cancelled by the next.
  it('leaves a navigation to the login already under way alone', async () => {
    const { navigate, fail } = setup({
      url: '/admin',
      navigatingTo: '/login?returnUrl=%2Fadmin&refused=1',
    });

    await expect(fail()).rejects.toMatchObject({ code: Code.Unauthenticated });

    expect(navigate).not.toHaveBeenCalled();
  });
});
