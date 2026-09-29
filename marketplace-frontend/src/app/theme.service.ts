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

type Theme = 'dark' | 'light';

export type ThemePreference = Theme | 'system';

const COOKIE_NAME = 'theme';
const COOKIE_MAX_AGE = 60 * 60 * 24 * 365;
// Pre-SSR versions of this app kept the choice in localStorage. Read it once so
// returning visitors keep their theme, then migrate it to the cookie.
const LEGACY_STORAGE_KEY = 'theme';

function parseThemeCookie(cookieHeader: string | null | undefined): Theme | null {
  const match = cookieHeader?.match(/(?:^|;\s*)theme=(dark|light)(?:;|$)/);
  return match ? (match[1] as Theme) : null;
}

/**
 * Owns the light/dark choice for the whole app.
 *
 * The choice lives in a cookie rather than in localStorage because the server
 * renders the page too: a cookie is the only client state the server can read,
 * so it can emit `<html class="dark">` in the first response instead of leaving
 * a dark-mode visitor with a flash of the light theme. Visitors who never chose
 * explicitly, or chose 'system', send no cookie; for them the inline script in
 * `index.html` applies the OS preference before first paint, and this service
 * picks it up on hydration. The OS preference is never persisted, so it keeps
 * tracking the OS.
 */
@Injectable({
  providedIn: 'root',
})
export default class ThemeService {
  private document = inject(DOCUMENT);

  private isServer = isPlatformServer(inject(PLATFORM_ID));

  private request = inject(REQUEST, { optional: true });

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

  private readStoredTheme(): Theme | null {
    if (this.isServer) {
      return parseThemeCookie(this.request?.headers.get('cookie'));
    }

    const fromCookie = parseThemeCookie(this.document.cookie);
    if (fromCookie) {
      return fromCookie;
    }

    // No cookie yet: adopt and re-persist a pre-SSR localStorage choice.
    try {
      const legacy = localStorage.getItem(LEGACY_STORAGE_KEY);
      if (legacy === 'dark' || legacy === 'light') {
        this.writeCookie(legacy);
        return legacy;
      }
    } catch {
      // Storage can be unavailable (private mode, blocked cookies); the OS
      // preference is a fine fallback.
    }

    return null;
  }

  // 'system' clears the cookie, so the server and the pre-paint script fall
  // back to the OS preference. The legacy key goes too, or the next load would
  // adopt it again as an explicit choice.
  private persistTheme() {
    const preference = this.preference();

    if (preference === 'system') {
      this.writeCookie('', 0);
      try {
        localStorage.removeItem(LEGACY_STORAGE_KEY);
      } catch {
        // Nothing stored that could be read back either.
      }
    } else {
      this.writeCookie(preference);
    }
  }

  private writeCookie(value: string, maxAge = COOKIE_MAX_AGE) {
    const secure = this.document.location?.protocol === 'https:' ? '; secure' : '';
    this.document.cookie = `${COOKIE_NAME}=${value}; path=/; max-age=${maxAge}; samesite=lax${secure}`;
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
