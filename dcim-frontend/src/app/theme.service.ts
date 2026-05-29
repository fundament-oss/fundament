import { Injectable, signal } from '@angular/core';

type Theme = 'light' | 'dark';

const STORAGE_KEY = 'dcim-theme';

function readInitialTheme(): Theme {
  // The inline script in index.html has already applied the class; trust it.
  if (document.documentElement.classList.contains('dark')) return 'dark';

  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === 'dark' || stored === 'light') return stored;
  } catch {
    /* ignore: storage unavailable */
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

/**
 * Manages the app's color theme by toggling the `.dark` class on <html>, which
 * drives both Tailwind's `dark:` variant and the CSS `color-scheme` (see
 * styles.css). The initial class is set by an inline script in index.html to
 * avoid a flash; this service mirrors that state into a signal and persists
 * changes to localStorage.
 */
@Injectable({ providedIn: 'root' })
export default class ThemeService {
  private readonly darkState = signal(readInitialTheme() === 'dark');

  /** Whether dark mode is currently active. */
  readonly isDark = this.darkState.asReadonly();

  toggle(): void {
    this.set(this.darkState() ? 'light' : 'dark');
  }

  set(theme: Theme): void {
    const dark = theme === 'dark';
    this.darkState.set(dark);
    document.documentElement.classList.toggle('dark', dark);
    try {
      localStorage.setItem(STORAGE_KEY, theme);
    } catch {
      /* ignore: storage unavailable */
    }
  }
}
