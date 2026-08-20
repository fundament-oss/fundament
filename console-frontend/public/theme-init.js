// Apply the saved/system theme before first paint to avoid a flash of the
// wrong theme on reload. Mirrors App.initializeTheme(). Lives as a separate
// asset (referenced from index.html) so the CSP script-src 'self' allows it —
// an inline script would need 'unsafe-inline' or a hash.
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
