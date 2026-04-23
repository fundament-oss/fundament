import { defineRouteMiddleware } from '@astrojs/starlight/route-data';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

interface SidebarLink {
  type: 'link';
  href: string;
  label: string;
}

interface SidebarGroup {
  type: 'group';
  label: string;
  entries: SidebarEntry[];
}

type SidebarEntry = SidebarLink | SidebarGroup;

const labelCache = new Map<string, string | undefined>();

function readMetaLabel(dirSlug: string): string | undefined {
  if (labelCache.has(dirSlug)) return labelCache.get(dirSlug);

  const metaPath = join(process.cwd(), 'src/content/docs', dirSlug, '_meta.yaml');
  let label: string | undefined;

  if (existsSync(metaPath)) {
    const content = readFileSync(metaPath, 'utf-8');
    const match = content.match(/^label:\s*(.+)$/m);
    label = match?.[1]?.trim();
  }

  labelCache.set(dirSlug, label);
  return label;
}

function getAllHrefs(entries: SidebarEntry[]): string[] {
  return entries.flatMap((entry) =>
    entry.type === 'link' ? [entry.href] : getAllHrefs(entry.entries)
  );
}

function commonHrefPrefix(hrefs: string[]): string {
  if (!hrefs.length) return '';
  let prefix: string = hrefs[0]!;
  for (const href of hrefs.slice(1)) {
    while (prefix && !href.startsWith(prefix)) {
      prefix = prefix.slice(0, prefix.lastIndexOf('/'));
    }
  }
  return prefix;
}

function patchSidebarLabels(entries: SidebarEntry[]): void {
  for (const entry of entries) {
    if (entry.type !== 'group') continue;

    const hrefs = getAllHrefs(entry.entries);
    if (hrefs.length > 0) {
      const prefix = commonHrefPrefix(hrefs);
      const dirSlug = prefix.replace(/^\//, '');
      if (dirSlug) {
        const label = readMetaLabel(dirSlug);
        if (label) entry.label = label;
      }
    }

    patchSidebarLabels(entry.entries);
  }
}

export const onRequest = defineRouteMiddleware(({ locals }) => {
  patchSidebarLabels(locals.starlightRoute.sidebar as SidebarEntry[]);
});
