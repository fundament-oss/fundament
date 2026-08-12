import { Injectable, computed, signal } from '@angular/core';

// Manages the app's light/dark theme. The active theme is reflected by a `dark`
// class on the <html> element (driving Tailwind `dark:` variants) and by
// `data-scheme` on that same element (driving the CSS color-scheme, and the
// design system's own scheme handling).
//
// Three settings, not two: light, dark, and following the operating system.
// Only an explicit choice is persisted, so "system" is the absence of a stored
// value and keeps tracking the OS on later visits — including a switch made
// while the app is open.
@Injectable({ providedIn: 'root' })
export default class ThemeService {
  private readonly darkMq = window.matchMedia('(prefers-color-scheme: dark)');

  private readonly systemPrefersDark = signal(this.darkMq.matches);

  readonly themePreference = signal<'system' | 'light' | 'dark'>('system');

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
  }

  // Initialize from an explicit saved choice; without one, follow the OS.
  initializeTheme() {
    const saved = localStorage.getItem('theme');
    this.themePreference.set(saved === 'dark' || saved === 'light' ? saved : 'system');
    this.applyTheme();
  }

  // Set theme explicitly in response to a user action, and persist the choice.
  setTheme(value: string) {
    const preference = value === 'dark' || value === 'light' ? value : 'system';
    this.themePreference.set(preference);

    if (preference === 'system') {
      localStorage.removeItem('theme');
    } else {
      localStorage.setItem('theme', preference);
    }

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
