/**
 * Builds the NLDS plugin-UI stylesheet (public/plugin-ui/nldd.css).
 * Run via: bun src/plugin-sdk/build-nldd-css.ts
 *
 * Bundles @nldd/design-system's global.css into one minified file and inlines the
 * woff2 fonts as data: URIs — the plugin iframe has an opaque origin, so a separate
 * cross-origin @font-face fetch would need a CORS header the assets don't set.
 */
import { resolve } from 'path';

const root = resolve(import.meta.dir, '../..');

const result = await Bun.build({
  entrypoints: [resolve(root, 'node_modules/@nldd/design-system/dist/css/global.css')],
  outdir: resolve(root, 'public/plugin-ui'),
  naming: { entry: 'nldd.css' },
  minify: true,
});

if (!result.success) {
  for (const log of result.logs) console.error(log);
  throw new Error('Failed to build nldd.css');
}

console.log('Built nldd.css');
