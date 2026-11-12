// The visitor's light/dark choice, shared with the other Fundament frontends.
//
// The Console, DCIM, the marketplace and these docs each run on their own
// subdomain, and localStorage is per origin. The choice therefore lives in a
// `fundament_theme` cookie on the common parent domain (`cookieDomain` in
// /config/config.js, the domain the session cookies use), which every frontend
// reads and writes. It holds `dark`, `light` or `system` (Starlight's `auto`);
// storing `system` keeps "chose to follow the OS" apart from "never chose". This
// mirrors theme-cookie.ts in the Angular apps; keep the two in step.
//
// Starlight's own theme code keeps reading localStorage `starlight-theme`, so
// syncStarlight() copies the cookie there before Starlight looks. That key also
// holds the choice from before the shared cookie, and is adopted while there is
// no cookie at all.
//
// A classic script, loaded before first paint by the ThemeProvider override and
// by the landing layout, each after /config/config.js. It only defines
// window.fundamentTheme.
(() => {
  const NAME = 'fundament_theme';
  const MAX_AGE = 60 * 60 * 24 * 365;
  const STARLIGHT_KEY = 'starlight-theme';
  // Where earlier versions of the landing page kept the choice.
  const LANDING_KEY = 'fnd-scheme';

  // 'dark', 'light' or 'system'; null without a (readable) cookie.
  const read = () => {
    const match = document.cookie.match(/(?:^|;\s*)fundament_theme=(dark|light|system)(?:;|$)/);
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

  const deleteHostOnly = () => {
    document.cookie = `${NAME}=; path=/; max-age=0`;
  };

  // Stores 'dark', 'light' or 'system' on the common domain, or on this host
  // when the domain is not usable or the browser refuses it.
  const write = (choice) => {
    const secure = window.location.protocol === 'https:' ? '; secure' : '';
    const cookie = `${NAME}=${choice}; path=/; max-age=${MAX_AGE}; samesite=lax${secure}`;
    const domain = usableDomain();
    if (domain) {
      // A host-only cookie of the same name would shadow the shared one.
      deleteHostOnly();
      document.cookie = `${cookie}; domain=${domain}`;
      if (read() === choice) return;
    }
    document.cookie = cookie;
  };

  const legacy = () => {
    try {
      const value = localStorage.getItem(STARLIGHT_KEY) || localStorage.getItem(LANDING_KEY);
      return value === 'dark' || value === 'light' ? value : null;
    } catch {
      // Storage is unavailable (private mode, blocked site data).
      return null;
    }
  };

  // The stored choice. Like readStoredTheme() in the Angular apps, it moves a
  // choice that only exists on this host to the common domain on the way.
  const load = () => {
    let stored = read();
    if (stored && usableDomain()) {
      // A host-only copy (from when the domain was not usable) can sit beside
      // the shared cookie, and a plain read may take the stale one. Drop it and
      // read again; if nothing is left, the copy was all there was.
      deleteHostOnly();
      stored = read() || stored;
    }
    stored = stored || legacy();
    if (stored) write(stored);
    if (read()) {
      try {
        localStorage.removeItem(LANDING_KEY);
      } catch {
        // Nothing stored there that could come back either.
      }
    }
    return stored;
  };

  // Starlight's own storage, which also keeps the choice when cookies are
  // blocked: '' is its word for following the OS.
  const setStarlight = (choice) => {
    try {
      localStorage.setItem(STARLIGHT_KEY, choice === 'auto' ? '' : choice);
    } catch {
      // Starlight then falls back to the OS preference, as for a new visitor.
    }
  };

  window.fundamentTheme = {
    /** The stored choice: 'dark', 'light' or 'auto'. */
    choice() {
      const stored = load();
      return stored === 'dark' || stored === 'light' ? stored : 'auto';
    },

    /** Stores 'dark', 'light' or 'auto'. */
    store(choice) {
      const explicit = choice === 'dark' || choice === 'light';
      write(explicit ? choice : 'system');
      setStarlight(explicit ? choice : 'auto');
    },

    /** Copies the stored choice to the key Starlight reads; returns it. */
    syncStarlight() {
      const choice = this.choice();
      // Without a readable cookie, Starlight's key may be the only copy of the
      // choice (cookies blocked), so it is left as it is.
      if (read()) setStarlight(choice);
      return choice;
    },
  };
})();
