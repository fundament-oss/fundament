import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import type LoginComponent from './login.component';
import StaffSessionService, { type StaffUser } from './staff-session.service';

const REVIEWER: StaffUser = { id: 'u1', name: 'Rev Iewer' };

let LoginComponentClass: typeof LoginComponent;

interface Options {
  /** What ensureUser resolves to, or a promise the test settles itself. */
  user: StaffUser | null | Promise<StaffUser | null>;
  /** Whether the review API refused the session on the way here. */
  refused?: boolean;
  /** What login() does; by default it never settles, so the form stays busy. */
  login?: () => Promise<StaffUser>;
}

async function setup({ user, refused = false, login }: Options) {
  const session = {
    takeRefusal: vi.fn(() => refused),
    ensureUser: vi.fn(() => Promise.resolve(user)),
    login: vi.fn(login ?? (() => new Promise<StaffUser>(() => {}))),
  };

  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      { provide: StaffSessionService, useValue: session },
      {
        provide: ActivatedRoute,
        useValue: {
          snapshot: { queryParamMap: convertToParamMap({ returnUrl: '/admin/submissions/42' }) },
        },
      },
    ],
  });
  // Not rendered: these exercise what the constructor decides, and the form's
  // design-system fields need ElementInternals, which jsdom lacks.
  TestBed.overrideComponent(LoginComponentClass, { set: { template: '' } });
  const navigateByUrl = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
  const fixture = TestBed.createComponent(LoginComponentClass);
  fixture.detectChanges();
  await fixture.whenStable();
  return { component: fixture.componentInstance, navigateByUrl, session };
}

describe('LoginComponent', () => {
  // Imported here because the design system's tooltip calls matchMedia as it
  // loads, and jsdom has none.
  beforeAll(async () => {
    vi.stubGlobal('matchMedia', () => ({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    }));
    LoginComponentClass = (await import('./login.component')).default;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('sends a reviewer who is already signed in on to where they were going', async () => {
    const { component, navigateByUrl } = await setup({ user: REVIEWER });

    expect(navigateByUrl).toHaveBeenCalledWith('/admin/submissions/42', { replaceUrl: true });
    expect(component.error()).toBeNull();
  });

  // dcim-authn-api still accepts the session the review API just refused:
  // sending the reviewer on would only lead back here, round and round.
  it('says the review service refused the session instead of sending the reviewer back', async () => {
    const { component, navigateByUrl } = await setup({ user: REVIEWER, refused: true });

    expect(navigateByUrl).not.toHaveBeenCalled();
    expect(component.error()).toContain('the review service did not accept your session');
  });

  // The session ran out on both sides: the form is all there is to say.
  it('shows only the form when a refused session has no reviewer left', async () => {
    const { component, navigateByUrl } = await setup({ user: null, refused: true });

    expect(navigateByUrl).not.toHaveBeenCalled();
    expect(component.error()).toBeNull();
  });

  // A sign-in started before the lookup answered navigates itself; a second
  // navigation from the lookup would cancel it.
  it('leaves the navigation to a sign-in already in progress', async () => {
    let answerLookup!: (user: StaffUser) => void;
    const lookup = new Promise<StaffUser | null>((resolve) => {
      answerLookup = resolve;
    });
    const { component, navigateByUrl, session } = await setup({ user: lookup });

    component.email.set('rev@example.com');
    component.password.set('secret');
    component.onSubmit().catch(() => {});
    expect(component.isLoading()).toBe(true);
    expect(session.login).toHaveBeenCalledWith('rev@example.com', 'secret');

    answerLookup(REVIEWER);
    await lookup;
    await Promise.resolve();

    expect(navigateByUrl).not.toHaveBeenCalled();
  });
});
