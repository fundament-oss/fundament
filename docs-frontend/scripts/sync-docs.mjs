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
import {
  cpSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs';
import { basename, dirname, join, resolve } from 'node:path';
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

/** Hidden files are never content: .DS_Store, editor swap files and the like. */
const isHidden = (path) => basename(path).startsWith('.');

/** Replace `dest` with a copy of `src`, minus hidden files. Safe to re-run. */
function replaceDir(src, dest, filter = () => true) {
  rmSync(dest, { recursive: true, force: true });
  mkdirSync(dirname(dest), { recursive: true });
  cpSync(src, dest, {
    recursive: true,
    filter: (path) => path === src || (!isHidden(path) && filter(path)),
  });
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

/** Rewrite every `ext` file under `dir` in place. Returns the number changed. */
function rewriteDir(dir, ext) {
  let rewritten = 0;
  for (const file of walk(dir, ext)) {
    const before = readFileSync(file, 'utf8');
    const after = rewriteProse(before, REWRITES[ext], CODE_SEGMENTS[ext]);
    if (after !== before) {
      writeFileSync(file, after);
      rewritten += 1;
    }
  }
  return rewritten;
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

  const rewritten =
    rewriteDir(join(root, DOCS_DEST), '.md') +
    rewriteDir(join(root, LIFTED.adr), '.adoc') +
    rewriteDir(join(root, LIFTED.funs), '.adoc');

  process.stdout.write(`synced docs from ${source} (${rewritten} files rewritten)\n`);
}

main();
