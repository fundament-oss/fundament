// The visitor's explicit light/dark choice, shared by every Fundament frontend.
//
// The Console, DCIM and the marketplace frontends each run on their own
// subdomain, and localStorage is per origin, so a choice kept there stayed in
// the app it was made in. A cookie on the common parent domain (the same
// `externalUrls.cookieDomain` the session cookies use) is visible to all of
// them, so a theme picked in one app applies in the others too.
//
// This file is duplicated in console-frontend, dcim-frontend and
// marketplace-frontend, like theme-init.js; keep the copies in sync.

export type StoredTheme = 'dark' | 'light';

export const THEME_COOKIE_NAME = 'theme';

// Pre-cookie versions of the apps kept the choice in localStorage under this
// key. It is adopted once, then removed.
export const LEGACY_THEME_STORAGE_KEY = 'theme';

const COOKIE_MAX_AGE = 60 * 60 * 24 * 365;

export function parseThemeCookie(cookieHeader: string | null | undefined): StoredTheme | null {
  const match = cookieHeader?.match(/(?:^|;\s*)theme=(dark|light)(?:;|$)/);
  return match ? (match[1] as StoredTheme) : null;
}

// The configured domain, or '' when it does not cover the current host (a
// misconfiguration, `bun start` on plain localhost, the demo on a static host).
// A browser silently drops a cookie for a domain outside its own, so writing
// one there would lose the choice.
function usableDomain(hostname: string, cookieDomain: string | undefined): string {
  const domain = (cookieDomain ?? '').trim().replace(/^\./, '').toLowerCase();
  const host = hostname.toLowerCase();
  return domain && (host === domain || host.endsWith(`.${domain}`)) ? domain : '';
}

/**
 * Persists an explicit choice on the common domain, or clears it for null (the
 * visitor follows the OS again). Without a usable domain the cookie is
 * host-only, which still beats per-origin localStorage for SSR. Browser only.
 */
export function writeThemeCookie(
  theme: StoredTheme | null,
  cookieDomain: string | undefined,
): void {
  const domain = usableDomain(window.location.hostname, cookieDomain);
  const secure = window.location.protocol === 'https:' ? '; secure' : '';
  const attributes = `; path=/; samesite=lax${secure}`;
  const value = theme ?? '';
  const maxAge = theme ? COOKIE_MAX_AGE : 0;

  if (domain) {
    // Drop a host-only cookie of the same name (written by an earlier
    // version), or it would shadow the shared one in this app.
    document.cookie = `${THEME_COOKIE_NAME}=${attributes}; max-age=0`;
    document.cookie = `${THEME_COOKIE_NAME}=${value}; domain=${domain}${attributes}; max-age=${maxAge}`;

    // A domain the browser refuses (a public suffix, say) leaves nothing
    // behind; keep the choice for this app at least.
    if (theme && parseThemeCookie(document.cookie) !== theme) {
      document.cookie = `${THEME_COOKIE_NAME}=${value}${attributes}; max-age=${maxAge}`;
    }
  } else {
    document.cookie = `${THEME_COOKIE_NAME}=${value}${attributes}; max-age=${maxAge}`;
  }
}

/**
 * Reads the stored choice in the browser. A choice that only exists in legacy
 * localStorage is moved into the cookie, and a cookie found on this host is
 * rewritten so it lands on the common domain. Browser only.
 */
export function readStoredTheme(cookieDomain: string | undefined): StoredTheme | null {
  let legacy: string | null = null;
  try {
    legacy = localStorage.getItem(LEGACY_THEME_STORAGE_KEY);
    localStorage.removeItem(LEGACY_THEME_STORAGE_KEY);
  } catch {
    // Storage can be unavailable (private mode, blocked site data).
  }

  const stored =
    parseThemeCookie(document.cookie) ?? (legacy === 'dark' || legacy === 'light' ? legacy : null);
  if (stored) {
    writeThemeCookie(stored, cookieDomain);
  }
  return stored;
}
