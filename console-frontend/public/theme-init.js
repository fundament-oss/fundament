// Apply the saved/system theme before first paint to avoid a flash of the wrong
// theme on reload. Mirrors App.initializeTheme().
//
// This lives in a separate file rather than an inline <script> in index.html
// because the Console's CSP is script-src 'self' (see nginx.conf) and would
// block inline execution.
(function () {
  try {
    var saved = localStorage.getItem('theme');
    var dark =
      saved === 'dark' ||
      (saved !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches);
    if (dark) document.documentElement.classList.add('dark');
    document.documentElement.dataset.scheme = dark ? 'dark' : 'light';
  } catch (e) {}
})();
