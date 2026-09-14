/**
 * Builds the marketplace's demo bundle and drops it into public/marketplace/,
 * so `build:demo` ships it as a static asset of the console demo.
 *
 * The walkthrough shows the Plugin Marketplace in an iframe. That frame has to
 * be same-origin for two reasons: the console demo is served with
 * `frame-ancestors 'self'` and `X-Frame-Options: DENY`, and slide drive scripts
 * reach into the frame's document to type and click. One origin means one
 * build output, which is what this script assembles.
 *
 * Run via: bun scripts/build-marketplace-demo.ts (from the prebuild:demo hook).
 */
import { rmSync, cpSync, existsSync } from 'fs';
import { resolve } from 'path';

const consoleRoot = resolve(import.meta.dir, '..');
const marketplaceRoot = resolve(consoleRoot, '../marketplace-frontend');
const source = resolve(marketplaceRoot, 'dist/fundament-marketplace/browser');
const target = resolve(consoleRoot, 'public/marketplace');

if (!existsSync(marketplaceRoot)) {
  throw new Error(`marketplace-frontend not found at ${marketplaceRoot}`);
}

function run(command: string[], cwd: string): void {
  const result = Bun.spawnSync(command, { cwd, stdout: 'inherit', stderr: 'inherit' });
  if (result.exitCode !== 0) {
    throw new Error(`${command.join(' ')} failed with exit code ${result.exitCode}`);
  }
}

// The marketplace is a separate package with its own lockfile, so it needs its
// own install before it can be built. Skipped when node_modules is already
// there, which is the common case in local development.
if (!existsSync(resolve(marketplaceRoot, 'node_modules'))) {
  run(['bun', 'install', '--frozen-lockfile'], marketplaceRoot);
}

run(['bun', 'run', 'build:demo'], marketplaceRoot);

// Replace rather than merge: a stale hashed chunk left behind by an earlier
// build would be served but never referenced.
rmSync(target, { recursive: true, force: true });
cpSync(source, target, { recursive: true });

// eslint-disable-next-line no-console
console.log(`marketplace demo bundle -> ${target}`);
