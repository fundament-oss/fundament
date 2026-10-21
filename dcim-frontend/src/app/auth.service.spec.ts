import { TestBed } from '@angular/core/testing';
import { Code, ConnectError, createRouterTransport } from '@connectrpc/connect';
import AuthService from './auth.service';
import { AuthnService } from '../generated/dcimauthn/v1/dcimauthn_pb';
import { AUTHN_TRANSPORT } from '../connect/tokens';

// The split under test: only Unauthenticated ends the session. Any other
// GetUserInfo failure is a transient outage — the cookie may still be valid,
// so the auth hint must survive and the next navigation retries.
describe('AuthService.initializeAuth', () => {
  const serviceWith = (getUserInfo: () => { user: { id: string; name: string } }) => {
    TestBed.configureTestingModule({
      providers: [
        {
          provide: AUTHN_TRANSPORT,
          useValue: createRouterTransport(({ service }) => {
            service(AuthnService, { getUserInfo });
          }),
        },
      ],
    });
    return TestBed.inject(AuthService);
  };

  afterEach(() => localStorage.removeItem('dcim_auth_hint'));

  it('resolves the user when the directory answers', async () => {
    localStorage.setItem('dcim_auth_hint', 'true');
    const service = serviceWith(() => ({
      user: { id: '019b4000-1000-7000-8000-000000000001', name: 'Alice' },
    }));

    await service.initializeAuth();

    expect(service.user()).toEqual({
      id: '019b4000-1000-7000-8000-000000000001',
      name: 'Alice',
    });
    expect(service.isAuthenticated()).toBe(true);
  });

  it('ends the session and drops the hint on Unauthenticated', async () => {
    localStorage.setItem('dcim_auth_hint', 'true');
    const service = serviceWith(() => {
      throw new ConnectError('invalid or expired token', Code.Unauthenticated);
    });

    await service.initializeAuth();

    expect(service.user()).toBeUndefined();
    expect(localStorage.getItem('dcim_auth_hint')).toBeNull();
  });

  it('keeps the hint on a transient failure, so the next navigation retries', async () => {
    localStorage.setItem('dcim_auth_hint', 'true');
    const service = serviceWith(() => {
      throw new ConnectError('temporarily unavailable', Code.Unavailable);
    });

    await service.initializeAuth();

    expect(service.user()).toBeUndefined();
    expect(localStorage.getItem('dcim_auth_hint')).toBe('true');
  });

  it('skips the probe entirely without a hint', async () => {
    const service = serviceWith(() => {
      throw new Error('GetUserInfo must not be called without an auth hint');
    });

    await service.initializeAuth();

    expect(service.user()).toBeUndefined();
  });
});
