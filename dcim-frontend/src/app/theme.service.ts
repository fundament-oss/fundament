import { Injectable, signal } from '@angular/core';

// Manages the app's light/dark theme. The active theme is reflected by a `dark`
// class on the <html> element (so Tailwind `dark:` variants and the CSS
// color-scheme follow it) and persisted to localStorage.
@Injectable({ providedIn: 'root' })
export default class ThemeService {
  readonly isDarkMode = signal(false);

  // Initialize theme from localStorage or system preference.
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

  // Set theme explicitly.
  setTheme(value: string) {
    this.isDarkMode.set(value === 'dark');

    if (document.startViewTransition) {
      document.startViewTransition(this.applyTheme.bind(this));
    } else {
      this.applyTheme();
    }
  }

  // Toggle theme.
  toggleTheme() {
    this.isDarkMode.set(!this.isDarkMode());

    // Apply with view transition if supported. Use 80 ms delay to allow CSS transition on the switch to start
    setTimeout(() => {
      if (document.startViewTransition) {
        document.startViewTransition(this.applyTheme.bind(this));
      } else {
        this.applyTheme();
      }
    }, 80);
  }

  // Apply theme to HTML element and save to localStorage.
  private applyTheme() {
    const htmlElement = document.documentElement;

    if (this.isDarkMode()) {
      htmlElement.classList.add('dark');
    } else {
      htmlElement.classList.remove('dark');
    }

    localStorage.setItem('theme', this.isDarkMode() ? 'dark' : 'light');
  }
}
