// Runtime configuration. A deployment mounts its own over this file (see the
// chart's docs-frontend.yaml); this default is what `bun start` and an image run
// without one get: no test-environment banner, and a theme cookie for this host
// only (see theme-cookie.js).
window.fundamentConfig = { pocUrl: '', cookieDomain: '' };
