import { defineCollection } from 'astro:content';
import { docsSchema } from '@astrojs/starlight/schema';
import { docsLoader } from '@astrojs/starlight/loaders';
import { asciidocLoader } from '../asciidoc-loader.ts';
import type { LoaderContext } from 'astro/loaders';
import { z } from 'astro:content';

// Hybrid loader that handles both .md/.mdx (via docsLoader) and .adoc (via asciidocLoader)
function hybridLoader() {
  const mdLoader = docsLoader();
  const adocLoader = asciidocLoader({ directory: 'src/content/docs' });

  return {
    name: 'hybrid-loader',
    load: async (context: LoaderContext) => {
      // Load Markdown/MDX files first
      await mdLoader.load(context);
      // Then load AsciiDoc files
      await adocLoader.load(context);
    },
  };
}

export const collections = {
  docs: defineCollection({
    loader: hybridLoader(),
    schema: docsSchema({
      extend: z.object({
        state: z
          .enum(['prediscussion', 'discussion', 'published', 'committed', 'abandoned'])
          .optional(),
      }),
    }),
  }),
};
