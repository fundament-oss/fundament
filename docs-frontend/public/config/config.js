// Runtime configuration. A deployment mounts its own over this file (see the
// chart's docs-frontend.yaml); this default is what `bun start` and an image run
// without one get: no test-environment banner.
window.fundamentConfig = { pocUrl: '' };
