// @vitest-environment-options { "url": "https://console.fundament.example.com/" }
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  LEGACY_THEME_KEY,
  parseLegacyThemeCookie,
  parseThemeCookie,
  readStoredTheme,
  writeThemeCookie,
} from './theme-cookie';

/**
 * The theme choice is one cookie on the domain every Fundament frontend shares,
 * so a choice made in the Console shows in DCIM and the marketplace too. These
 * tests run on a subdomain of that shared domain.
 */

const SHARED_DOMAIN = 'fundament.example.com';

const hostOnly = (cookie: string) => {
  document.cookie = `${cookie}; path=/`;
};

const onSharedDomain = (cookie: string) => {
  document.cookie = `${cookie}; path=/; domain=${SHARED_DOMAIN}`;
};

function clearCookies() {
  ['fundament_theme', 'theme'].forEach((name) => {
    hostOnly(`${name}=; max-age=0`);
    onSharedDomain(`${name}=; max-age=0`);
  });
}

describe('theme cookie', () => {
  beforeEach(() => {
    clearCookies();
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    clearCookies();
    localStorage.clear();
  });

  it('runs on a subdomain of the shared domain', () => {
    expect(window.location.hostname).toBe(`console.${SHARED_DOMAIN}`);
  });

  it('parses a dark, light or system choice', () => {
    expect(parseThemeCookie('a=1; fundament_theme=dark; b=2')).toBe('dark');
    expect(parseThemeCookie('fundament_theme=light')).toBe('light');
    expect(parseThemeCookie('fundament_theme=system')).toBe('system');
    expect(parseThemeCookie('fundament_theme=auto')).toBeNull();
    expect(parseThemeCookie('theme=dark')).toBeNull();
    expect(parseThemeCookie(undefined)).toBeNull();
  });

  it('parses the legacy marketplace cookie apart from the shared one', () => {
    expect(parseLegacyThemeCookie('theme=dark')).toBe('dark');
    expect(parseLegacyThemeCookie('fundament_theme=dark')).toBeNull();
  });

  it('writes the choice on the shared domain, not only on this host', () => {
    writeThemeCookie('dark', SHARED_DOMAIN);
    // What is left after dropping a host-only cookie is the shared one.
    hostOnly('fundament_theme=; max-age=0');
    expect(parseThemeCookie(document.cookie)).toBe('dark');

    writeThemeCookie('light', SHARED_DOMAIN);
    hostOnly('fundament_theme=; max-age=0');
    expect(parseThemeCookie(document.cookie)).toBe('light');
  });

  it('keeps the choice on this host when the domain does not cover it', () => {
    const setter = vi.spyOn(document, 'cookie', 'set');
    writeThemeCookie('dark', 'elsewhere.example');

    expect(parseThemeCookie(document.cookie)).toBe('dark');
    expect(setter.mock.calls.flat().join('\n')).not.toContain('domain=');
  });

  it('stores system as a choice of its own', () => {
    writeThemeCookie('dark', SHARED_DOMAIN);
    writeThemeCookie('system', SHARED_DOMAIN);
    expect(parseThemeCookie(document.cookie)).toBe('system');
  });

  it('moves a legacy localStorage choice into the cookie', () => {
    localStorage.setItem(LEGACY_THEME_KEY, 'dark');

    expect(readStoredTheme(SHARED_DOMAIN)).toBe('dark');
    expect(parseThemeCookie(document.cookie)).toBe('dark');
    expect(localStorage.getItem(LEGACY_THEME_KEY)).toBeNull();
  });

  it('moves the legacy host-only marketplace cookie into the shared one', () => {
    hostOnly('theme=dark');

    expect(readStoredTheme(SHARED_DOMAIN)).toBe('dark');
    expect(parseLegacyThemeCookie(document.cookie)).toBeNull();
    hostOnly('fundament_theme=; max-age=0');
    expect(parseThemeCookie(document.cookie)).toBe('dark');
  });

  it('never migrates a legacy choice over an explicit System', () => {
    writeThemeCookie('system', SHARED_DOMAIN);
    localStorage.setItem(LEGACY_THEME_KEY, 'dark');
    hostOnly('theme=dark');

    expect(readStoredTheme(SHARED_DOMAIN)).toBe('system');
    expect(parseThemeCookie(document.cookie)).toBe('system');
    expect(localStorage.getItem(LEGACY_THEME_KEY)).toBeNull();
  });

  it('prefers the shared cookie over a stale host-only copy', () => {
    // The host-only copy is older, so the browser lists it first.
    hostOnly('fundament_theme=dark');
    onSharedDomain('fundament_theme=light');

    expect(readStoredTheme(SHARED_DOMAIN)).toBe('light');
    expect(parseThemeCookie(document.cookie)).toBe('light');
  });

  it('promotes a host-only choice once the domain is usable', () => {
    hostOnly('fundament_theme=dark');

    expect(readStoredTheme(SHARED_DOMAIN)).toBe('dark');
    hostOnly('fundament_theme=; max-age=0');
    expect(parseThemeCookie(document.cookie)).toBe('dark');
  });

  it('keeps the legacy choice while the cookie cannot be written', () => {
    localStorage.setItem(LEGACY_THEME_KEY, 'dark');
    vi.spyOn(document, 'cookie', 'set').mockImplementation(() => undefined);

    expect(readStoredTheme(SHARED_DOMAIN)).toBe('dark');
    expect(localStorage.getItem(LEGACY_THEME_KEY)).toBe('dark');
  });

  it('reports no choice when nothing is stored', () => {
    expect(readStoredTheme(SHARED_DOMAIN)).toBeNull();
  });
});
