import { Injectable, signal } from '@angular/core';

// Manages the app's light/dark theme. The active theme is reflected by a `dark`
// class on the <html> element (driving Tailwind `dark:` variants) and by
// `data-scheme` on that same element (driving the CSS color-scheme, and the
// design system's own scheme handling). An explicit user choice is persisted to
// localStorage; without one, the OS `prefers-color-scheme` setting is followed
// so it keeps tracking the OS on later visits.
@Injectable({ providedIn: 'root' })
export default class ThemeService {
  readonly isDarkMode = signal(false);

  // Initialize theme from an explicit saved choice, falling back to the OS
  // preference. The OS preference is never persisted here.
  initializeTheme() {
    const savedTheme = localStorage.getItem('theme');

    if (savedTheme === 'dark' || savedTheme === 'light') {
      this.isDarkMode.set(savedTheme === 'dark');
    } else {
      // Use system preference
      this.isDarkMode.set(window.matchMedia('(prefers-color-scheme: dark)').matches);
    }

    this.applyTheme();
  }

  // Set theme explicitly in response to a user action, and persist the choice.
  setTheme(value: string) {
    this.isDarkMode.set(value === 'dark');
    localStorage.setItem('theme', this.isDarkMode() ? 'dark' : 'light');

    if (document.startViewTransition) {
      document.startViewTransition(this.applyTheme.bind(this));
    } else {
      this.applyTheme();
    }
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
