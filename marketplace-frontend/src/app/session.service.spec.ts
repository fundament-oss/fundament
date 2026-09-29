import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';
import { of, throwError } from 'rxjs';
import SessionService from './session.service';
import { CONFIG_LOADER, ConfigService, EMPTY_CONFIGURATION } from './config.service';
import { AUTHN_CLIENT } from '../connect/authn';

const signedIn = { user: { organizationIds: ['org'] } };

describe('SessionService', () => {
  let getUserInfo: ReturnType<typeof vi.fn>;

  beforeEach(async () => {
    vi.useFakeTimers();
    sessionStorage.clear();
    getUserInfo = vi.fn(() => of(signedIn));

    TestBed.configureTestingModule({
      providers: [
        {
          provide: CONFIG_LOADER,
          useValue: async () => ({ ...EMPTY_CONFIGURATION, authnApiUrl: 'https://authn.test' }),
        },
        { provide: AUTHN_CLIENT, useValue: { getUserInfo } },
      ],
    });
    await TestBed.inject(ConfigService).loadConfig();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('does not ask again for a user it just confirmed', async () => {
    const session = TestBed.inject(SessionService);
    await session.ensureUser();

    await vi.advanceTimersByTimeAsync(14_999);
    await expect(session.recheckUser()).resolves.toEqual(signedIn.user);
    expect(getUserInfo).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(1);
    await session.recheckUser();
    expect(getUserInfo).toHaveBeenCalledTimes(2);
  });

  it('always asks again for a visitor who is signed out', async () => {
    const session = TestBed.inject(SessionService);
    getUserInfo.mockImplementation(() =>
      throwError(() => new ConnectError('no session', Code.Unauthenticated)),
    );
    await session.ensureUser();

    await session.recheckUser();
    expect(getUserInfo).toHaveBeenCalledTimes(2);
  });

  it('ends the session when authn says it is gone', async () => {
    const session = TestBed.inject(SessionService);
    await session.ensureUser();
    await vi.advanceTimersByTimeAsync(15_000);
    getUserInfo.mockImplementation(() =>
      throwError(() => new ConnectError('no session', Code.Unauthenticated)),
    );

    await expect(session.recheckUser()).resolves.toBeNull();
    expect(session.user()).toBeNull();
  });

  it('keeps the user, unconfirmed, when authn does not answer', async () => {
    const session = TestBed.inject(SessionService);
    await session.ensureUser();
    await vi.advanceTimersByTimeAsync(15_000);
    getUserInfo.mockImplementation(() =>
      throwError(() => new ConnectError('down', Code.Unavailable)),
    );

    await expect(session.recheckUser()).resolves.toEqual(signedIn.user);
    // Not confirmed, so the next recheck asks again straight away.
    await session.recheckUser();
    expect(getUserInfo).toHaveBeenCalledTimes(3);
  });
});
