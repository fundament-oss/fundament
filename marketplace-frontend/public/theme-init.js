/**
 * Applies the saved or system theme before first paint, so a reload does not
 * flash the wrong one. Server-rendered responses already carry the class when
 * the visitor has a theme cookie; this covers the client-rendered routes and
 * the visitors who never chose, whose OS preference the server cannot know.
 *
 * Kept as a file rather than an inline script because the app is served with
 * `script-src 'self'`, which blocks inline execution. Mirrors ThemeService.
 */
(function () {
  try {
    var match = document.cookie.match(/(?:^|;\s*)theme=(dark|light)(?:;|$)/);
    var saved = match ? match[1] : localStorage.getItem('theme');
    var dark =
      saved === 'dark' ||
      (saved !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches);
    document.documentElement.classList.toggle('dark', dark);
  } catch (e) {
    /* no stored preference is readable; the app applies one after boot */
  }
})();
