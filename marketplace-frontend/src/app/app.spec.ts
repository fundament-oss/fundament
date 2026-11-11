import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { of } from 'rxjs';
import type App from './app';
import SessionService from './session.service';
import StaffSessionService from './admin-review/staff-session.service';
import OrganizationContextService from './organization-context.service';
import { ToastService } from './toast.service';
import { CONFIG_LOADER, ConfigService, EMPTY_CONFIGURATION } from './config.service';
import { AUTHN_CLIENT } from '../connect/authn';
import { ORGANIZATION_CLIENT } from '../connect/organization';

describe('App', () => {
  let AppClass: typeof App;
  let app: App;
  let session: SessionService;
  let context: OrganizationContextService;
  let toast: ToastService;

  // Imported here because the design system's tooltip calls matchMedia as it
  // loads, and jsdom has none.
  beforeAll(async () => {
    vi.stubGlobal('matchMedia', () => ({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    }));
    AppClass = (await import('./app')).default;
  });

  beforeEach(async () => {
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: CONFIG_LOADER,
          useValue: async () => ({ ...EMPTY_CONFIGURATION, authnApiUrl: 'https://authn.test' }),
        },
        { provide: AUTHN_CLIENT, useValue: { getUserInfo: () => of({}) } },
        { provide: ORGANIZATION_CLIENT, useValue: {} },
      ],
    });
    await TestBed.inject(ConfigService).loadConfig();
    session = TestBed.inject(SessionService);
    context = TestBed.inject(OrganizationContextService);
    toast = TestBed.inject(ToastService);
    // Not rendered: these exercise the class, and the variant is a build-time
    // constant that a test can only change on the instance.
    app = TestBed.createComponent(AppClass).componentInstance;
    Object.assign(app, { variant: 'registry' });
  });

  function stubLogout(outcome: 'ok' | 'fails') {
    const redirect = vi.spyOn(session, 'redirectAfterLogout').mockImplementation(() => {});
    const forget = vi.spyOn(context, 'forgetStoredOrganization');
    vi.spyOn(session, 'logout').mockImplementation(async () => {
      if (outcome === 'fails') throw new Error('authn said no');
    });
    return { redirect, forget };
  }

  it('sends the portal to the login after logging out, without emptying the organization', async () => {
    const { redirect, forget } = stubLogout('ok');
    context.organizationId.set('org');

    await app.handleLogout();

    expect(forget).toHaveBeenCalledOnce();
    expect(redirect).toHaveBeenCalledWith('/manage');
    expect(context.organizationId()).toBe('org');
  });

  it('stays on the page and says so when logging out fails', async () => {
    const { redirect, forget } = stubLogout('fails');
    const error = vi.spyOn(toast, 'error');

    await app.handleLogout();

    expect(error).toHaveBeenCalledOnce();
    expect(forget).not.toHaveBeenCalled();
    expect(redirect).not.toHaveBeenCalled();
  });

  describe('in the review backoffice', () => {
    function stubStaffLogout(outcome: 'ok' | 'fails') {
      const staffSession = TestBed.inject(StaffSessionService);
      const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
      vi.spyOn(staffSession, 'logout').mockImplementation(async () => {
        if (outcome === 'fails') throw new Error('dcim-authn said no');
      });
      const consoleLogout = vi.spyOn(session, 'logout');
      Object.assign(app, { variant: 'admin' });
      return { navigate, consoleLogout };
    }

    it('ends the staff session and goes to the login', async () => {
      const { navigate, consoleLogout } = stubStaffLogout('ok');

      await app.handleLogout();

      expect(navigate).toHaveBeenCalledWith('/login', { replaceUrl: true });
      expect(consoleLogout).not.toHaveBeenCalled();
    });

    it('stays on the page and says so when logging out fails', async () => {
      const { navigate } = stubStaffLogout('fails');
      const error = vi.spyOn(toast, 'error');

      await app.handleLogout();

      expect(error).toHaveBeenCalledOnce();
      expect(navigate).not.toHaveBeenCalled();
    });
  });

  it('keeps the storefront visitor on the page after logging out', async () => {
    const { redirect, forget } = stubLogout('ok');
    Object.assign(app, { variant: 'catalog' });

    await app.handleLogout();

    expect(forget).not.toHaveBeenCalled();
    expect(redirect).not.toHaveBeenCalled();
  });
});
