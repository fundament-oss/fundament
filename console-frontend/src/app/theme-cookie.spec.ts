import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  LEGACY_THEME_STORAGE_KEY,
  parseThemeCookie,
  readStoredTheme,
  writeThemeCookie,
} from './theme-cookie';

/**
 * The theme choice is one cookie on the domain every Fundament frontend shares,
 * so a choice made in the Console shows in DCIM and the marketplace too.
 */

function clearThemeCookies() {
  document.cookie = 'theme=; path=/; max-age=0';
  document.cookie = `theme=; domain=${window.location.hostname}; path=/; max-age=0`;
}

describe('theme cookie', () => {
  beforeEach(() => {
    clearThemeCookies();
    localStorage.clear();
  });

  afterEach(() => {
    clearThemeCookies();
    localStorage.clear();
  });

  it('parses only an explicit light or dark choice', () => {
    expect(parseThemeCookie('a=1; theme=dark; b=2')).toBe('dark');
    expect(parseThemeCookie('theme=light')).toBe('light');
    expect(parseThemeCookie('theme=system')).toBeNull();
    expect(parseThemeCookie('mytheme=dark')).toBeNull();
    expect(parseThemeCookie(undefined)).toBeNull();
  });

  it('round-trips a choice on the configured domain', () => {
    writeThemeCookie('dark', window.location.hostname);
    expect(parseThemeCookie(document.cookie)).toBe('dark');

    writeThemeCookie('light', window.location.hostname);
    expect(parseThemeCookie(document.cookie)).toBe('light');
  });

  it('keeps the choice on this host when the domain does not cover it', () => {
    writeThemeCookie('dark', 'elsewhere.example');
    expect(parseThemeCookie(document.cookie)).toBe('dark');
  });

  it('clears the choice for null, so the app follows the OS again', () => {
    writeThemeCookie('dark', window.location.hostname);
    writeThemeCookie(null, window.location.hostname);
    expect(parseThemeCookie(document.cookie)).toBeNull();
  });

  it('moves a legacy localStorage choice into the cookie', () => {
    localStorage.setItem(LEGACY_THEME_STORAGE_KEY, 'dark');

    expect(readStoredTheme(window.location.hostname)).toBe('dark');
    expect(parseThemeCookie(document.cookie)).toBe('dark');
    expect(localStorage.getItem(LEGACY_THEME_STORAGE_KEY)).toBeNull();
  });

  it('prefers the shared cookie over a legacy localStorage choice', () => {
    writeThemeCookie('light', window.location.hostname);
    localStorage.setItem(LEGACY_THEME_STORAGE_KEY, 'dark');

    expect(readStoredTheme(window.location.hostname)).toBe('light');
    expect(localStorage.getItem(LEGACY_THEME_STORAGE_KEY)).toBeNull();
  });

  it('reports no choice when nothing is stored', () => {
    expect(readStoredTheme(window.location.hostname)).toBeNull();
  });
});
