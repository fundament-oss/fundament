import { TestBed } from '@angular/core/testing';
import { of } from 'rxjs';
import { vi } from 'vitest';
import AuthnApiService from './authn-api.service';
import { AUTHN } from '../connect/tokens';
import { ConfigService } from './config.service';

/**
 * A fetch that answers only when the test lets it: `sent` is every path asked
 * for, in the order it went out, and `answer` lets the oldest one still waiting
 * through with that status. Whether a request went out before another was
 * answered is the whole question, so nothing answers on its own. An aborted
 * request fails the way fetch does and stops waiting.
 */
function heldFetch() {
  const sent: string[] = [];
  const waiting: ((status: number) => void)[] = [];
  const fetch = vi.fn(
    (request: Request) =>
      new Promise<Response>((resolve, reject) => {
        sent.push(new URL(request.url).pathname);
        const answer = (status: number) =>
          resolve(
            new Response(status === 200 ? '{}' : '{"error":"Unauthorized"}', {
              status,
              headers: { 'Content-Type': 'application/json' },
            }),
          );
        waiting.push(answer);
        request.signal.addEventListener('abort', () => {
          waiting.splice(waiting.indexOf(answer), 1);
          reject(request.signal.reason);
        });
      }),
  );
  return { fetch, sent, answer: (status = 200) => waiting.shift()!(status) };
}

/** Lets every request that is free to go out do so. */
const settle = () =>
  new Promise((resolve) => {
    setTimeout(resolve);
  });

describe('AuthnApiService', () => {
  let held: ReturnType<typeof heldFetch>;
  let service: AuthnApiService;

  beforeEach(() => {
    held = heldFetch();
    vi.stubGlobal('fetch', held.fetch);
    TestBed.configureTestingModule({
      providers: [
        { provide: AUTHN, useValue: { getUserInfo: () => of({ user: undefined }) } },
        {
          provide: ConfigService,
          useValue: { getConfig: () => ({ authnApiUrl: 'https://authn.test' }) },
        },
      ],
    });
    service = TestBed.inject(AuthnApiService);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  // Both answers set the auth cookie and the last one to land wins: a refresh
  // answered after the logout would sign the user back in.
  it('sends a logout only once the refresh under way has been answered', async () => {
    const refreshing = service.refreshToken();
    await settle();
    const loggingOut = service.logout();
    await settle();

    expect(held.sent).toEqual(['/refresh']);

    held.answer();
    await refreshing;
    await settle();

    expect(held.sent).toEqual(['/refresh', '/logout']);

    held.answer();
    await loggingOut;
  });

  // The logout has taken the cookie away by then, so that refresh is refused.
  it('sends a refresh only once the logout under way has been answered', async () => {
    const loggingOut = service.logout();
    await settle();
    const refreshing = service.refreshToken();
    await settle();

    expect(held.sent).toEqual(['/logout']);

    held.answer();
    await loggingOut;
    await settle();

    expect(held.sent).toEqual(['/logout', '/refresh']);

    held.answer(401);
    await expect(refreshing).rejects.toThrow('Unauthorized');
  });

  it('sends a login only once the refresh under way has been answered', async () => {
    const refreshing = service.refreshToken();
    await settle();
    const loggingIn = service.login('alice@acme-corp.com', 'password');
    await settle();

    expect(held.sent).toEqual(['/refresh']);

    held.answer();
    await refreshing;
    await settle();

    expect(held.sent).toEqual(['/refresh', '/login/password']);

    held.answer();
    await loggingIn;
  });

  // A refresh that never answers would otherwise keep the logout from ever
  // going out.
  it('aborts a request that does not answer, and sends the next', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    const refreshing = service.refreshToken();
    const refused = expect(refreshing).rejects.toThrow();
    await vi.advanceTimersByTimeAsync(0);
    const loggingOut = service.logout();
    await vi.advanceTimersByTimeAsync(9_999);

    expect(held.sent).toEqual(['/refresh']);

    await vi.advanceTimersByTimeAsync(1);
    await refused;

    expect(held.sent).toEqual(['/refresh', '/logout']);

    held.answer();
    await loggingOut;
  });
});
