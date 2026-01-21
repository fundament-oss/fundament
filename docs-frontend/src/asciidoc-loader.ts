import type { Loader, LoaderContext } from 'astro/loaders';
import { readdir, readFile, stat } from 'node:fs/promises';
import { join, relative } from 'node:path';
import Asciidoctor from '@asciidoctor/core';
import { fileURLToPath } from 'node:url';

const asciidoctor = Asciidoctor();

interface AsciidocLoaderOptions {
  directory: string;
}

interface AsciidocAttributes {
  title?: string;
  description?: string;
  state?: string;
  'sidebar-label'?: string;
  'sidebar-order'?: string;
  'sidebar-hidden'?: string | boolean;
  [key: string]: unknown;
}

async function* walkDirectory(dir: string): AsyncGenerator<string> {
  const entries = await readdir(dir, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = join(dir, entry.name);
    if (entry.isDirectory()) {
      yield* walkDirectory(fullPath);
    } else if (entry.isFile() && entry.name.endsWith('.adoc')) {
      yield fullPath;
    }
  }
}

export function asciidocLoader(options: AsciidocLoaderOptions): Loader {
  return {
    name: 'asciidoc-loader',
    load: async (context: LoaderContext) => {
      const { directory } = options;
      const baseDir = fileURLToPath(new URL(directory, `file://${process.cwd()}/`));

      context.logger.info(`Loading AsciiDoc files from ${baseDir}`);

      // Find all .adoc files
      for await (const filePath of walkDirectory(baseDir)) {
        try {
          const content = await readFile(filePath, 'utf-8');
          const stats = await stat(filePath);

          // Parse AsciiDoc
          // Note: We set 'showtitle' to false because Starlight displays the title separately
          const doc = asciidoctor.load(content, {
            safe: 'server',
            attributes: {
              showtitle: false, // Don't include title in rendered output
              'source-highlighter': 'shiki',
              notitle: true, // Prevent title rendering
            },
          });

          // Extract data from AsciiDoc attributes
          const attributes = doc.getAttributes() as AsciidocAttributes;
          const title = doc.getDocumentTitle()?.toString() || attributes.title || '';
          const description = attributes.description || '';
          const state = attributes.state || undefined;

          // Support custom sidebar attributes
          const sidebarLabel = attributes['sidebar-label'] || undefined;
          const sidebarOrder = attributes['sidebar-order']
            ? parseInt(attributes['sidebar-order'], 10)
            : undefined;
          const sidebarHidden =
            attributes['sidebar-hidden'] === 'true' || attributes['sidebar-hidden'] === true;

          // Generate slug from file path (lowercase to match glob behavior)
          const slug = relative(baseDir, filePath)
            .replace(/\.adoc$/, '')
            .replace(/\\/g, '/')
            .toLowerCase();

          // Get relative path from project root for filePath
          const relativeFilePath = relative(process.cwd(), filePath);

          // Extract headings from sections
          const sections = doc.getSections();
          const headings = sections.map((section) => ({
            depth: section.getLevel() + 1, // Starlight expects h2=depth 2
            slug: section.getId() || '',
            text: section.getTitle() || '',
          }));

          // Convert to HTML
          const html = doc.convert({ standalone: false }) as string;

          // Store the entry with rendered HTML in body
          // We wrap it in a div to ensure proper rendering
          const bodyContent = `<div class="asciidoc-content">\n${html}\n</div>`;

          context.store.set({
            id: slug,
            data: {
              title,
              description,
              state,
              editUrl: false,
              head: [],
              pagefind: true,
              draft: false,
              sidebar: {
                label: sidebarLabel,
                order: sidebarOrder,
                hidden: sidebarHidden,
              },
            },
            // Store the HTML in body - Starlight will render this
            body: bodyContent,
            rendered: {
              html: bodyContent,
              metadata: {
                headings,
                imagePaths: [],
                frontmatter: {
                  title,
                  description,
                },
              },
            },
            filePath: relativeFilePath,
            digest: stats.mtime.getTime(),
          });

          context.logger.info(`Loaded: ${slug}`);
        } catch (error) {
          context.logger.error(`Failed to load ${filePath}: ${error}`);
        }
      }
    },
  };
}
