// Runtime configuration. A deployment mounts its own over this file (see the
// chart's docs-frontend.yaml); this default is what `bun start` and an image run
// without one get: no test-environment banner, a theme cookie for this host
// only (see theme-cookie.js), and links to the sandbox's Console and marketplace
// (see environment-links.js).
window.fundamentConfig = { pocUrl: '', cookieDomain: '', consoleUrl: '', marketplaceUrl: '' };
