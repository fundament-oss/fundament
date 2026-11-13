import { Injectable, computed, inject, signal } from '@angular/core';
import { ConfigService } from './config.service';
import {
  type ThemeChoice,
  parseThemeCookie,
  readStoredTheme,
  writeThemeCookie,
} from './theme-cookie';

// Manages the app's light/dark theme. The active theme is reflected by a `dark`
// class on the <html> element (driving Tailwind `dark:` variants) and by
// `data-scheme` on that same element (driving the CSS color-scheme, and the
// design system's own scheme handling).
//
// Three settings, not two: light, dark, and following the operating system.
// "system" is stored as such, never as the OS's current answer, so it keeps
// tracking the OS on later visits — including a switch made while the app is
// open. The choice is a cookie shared with the other Fundament apps (see
// theme-cookie.ts), so it carries over between them.
@Injectable({ providedIn: 'root' })
export default class ThemeService {
  private readonly config = inject(ConfigService);

  private readonly darkMq = window.matchMedia('(prefers-color-scheme: dark)');

  private readonly systemPrefersDark = signal(this.darkMq.matches);

  readonly themePreference = signal<ThemeChoice>('system');

  readonly isDarkMode = computed(() =>
    this.themePreference() === 'system'
      ? this.systemPrefersDark()
      : this.themePreference() === 'dark',
  );

  constructor() {
    this.darkMq.addEventListener('change', (event) => {
      this.systemPrefersDark.set(event.matches);
      this.applyTheme();
    });
    // Pick up a choice made in another Fundament app while this tab was in the
    // background. No readable cookie (cookies blocked) leaves this tab as is.
    document.addEventListener('visibilitychange', () => {
      const stored = parseThemeCookie(document.cookie);
      if (document.visibilityState !== 'visible' || !stored) return;
      this.themePreference.set(stored);
      this.applyTheme();
    });
  }

  // Initialize from the saved choice; without one, follow the OS.
  initializeTheme() {
    this.themePreference.set(readStoredTheme(this.config.getConfig().cookieDomain) ?? 'system');
    this.applyTheme();
  }

  // Set theme explicitly in response to a user action, and persist the choice.
  setTheme(value: string) {
    const preference = value === 'dark' || value === 'light' ? value : 'system';
    this.themePreference.set(preference);

    writeThemeCookie(preference, this.config.getConfig().cookieDomain);

    this.applyTheme();
  }

  // Apply the active theme to the <html> element.
  private applyTheme() {
    const htmlElement = document.documentElement;

    if (this.isDarkMode()) {
      htmlElement.classList.add('dark');
    } else {
      htmlElement.classList.remove('dark');
    }

    // The design system keys its own color-scheme handling on :root[data-scheme],
    // so keep that in sync with our 'dark' class. Mirrors the inline script in index.html.
    htmlElement.dataset['scheme'] = this.isDarkMode() ? 'dark' : 'light';
  }
}
