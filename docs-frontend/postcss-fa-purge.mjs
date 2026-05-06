// PostCSS plugin that removes unused Font Awesome icon rules from the build.
// @fortawesome/fontawesome-free ships CSS for every icon (~1500+), but we only use a handful.
// Font Awesome is required by Asciidoctor's font-based icon mode (icons: 'font' in astro.config.ts):
//   - Admonitions (NOTE, TIP, WARNING, CAUTION, IMPORTANT) render as <i class="fa icon-{type}">
//   - Inline icon:name[] macros render as <i class="fas fa-{name}">
// FA7 renders glyphs via --fa CSS variable + .fa::before { content: var(--fa) }.
// This plugin scans source files, keeps only .fa-* rules for icons that are actually used,
// and injects .icon-* rules for admonition types that are actually present in the docs.
// Tailwind v4 already tree-shakes its own output, so no Tailwind handling is needed here.
import { globSync } from 'glob';
import { readFileSync } from 'fs';

// Maps Asciidoctor admonition types to their FA7 alias icon names.
// These aliases all exist in fontawesome.css, so we can look up their --fa codepoints dynamically.
const ADMONITION_FA = {
  note: 'info-circle',
  tip: 'lightbulb',
  warning: 'warning',
  caution: 'fire',
  important: 'exclamation-circle',
};

const plugin = () => ({
  postcssPlugin: 'postcss-fa-purge',
  Once(root, { result }) {
    const from = result.opts.from || '';
    // Only fontawesome.css contains .fa-* icon variable rules.
    // solid.css only sets up font-face and style selectors — skip it.
    if (!from.endsWith('fontawesome.css')) return;

    const iconCssVars = new Map();
    const faRules = [];
    root.walkRules((rule) => {
      const m = rule.selector.match(/^\.fa-([a-z0-9-]+)$/);
      if (!m) return;
      rule.walkDecls('--fa', (decl) => iconCssVars.set(m[1], decl.value));
      faRules.push([rule, m[1]]);
    });

    // Scan source files for used FA icons and admonition types.
    const usedFaIcons = new Set();
    const usedAdmonitions = new Set();
    const files = globSync('./src/**/*.{astro,html,js,ts,adoc}');
    for (const file of files) {
      const content = readFileSync(file, 'utf-8');
      for (const m of content.matchAll(/\bfa-([a-z0-9-]+)\b/g)) usedFaIcons.add(m[1]);
      for (const m of content.matchAll(/\bicon:([a-z0-9-]+)\[/g)) usedFaIcons.add(m[1]);
      for (const m of content.matchAll(/^\[(NOTE|TIP|WARNING|CAUTION|IMPORTANT)\]/gm)) {
        usedAdmonitions.add(m[1].toLowerCase());
      }
      for (const m of content.matchAll(/^(NOTE|TIP|WARNING|CAUTION|IMPORTANT):/gm)) {
        usedAdmonitions.add(m[1].toLowerCase());
      }
    }

    // Resolve admonition icons before pruning so their .fa-* rules are preserved.
    for (const admonition of usedAdmonitions) {
      const faName = ADMONITION_FA[admonition];
      if (faName) usedFaIcons.add(faName);
    }

    for (const [rule, iconName] of faRules) {
      if (!usedFaIcons.has(iconName)) rule.remove();
    }

    // Inject .icon-{type} rules; these set --fa so .fa::before { content: var(--fa) } renders the correct glyph.
    for (const admonition of usedAdmonitions) {
      const faName = ADMONITION_FA[admonition];
      const cssVar = faName && iconCssVars.get(faName);
      if (cssVar) root.append(`.icon-${admonition} { --fa: ${cssVar}; }`);
    }
  },
});
plugin.postcss = true;

export default plugin;
