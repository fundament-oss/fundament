#!/usr/bin/env node
/**
 * Sync the repo's `docs/` tree into the Astro content collection and rewrite
 * links that are written for GitHub so they also work on the site.
 *
 * Usage: node scripts/sync-docs.mjs [--source <dir>]
 *
 * Layout produced (all of it gitignored):
 *
 *   docs/user, docs/developer -> src/content/docs/docs/...  served at /docs/...
 *   docs/funs                 -> src/content/docs/funs      served at /funs/...
 *   docs/adr                  -> src/content/docs/adr       served at /adr/...
 *   docs/assets               -> public/assets              served at /assets/...
 *
 * Assets are served statically to avoid Astro/Vite processing large SVGs.
 *
 * The two rewrite passes make links that are written for GitHub work on the
 * site: asset paths become absolute, and relative `./page.md` links lose the
 * extension (Astro serves them extensionless). Prefer that relative form -- it
 * is valid on GitHub and rewritten here.
 *
 * Three cases have no relative form that works in both places and must be
 * written site-absolute (`/docs/...`, `/adr/...`), accepting that they 404 on
 * GitHub:
 *
 *   1. Links to ADRs and FUNs. The rewrite only strips `.md`, and those pages
 *      are `.adoc` moved *out* of docs/ above: GitHub needs
 *      `../adr/0009-x.adoc` while the site needs `../../adr/0009-x`.
 *   2. Links *to* an index.md page. `astro.config.ts` sets trailingSlash
 *      'never', so index.md is served at `/docs/developer/plugins` with no
 *      trailing slash and `../developer/plugins/index` is not a route.
 *   3. Links *from* an index.md page, for the same reason: relative links
 *      resolve against `/docs/developer/` rather than
 *      `/docs/developer/plugins/`.
 *
 * Cases 2 and 3 fail silently in production -- nginx.conf falls back to
 * `/index.html`, so a broken link serves the home page with a 200 rather than
 * a 404. That is what src/links-validator.ts exists to catch: it runs after
 * `astro build` and fails the build on any link that does not resolve.
 */
import {
  cpSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');

/** Subtrees that are lifted out of docs/ into their own content directory. */
const LIFTED = {
  funs: 'src/content/docs/funs',
  adr: 'src/content/docs/adr',
  assets: 'public/assets',
};

/** Everything else lands here, matching the sidebar's `directory: 'docs/...'`. */
const DOCS_DEST = 'src/content/docs/docs';

const REWRITES = [
  // `](../assets/x.svg)` and `](assets/x.svg)` -> `](/assets/x.svg)`
  [/\]\((?:\.\.\/)*assets\//g, '](/assets/'],
  // `](./page.md)` / `](../page.md#frag)` -> `](./page)` / `](../page#frag)`
  [/\]\((\.{1,2}\/[^)]*)\.md([)#])/g, ']($1$2'],
];

function parseArgs(argv) {
  let source = resolve(root, '..', 'docs');
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === '--source') {
      const value = argv[i + 1];
      if (!value) throw new Error('--source requires a directory');
      source = resolve(value);
      i += 1;
    } else {
      throw new Error(`unknown argument: ${argv[i]}`);
    }
  }
  return { source };
}

/** Replace `dest` with a copy of `src`. Idempotent: safe to re-run. */
function replaceDir(src, dest, filter) {
  rmSync(dest, { recursive: true, force: true });
  mkdirSync(dirname(dest), { recursive: true });
  cpSync(src, dest, { recursive: true, ...(filter ? { filter } : {}) });
}

/** Every file under `dir` whose name ends in `ext`. */
function walk(dir, ext, found = []) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) walk(path, ext, found);
    else if (entry.name.endsWith(ext)) found.push(path);
  }
  return found;
}

function main() {
  const { source } = parseArgs(process.argv.slice(2));
  if (!statSync(source, { throwIfNoEntry: false })?.isDirectory()) {
    throw new Error(`source directory not found: ${source}`);
  }

  for (const [name, dest] of Object.entries(LIFTED)) {
    const from = join(source, name);
    if (!statSync(from, { throwIfNoEntry: false })?.isDirectory()) {
      throw new Error(`expected ${from} to exist`);
    }
    replaceDir(from, join(root, dest));
  }

  const lifted = new Set(Object.keys(LIFTED).map((name) => join(source, name)));
  replaceDir(source, join(root, DOCS_DEST), (path) => !lifted.has(path));

  let rewritten = 0;
  for (const file of walk(join(root, DOCS_DEST), '.md')) {
    const before = readFileSync(file, 'utf8');
    const after = REWRITES.reduce((text, [pattern, to]) => text.replace(pattern, to), before);
    if (after !== before) {
      writeFileSync(file, after);
      rewritten += 1;
    }
  }

  process.stdout.write(`synced docs from ${source} (${rewritten} files rewritten)\n`);
}

main();
