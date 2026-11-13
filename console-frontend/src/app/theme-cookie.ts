// The visitor's light/dark choice, shared by every Fundament frontend.
//
// The Console, DCIM and the marketplace frontends each run on their own
// subdomain, and localStorage is per origin, so a choice kept there stayed in
// the app it was made in. A cookie on the common parent domain (the same
// `externalUrls.cookieDomain` the session cookies use) is visible to all of
// them, so a theme picked in one app applies in the others too.
//
// The cookie is `fundament_theme`, not plain `theme`: the parent domain can be
// shared with services that are not ours (the sandbox domain, say), and the
// marketplace's older host-only `theme` cookie would otherwise shadow it.
//
// It holds `dark`, `light` or `system`. Storing `system` rather than deleting
// the cookie keeps "chose to follow the OS" apart from "never chose", so an old
// per-app choice is never migrated over an explicit System.
//
// This file is duplicated in console-frontend, dcim-frontend and
// marketplace-frontend, like theme-init.js. CI fails when the copies differ
// (theme-cookie-copies in .github/workflows/test.yml).

export type ThemeChoice = 'dark' | 'light' | 'system';

export const THEME_COOKIE_NAME = 'fundament_theme';

// Where earlier versions kept an explicit choice: localStorage in the Console
// and DCIM, a host-only cookie in the marketplace, both named `theme`. Adopted
// only while no shared cookie exists, and removed once the cookie holds it.
export const LEGACY_THEME_KEY = 'theme';

const COOKIE_MAX_AGE = 60 * 60 * 24 * 365;

export function parseThemeCookie(cookieHeader: string | null | undefined): ThemeChoice | null {
  const match = cookieHeader?.match(/(?:^|;\s*)fundament_theme=(dark|light|system)(?:;|$)/);
  return match ? (match[1] as ThemeChoice) : null;
}

/** The marketplace's host-only cookie from before the shared one. */
export function parseLegacyThemeCookie(
  cookieHeader: string | null | undefined,
): 'dark' | 'light' | null {
  const match = cookieHeader?.match(/(?:^|;\s*)theme=(dark|light)(?:;|$)/);
  return match ? (match[1] as 'dark' | 'light') : null;
}

// The configured domain, or '' when it does not cover the current host (a
// misconfiguration, `bun start` on plain localhost, the demo on a static host).
// A browser silently drops a cookie for a domain outside its own, so writing
// one there would lose the choice.
function usableDomain(cookieDomain: string | undefined): string {
  const domain = (cookieDomain ?? '').trim().replace(/^\./, '').toLowerCase();
  const host = window.location.hostname.toLowerCase();
  return domain && (host === domain || host.endsWith(`.${domain}`)) ? domain : '';
}

function deleteHostOnlyCookie(name: string): void {
  document.cookie = `${name}=; path=/; max-age=0`;
}

/**
 * Persists the choice on the common domain. Without a usable domain, or when
 * the browser refuses it (a public suffix, say), the cookie is host-only, so
 * the choice still holds in this app. Browser only.
 */
export function writeThemeCookie(theme: ThemeChoice, cookieDomain: string | undefined): void {
  const secure = window.location.protocol === 'https:' ? '; secure' : '';
  const cookie = `${THEME_COOKIE_NAME}=${theme}; path=/; max-age=${COOKIE_MAX_AGE}; samesite=lax${secure}`;
  const domain = usableDomain(cookieDomain);

  if (domain) {
    // A host-only cookie of the same name would shadow the shared one here.
    deleteHostOnlyCookie(THEME_COOKIE_NAME);
    document.cookie = `${cookie}; domain=${domain}`;
    if (parseThemeCookie(document.cookie) === theme) return;
  }
  document.cookie = cookie;
}

function readLegacyTheme(): 'dark' | 'light' | null {
  const fromCookie = parseLegacyThemeCookie(document.cookie);
  if (fromCookie) return fromCookie;
  try {
    const legacy = localStorage.getItem(LEGACY_THEME_KEY);
    return legacy === 'dark' || legacy === 'light' ? legacy : null;
  } catch {
    // Storage can be unavailable (private mode, blocked site data).
    return null;
  }
}

function deleteLegacyTheme(): void {
  deleteHostOnlyCookie(LEGACY_THEME_KEY);
  try {
    localStorage.removeItem(LEGACY_THEME_KEY);
  } catch {
    // Nothing stored there that could come back either.
  }
}

/**
 * Reads the stored choice in the browser, or null when there is none. Along
 * the way it moves a choice that only exists on this host (a legacy one, or a
 * host-only cookie from when the domain was not usable) to the common domain.
 * Browser only.
 */
export function readStoredTheme(cookieDomain: string | undefined): ThemeChoice | null {
  let stored = parseThemeCookie(document.cookie);

  if (stored && usableDomain(cookieDomain)) {
    // A host-only copy can sit beside the shared cookie, and a plain read takes
    // whichever the browser lists first, which may be the stale one. Drop the
    // copy and read again; if nothing is left, the copy was all there was, and
    // its value is moved to the domain below.
    deleteHostOnlyCookie(THEME_COOKIE_NAME);
    stored = parseThemeCookie(document.cookie) ?? stored;
  }

  const choice = stored ?? readLegacyTheme();
  if (choice) {
    // Also renews the expiry, so a regular visitor never loses the choice.
    writeThemeCookie(choice, cookieDomain);
  }

  // With cookies blocked, the legacy copy is all that keeps the choice, so it
  // only goes once the cookie reads back.
  if (parseThemeCookie(document.cookie)) {
    deleteLegacyTheme();
  }
  return choice;
}
