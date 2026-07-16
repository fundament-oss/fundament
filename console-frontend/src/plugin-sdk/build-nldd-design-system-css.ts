/**
 * Builds the NLDD Design System plugin-UI stylesheet
 * (public/plugin-ui/nldd-design-system.css).
 * Run via: bun src/plugin-sdk/build-nldd-design-system-css.ts
 *
 * Bundles nldd-design-system.css — the NLDD Design System's global.css plus the
 * host's token overrides — into one minified file and inlines the woff2 fonts as
 * data: URIs: the plugin iframe has an opaque origin, so a separate cross-origin
 * @font-face fetch would need a CORS header the assets don't set.
 */
import { resolve } from 'path';

const root = resolve(import.meta.dir, '../..');

const result = await Bun.build({
  entrypoints: [resolve(root, 'src/plugin-sdk/nldd-design-system.css')],
  outdir: resolve(root, 'public/plugin-ui'),
  naming: { entry: 'nldd-design-system.css' },
  minify: true,
});

if (!result.success) {
  result.logs.forEach((log) => console.error(log));
  throw new Error('Failed to build nldd-design-system.css');
}

console.log('Built nldd-design-system.css');
