import { AppConfiguration } from '../config.service';

/**
 * Signing in on behalf of another Fundament surface (FUN-20).
 *
 * The marketplace developer portal has no session of its own: it borrows the
 * console's `fundament_auth` cookie, which authn-api sets on the parent domain.
 * So rather than standing up a second password form on the portal, the portal's
 * auth guard sends a visitor here and the console sends them back once the
 * cookie exists. One form in the product means one place to later add rate
 * limiting, lockout or a password reset.
 *
 * The request names an *app*, not a URL. A URL would make this page an open
 * redirect unless the console carried its own allowlist, which would be a
 * second copy of the one authn-api already keeps, in a static bundle, free to
 * drift. An app id resolves against this deployment's own configuration
 * instead: an id it does not know, or knows but has no URL for, is simply not
 * a hand-off and the visitor lands on the console as usual.
 */

/** The surfaces that share this console's session and may hand off to it. */
const HANDOFF_APPS = {
  'marketplace-registry': {
    configKey: 'developerUrl',
    // Says why a visitor who asked for the portal is looking at a console
    // login on another host. Without it the page reads as a wrong turn.
    description:
      'Publishing plugins uses your Fundament account. Signing in here takes you back to the Developer Portal.',
  },
} as const satisfies Record<string, { configKey: keyof AppConfiguration; description: string }>;

type HandoffApp = keyof typeof HANDOFF_APPS;

export interface Handoff {
  /** Absolute URL to leave for once the session exists. */
  url: string;
  /** One line on the login page saying why they are here. */
  description: string;
}

/**
 * Whether a path may be appended to another surface's origin.
 *
 * It has to be a single-slash absolute path: `//evil.example` and its
 * backslash spellings are protocol-relative and resolve to somebody else's
 * origin, and a relative path would resolve against this page. The caller
 * checks the resulting origin too, so either test alone would do; both are
 * here because `new URL` normalizes by the WHATWG rules rather than these,
 * and neither should be the only thing standing between a query parameter and
 * a redirect.
 */
function isAppendablePath(path: string): boolean {
  return /^\/(?![/\\])/.test(path);
}

/**
 * Resolves the `app` and `path` query parameters of a login into the place to
 * return to, or null when this login is the console's own.
 */
export function resolveHandoff(
  app: string | null,
  path: string | null,
  config: AppConfiguration,
): Handoff | null {
  if (!app || !(app in HANDOFF_APPS)) {
    return null;
  }

  const handoff = HANDOFF_APPS[app as HandoffApp];
  const base = config[handoff.configKey];
  if (!base) {
    // The app is known but not deployed here, so there is nowhere to go back
    // to. Treat it as an ordinary login rather than an error: the visitor has
    // a console account either way.
    return null;
  }

  let url: URL;
  try {
    url = new URL(base);
  } catch {
    return null;
  }

  if (path && isAppendablePath(path)) {
    const target = new URL(path, url);
    // Only if it stayed on the surface the app names. A path that resolved
    // somewhere else is dropped, and the visitor lands on that surface's front
    // page, which is somewhere they did ask to be.
    if (target.origin === url.origin) {
      url = target;
    }
  }

  return { url: url.href, description: handoff.description };
}
