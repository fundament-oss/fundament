// Points the docs' links to the Console and the marketplace at this environment.
//
// The same static build serves every environment, so the Markdown links to the
// sandbox (which also keeps them working on GitHub and without JavaScript). A
// deployment names its own Console and marketplace in /config/config.js (see the
// chart's docs-frontend.yaml); this swaps the sandbox origin for those, keeping
// path, query and fragment. Without them (`bun start`, an image run without a
// config) the links stay on the sandbox.
//
// A classic script, loaded in <head> after /config/config.js by the
// ThemeProvider override and by the landing layout.
(() => {
  const SANDBOX = {
    consoleUrl: 'https://console.fundament.projects.digilab.network',
    marketplaceUrl: 'https://marketplace.fundament.projects.digilab.network',
  };

  const config = window.fundamentConfig || {};
  const swaps = Object.keys(SANDBOX)
    .map((key) => [SANDBOX[key], String(config[key] || '').replace(/\/+$/, '')])
    .filter(([from, to]) => to && to !== from);
  if (swaps.length === 0) return;

  const rewrite = () => {
    document.querySelectorAll('a[href^="https://"]').forEach((link) => {
      const href = link.getAttribute('href');
      swaps.forEach(([from, to]) => {
        // The origin alone, not a longer host that starts the same.
        if (href.startsWith(from) && /^([/?#]|$)/.test(href.slice(from.length))) {
          link.setAttribute('href', to + href.slice(from.length));
        }
      });
    });
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', rewrite);
  } else {
    rewrite();
  }
})();
