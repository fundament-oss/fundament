#!/usr/bin/env node
/**
 * Sync the repo's `docs/` tree into the Astro content collection and rewrite
 * links that are written for GitHub so they also work on the site.
 *
 * Usage: node scripts/sync-docs.mjs [--source <dir>] [--watch]
 *
 * --watch syncs again whenever a file under the source changes.
 *
 * Layout produced (all of it gitignored):
 *
 *   docs/user, docs/developer -> src/content/docs/docs/...  served at /docs/...
 *   docs/funs                 -> src/content/docs/funs      served at /funs/...
 *   docs/adr                  -> src/content/docs/adr       served at /adr/...
 *   docs/assets               -> public/assets              served at /assets/...
 *
 * Assets are served statically to avoid Astro/Vite processing large SVGs.
 * Hidden files are skipped, so editor and OS droppings (.DS_Store) never reach
 * the content collection or the published image.
 *
 * The rewrite passes make links that are written for GitHub work on the site.
 * Prefer the relative form everywhere: it is valid on GitHub and rewritten
 * here.
 *
 *   Markdown: `](./page.md)`, `](page.md)` and `](../page.md#frag)` lose the
 *   extension (Astro serves pages extensionless), and `](assets/x.svg)` at any
 *   depth becomes absolute.
 *
 *   AsciiDoc: `link:template.adoc[]` and `link:../funs/FUN-7.adoc[]` lose the
 *   extension and are lowercased to match the slugs asciidoc-loader.ts
 *   generates. ADR and FUN pages sit one level below the site root exactly as
 *   they sit one level below docs/, so a relative link between them resolves
 *   the same way in both places.
 *
 * Rewrites skip code blocks and inline code spans: a link inside a sample is
 * content to be shown verbatim, not navigation.
 *
 * Three cases have no relative form that works in both places and must be
 * written site-absolute (`/docs/...`, `/adr/...`), accepting that they 404 on
 * GitHub:
 *
 *   1. Links from a Markdown page to an ADR or FUN. Those are `.adoc` pages
 *      moved *out* of docs/ above, so the depth differs on either side: GitHub
 *      needs `../adr/0009-x.adoc` from docs/user/, the site needs
 *      `../../adr/0009-x` from /docs/user/. (AsciiDoc-to-AsciiDoc links are
 *      not affected -- see above.)
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
import { mkdirSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { basename, dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');

/** Subtrees that are lifted out of docs/ into their own content directory. */
const LIFTED = {
  funs: 'src/content/docs/funs',
  adr: 'src/content/docs/adr',
  assets: 'public/assets',
};

/** Subtrees that are internal working notes, never site content. */
const EXCLUDED = ['superpowers'];

/** Everything else lands here, matching the sidebar's `directory: 'docs/...'`. */
const DOCS_DEST = 'src/content/docs/docs';

/**
 * A link target that is already site-absolute (`/docs/...`) or carries a scheme
 * (`https:`, `mailto:`) is written for the site as-is and must be left alone.
 */
const NOT_RELATIVE = String.raw`(?!\/|[a-z][a-z0-9+.-]*:)`;

const REWRITES = {
  // `](assets/x.svg)`, `](./assets/x.svg)`, `](../../assets/x.svg)` -> `](/assets/x.svg)`
  '.md': [
    [/\]\((?:\.{1,2}\/)*assets\//g, '](/assets/'],
    // `](./page.md)` / `](page.md#frag)` -> `](./page)` / `](page#frag)`
    [new RegExp(String.raw`\]\(${NOT_RELATIVE}([^)\s]*?)\.md([)#\s])`, 'g'), ']($1$2'],
  ],
  // `link:../funs/FUN-7.adoc[]` -> `link:../funs/fun-7[]`; `xref:` likewise.
  // Lowercased because asciidoc-loader.ts lowercases every slug it generates.
  '.adoc': [
    [
      new RegExp(String.raw`\b(link|xref):${NOT_RELATIVE}([^[\s#]+)\.adoc(#[^[\s]*)?\[`, 'g'),
      (_match, macro, target, fragment) => `${macro}:${target.toLowerCase()}${fragment ?? ''}[`,
    ],
  ],
};

/**
 * Spans whose contents are samples rather than navigation: fenced/delimited
 * blocks and inline code. Rewrites skip these, so a documented link stays
 * verbatim. One capture group, so `String.split` yields prose at even indices
 * and code at odd ones.
 */
const CODE_SEGMENTS = {
  '.md': /(^[ \t]*```[\s\S]*?^[ \t]*```[^\n]*$|^[ \t]*~~~[\s\S]*?^[ \t]*~~~[^\n]*$|`[^`\n]*`)/gm,
  '.adoc': /(^-{4,}[ \t]*$[\s\S]*?^-{4,}[ \t]*$|^\.{4,}[ \t]*$[\s\S]*?^\.{4,}[ \t]*$|`[^`\n]*`)/gm,
};

function parseArgs(argv) {
  let source = resolve(root, '..', 'docs');
  let watch = false;
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === '--source') {
      const value = argv[i + 1];
      if (!value) throw new Error('--source requires a directory');
      source = resolve(value);
      i += 1;
    } else if (argv[i] === '--watch') {
      watch = true;
    } else {
      throw new Error(`unknown argument: ${argv[i]}`);
    }
  }
  return { source, watch };
}

/** Hidden files are never content: .DS_Store, editor swap files and the like. */
const isHidden = (path) => basename(path).startsWith('.');

/**
 * Make `dest` match `src`, minus hidden files, rewriting files whose extension has
 * rewrites. Only files whose content differs are written and only stale ones
 * removed, so a dev server watching `dest` keeps its watches. Returns the number
 * of files written.
 */
function mirrorDir(src, dest, filter = () => true) {
  const wanted = new Set();
  let written = 0;
  const visit = (dir) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name);
      if (isHidden(path) || !filter(path)) continue;
      const target = join(dest, relative(src, path));
      // A source path that switched between file and directory leaves the old type in
      // `dest`, which would make every later sync throw; remove it first.
      const existing = statSync(target, { throwIfNoEntry: false });
      if (entry.isDirectory()) {
        if (existing && !existing.isDirectory()) rmSync(target);
        visit(path);
        continue;
      }
      if (existing?.isDirectory()) rmSync(target, { recursive: true });
      wanted.add(target);
      let content = readFileSync(path);
      const ext = Object.keys(REWRITES).find((e) => entry.name.endsWith(e));
      if (ext)
        content = Buffer.from(
          rewriteProse(content.toString('utf8'), REWRITES[ext], CODE_SEGMENTS[ext])
        );
      const current = existing?.isFile() ? readFileSync(target) : null;
      if (current && current.equals(content)) continue;
      mkdirSync(dirname(target), { recursive: true });
      writeFileSync(target, content);
      written += 1;
    }
  };
  visit(src);
  removeStale(dest, wanted);
  return written;
}

/** Remove files under `dir` that are not in `wanted`, then directories left empty. */
function removeStale(dir, wanted) {
  if (!statSync(dir, { throwIfNoEntry: false })?.isDirectory()) return;
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      removeStale(path, wanted);
      if (readdirSync(path).length === 0) rmSync(path, { recursive: true });
    } else if (!wanted.has(path)) {
      rmSync(path);
    }
  }
}

/** Apply `rewrites` to the prose of `text`, leaving code spans untouched. */
function rewriteProse(text, rewrites, codeSegments) {
  return text
    .split(codeSegments)
    .map((segment, index) =>
      index % 2 === 1
        ? segment
        : rewrites.reduce((prose, [pattern, to]) => prose.replace(pattern, to), segment)
    )
    .join('');
}

/** Path, size and modification time of every file under `dir`; changes when any file does. */
function fingerprint(dir) {
  const parts = [];
  const visit = (current) => {
    for (const entry of readdirSync(current, { withFileTypes: true })) {
      const path = join(current, entry.name);
      if (entry.isDirectory()) {
        visit(path);
      } else {
        const { size, mtimeMs } = statSync(path);
        parts.push(`${path}:${size}:${mtimeMs}`);
      }
    }
  };
  visit(dir);
  return parts.join('\n');
}

function sync(source) {
  let written = 0;
  for (const [name, dest] of Object.entries(LIFTED)) {
    const from = join(source, name);
    if (!statSync(from, { throwIfNoEntry: false })?.isDirectory()) {
      throw new Error(`expected ${from} to exist`);
    }
    written += mirrorDir(from, join(root, dest));
  }

  const skipped = new Set([...Object.keys(LIFTED), ...EXCLUDED].map((name) => join(source, name)));
  written += mirrorDir(source, join(root, DOCS_DEST), (path) => !skipped.has(path));

  process.stdout.write(`synced docs from ${source} (${written} files written)\n`);
}

function main() {
  const { source, watch } = parseArgs(process.argv.slice(2));
  if (!statSync(source, { throwIfNoEntry: false })?.isDirectory()) {
    throw new Error(`source directory not found: ${source}`);
  }

  sync(source);
  if (!watch) return;

  let last = fingerprint(source);
  // A file skaffold writes or deletes mid-scan makes fingerprint or sync throw; the next tick retries.
  setInterval(() => {
    try {
      const current = fingerprint(source);
      if (current === last) return;
      sync(source);
      last = current;
    } catch (err) {
      process.stderr.write(`sync failed: ${err.message}\n`);
    }
  }, 1000);
}

main();
