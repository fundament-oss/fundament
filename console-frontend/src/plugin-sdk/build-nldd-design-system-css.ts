/**
 * Builds the NLDD Design System plugin-UI stylesheet
 * (public/plugin-ui/nldd-design-system.css) and its fonts (public/plugin-ui/fonts/).
 * Run via: bun src/plugin-sdk/build-nldd-design-system-css.ts
 *
 * Bundles nldd-design-system.css — the NLDD Design System's global.css plus the
 * host's token overrides — into one minified file, and copies the woff2 fonts
 * next to it as files. Not inlined as data: URIs: the plugin CSP (FUN-17) has no
 * font-src, so fonts fall under default-src 'self', which blocks data: URIs and
 * left plugins in a system font. The iframe runs on the plugin-proxy origin that
 * also serves these files, so a relative font URL is 'self'.
 */
import { copyFileSync, mkdirSync, readFileSync, writeFileSync } from 'fs';
import { basename, resolve } from 'path';

const root = resolve(import.meta.dir, '../..');
const outdir = resolve(root, 'public/plugin-ui');
const fontsSrc = resolve(root, 'node_modules/@nldd/design-system/dist/fonts');
const fontsOut = resolve(outdir, 'fonts');

const result = await Bun.build({
  entrypoints: [resolve(root, 'src/plugin-sdk/nldd-design-system.css')],
  outdir,
  naming: { entry: 'nldd-design-system.css' },
  // Left as url()s, which are still relative to the design system's own css/
  // directory, and pointed at the copies below.
  external: ['*.woff2'],
  minify: true,
});

if (!result.success) {
  result.logs.forEach((log) => console.error(log));
  throw new Error('Failed to build nldd-design-system.css');
}

const cssPath = resolve(outdir, 'nldd-design-system.css');
const fonts = new Set<string>();
const css = readFileSync(cssPath, 'utf8').replace(
  /url\(["']?\.\.\/fonts\/([^"')]+\.woff2)["']?\)/g,
  (_, file: string) => {
    fonts.add(file);
    return `url("fonts/${file}")`;
  },
);
if (fonts.size === 0) {
  throw new Error('nldd-design-system.css references no fonts: did the design system move them?');
}
// The minifier unquotes format('woff2-variations'), and unquoted it is no valid
// format keyword: the browser drops the whole src and no font ever loads.
writeFileSync(cssPath, css.replace(/format\(([a-z0-9-]+)\)/g, 'format("$1")'));

mkdirSync(fontsOut, { recursive: true });
fonts.forEach((file) => copyFileSync(resolve(fontsSrc, file), resolve(fontsOut, basename(file))));

console.log(`Built nldd-design-system.css with ${fonts.size} fonts`);
