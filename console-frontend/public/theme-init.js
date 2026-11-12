// Apply the saved/system theme before first paint to avoid a flash of the wrong
// theme on reload. Mirrors App.initializeTheme(). The saved choice is a cookie
// shared with the other Fundament apps (see src/app/theme-cookie.ts); the
// localStorage key is where earlier versions kept it, until the app migrates it.
//
// This lives in a separate file rather than an inline <script> in index.html
// because the Console's CSP is script-src 'self' (see nginx.conf) and would
// block inline execution.
(function () {
  try {
    var match = document.cookie.match(/(?:^|;\s*)theme=(dark|light)(?:;|$)/);
    var saved = match ? match[1] : localStorage.getItem('theme');
    var dark =
      saved === 'dark' ||
      (saved !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches);
    if (dark) document.documentElement.classList.add('dark');
    document.documentElement.dataset.scheme = dark ? 'dark' : 'light';
  } catch (e) {}
})();
