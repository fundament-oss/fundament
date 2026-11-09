/**
 * Fails the build when a plugin-UI bundle cannot run as a classic script.
 * Run via: bun src/plugin-sdk/check-classic-scripts.ts
 *
 * Plugin iframes load these bundles with a plain <script> (FUN-18). `bun build
 * --format iife` leaves import.meta in place, which is a syntax error there, and
 * the browser still fires `load` for a script that failed to parse: no element
 * gets registered and plugin forms silently lose their fields. @nldd/design-system
 * reads import.meta.env, which is why its build sets --define 'import.meta.env={}'.
 *
 * Function parses its body as a function, not a module, so it rejects module-only
 * syntax like import.meta. It is not a full <script> parse: it allows a top-level
 * return, for one. node:vm's Script is no substitute: under Bun it accepts import.meta.
 */
import { readdirSync, readFileSync } from 'fs';
import { resolve } from 'path';

const outdir = resolve(import.meta.dir, '../../public/plugin-ui');

const bundles = readdirSync(outdir).filter((file) => file.endsWith('.js'));

bundles.forEach((file) => {
  try {
    Function(readFileSync(resolve(outdir, file), 'utf8'));
  } catch (error) {
    throw new Error(`${file} cannot run as a classic script: ${String(error)}`);
  }
});

console.log(`Checked ${bundles.join(', ')}: all parse as classic scripts`);
