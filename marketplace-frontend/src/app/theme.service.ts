import {
  DOCUMENT,
  Injectable,
  PLATFORM_ID,
  REQUEST,
  computed,
  inject,
  signal,
} from '@angular/core';
import { isPlatformServer } from '@angular/common';

import { ConfigService } from './config.service';
import {
  type ThemeChoice,
  parseLegacyThemeCookie,
  parseThemeCookie,
  readStoredTheme,
  writeThemeCookie,
} from './theme-cookie';

export type ThemePreference = ThemeChoice;

/**
 * Owns the light/dark choice for the whole app.
 *
 * The choice lives in a cookie rather than in localStorage because the server
 * renders the page too: a cookie is the only client state the server can read,
 * so it can emit `<html class="dark">` in the first response instead of leaving
 * a dark-mode visitor with a flash of the light theme. For visitors who never
 * chose, or chose 'system', the server renders light and the script in
 * `index.html` applies the OS preference before first paint; this service picks
 * it up on hydration. The OS's current answer is never persisted, so 'system'
 * keeps tracking the OS.
 *
 * The cookie lives on the domain shared by all Fundament frontends (see
 * theme-cookie.ts), so the Console and DCIM read and write the same choice.
 */
@Injectable({
  providedIn: 'root',
})
export default class ThemeService {
  private document = inject(DOCUMENT);

  private isServer = isPlatformServer(inject(PLATFORM_ID));

  private request = inject(REQUEST, { optional: true });

  private config = inject(ConfigService);

  // The server cannot know the OS preference, so it renders the light theme
  // and leaves the rest to the pre-paint script.
  private readonly darkMq = this.isServer ? null : matchMedia('(prefers-color-scheme: dark)');

  private readonly systemPrefersDark = signal(this.darkMq?.matches ?? false);

  /** What the visitor picked; 'system' follows the OS. */
  readonly preference = signal<ThemePreference>('system');

  readonly isDarkMode = computed(() =>
    this.preference() === 'system' ? this.systemPrefersDark() : this.preference() === 'dark',
  );

  constructor() {
    this.preference.set(this.readStoredTheme() ?? 'system');

    // Keep following the OS while the preference is 'system'.
    this.darkMq?.addEventListener('change', (e) => {
      this.systemPrefersDark.set(e.matches);
      this.applyTheme();
    });

    // Pick up a choice made in another Fundament app while this tab was in the
    // background. No readable cookie (cookies blocked) leaves this tab as is.
    if (!this.isServer) {
      this.document.addEventListener('visibilitychange', () => {
        const stored = parseThemeCookie(this.document.cookie);
        if (this.document.visibilityState !== 'visible' || !stored) return;
        this.preference.set(stored);
        this.applyTheme();
      });
    }

    this.applyTheme();
  }

  /**
   * Toggle in response to a user action, and persist the choice. Browser only:
   * nothing on the server can toggle a theme.
   */
  toggle() {
    this.setPreference(this.isDarkMode() ? 'light' : 'dark');
  }

  /** Set the theme in response to a user action, and persist the choice. */
  setPreference(preference: ThemePreference) {
    this.preference.set(preference);
    this.persistTheme();

    // Apply with view transition if supported. Use 80 ms delay to allow CSS transition on the switch to start
    setTimeout(() => {
      if (this.document.startViewTransition) {
        this.document.startViewTransition(() => this.applyTheme());
      } else {
        this.applyTheme();
      }
    }, 80);
  }

  private readStoredTheme(): ThemeChoice | null {
    if (this.isServer) {
      // The browser moves a legacy host-only cookie to the shared one after
      // boot; until then it is the visitor's choice for this first paint.
      const cookies = this.request?.headers.get('cookie');
      return parseThemeCookie(cookies) ?? parseLegacyThemeCookie(cookies);
    }
    return readStoredTheme(this.config.getConfig().cookieDomain);
  }

  // For 'system' the server renders light and the pre-paint script applies the
  // OS preference.
  private persistTheme() {
    writeThemeCookie(this.preference(), this.config.getConfig().cookieDomain);
  }

  // Apply the active theme to the <html> element. Runs on the server too, which
  // is what puts the right theme in the server-rendered HTML.
  private applyTheme() {
    const htmlElement = this.document.documentElement;

    if (this.isDarkMode()) {
      htmlElement.classList.add('dark');
    } else {
      htmlElement.classList.remove('dark');
    }
  }
}
