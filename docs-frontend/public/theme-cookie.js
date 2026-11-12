// The visitor's light/dark choice, shared with the other Fundament frontends.
//
// The Console, DCIM, the marketplace and these docs each run on their own
// subdomain, and localStorage is per origin. The choice therefore lives in a
// `theme` cookie on the common parent domain (`cookieDomain` in
// /config/config.js, the domain the session cookies use), which every frontend
// reads and writes. It holds `dark` or `light`; no cookie means "follow the OS",
// which Starlight calls `auto`. This mirrors theme-cookie.ts in the Angular apps.
//
// Starlight's own theme code keeps reading localStorage `starlight-theme`, so
// syncStarlight() copies the cookie there before Starlight looks.
//
// A classic script, loaded before first paint by the ThemeProvider override and
// by the landing layout. It only defines window.fundamentTheme.
(() => {
  const NAME = 'theme';
  const MAX_AGE = 60 * 60 * 24 * 365;
  const STARLIGHT_KEY = 'starlight-theme';
  // Where earlier versions of the landing page kept the choice.
  const LANDING_KEY = 'fnd-scheme';
  // Set once the localStorage choice from before the shared cookie has been
  // moved into it. Starlight keeps writing its own key, so without this a stale
  // copy would be taken for a choice still to migrate.
  const MIGRATED_KEY = 'fundament-theme-cookie';

  const read = () => {
    const match = document.cookie.match(/(?:^|;\s*)theme=(dark|light)(?:;|$)/);
    return match ? match[1] : null;
  };

  // The configured domain, or '' when it does not cover this host (`bun start`
  // on localhost, a misconfiguration): the browser would drop such a cookie.
  const usableDomain = () => {
    const config = window.fundamentConfig || {};
    const domain = String(config.cookieDomain || '')
      .trim()
      .replace(/^\./, '')
      .toLowerCase();
    const host = window.location.hostname.toLowerCase();
    return domain && (host === domain || host.endsWith(`.${domain}`)) ? domain : '';
  };

  // Stores `dark` or `light`, or clears the choice for anything else.
  const write = (theme) => {
    const explicit = theme === 'dark' || theme === 'light';
    const value = explicit ? theme : '';
    const maxAge = explicit ? MAX_AGE : 0;
    const secure = window.location.protocol === 'https:' ? '; secure' : '';
    const attributes = `; path=/; samesite=lax${secure}`;
    const domain = usableDomain();

    if (domain) {
      // A host-only cookie of the same name would shadow the shared one.
      document.cookie = `${NAME}=${attributes}; max-age=0`;
      document.cookie = `${NAME}=${value}; domain=${domain}${attributes}; max-age=${maxAge}`;
      // A domain the browser refuses leaves nothing; keep it for this site then.
      if (explicit && read() !== theme) {
        document.cookie = `${NAME}=${value}${attributes}; max-age=${maxAge}`;
      }
    } else {
      document.cookie = `${NAME}=${value}${attributes}; max-age=${maxAge}`;
    }
  };

  const migrate = () => {
    try {
      if (localStorage.getItem(MIGRATED_KEY)) return;
      const legacy = localStorage.getItem(STARLIGHT_KEY) || localStorage.getItem(LANDING_KEY);
      if (!read() && (legacy === 'dark' || legacy === 'light')) write(legacy);
      localStorage.removeItem(LANDING_KEY);
      localStorage.setItem(MIGRATED_KEY, '1');
    } catch {
      // Storage is unavailable (private mode, blocked site data): nothing to move.
    }
  };

  window.fundamentTheme = {
    /** The stored choice: 'dark', 'light' or 'auto'. */
    choice() {
      migrate();
      return read() || 'auto';
    },

    /** Stores 'dark' or 'light', or clears the choice for 'auto'. */
    store(choice) {
      write(choice);
    },

    /** Copies the stored choice to the key Starlight reads; returns it. */
    syncStarlight() {
      const choice = this.choice();
      try {
        localStorage.setItem(STARLIGHT_KEY, choice === 'auto' ? '' : choice);
      } catch {
        // Starlight then falls back to the OS preference, as for a new visitor.
      }
      return choice;
    },
  };
})();
