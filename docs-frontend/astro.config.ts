import { defineConfig, passthroughImageService } from 'astro/config';
import asciidoc from 'astro-asciidoc';
import type { default as shikiHighlighter } from './shiki-highlighter.js';
import { fileURLToPath } from 'node:url';
import starlight from '@astrojs/starlight';
import tailwindcss from '@tailwindcss/vite';

type ShikiOptions = Parameters<typeof shikiHighlighter>[0];

export default defineConfig({
  // Disable Astro's built-in image optimization in favor of passthrough, see https://docs.astro.build/en/reference/errors/missing-sharp/
  image: {
    service: passthroughImageService(),
  },
  vite: {
    css: {
      postcss: './postcss.config.cjs',
    },

    plugins: [tailwindcss()],

    server: {
      allowedHosts: ['docs.127.0.0.1.nip.io'],
    },
  },
  integrations: [
    asciidoc({
      options: {
        safe: 'server',
        attributes: {
          'source-highlighter': 'shiki',
          'page-layout': fileURLToPath(
            new URL('src/layouts/StarlightAsciidoc.astro', import.meta.url)
          ),
          icons: 'font',
          'icon-set': 'fas', // Use the newer Font Awesome 5+ icon set instead of the deprecated 4 set, see https://docs.asciidoctor.org/pdf-converter/latest/icons/#font
        },
      },
      highlighters: {
        shiki: {
          path: fileURLToPath(new URL('shiki-highlighter.js', import.meta.url)),
          options: {
            themes: ['solarized-light'],
            langs: ['javascript', 'bash', 'asciidoc'],
          } satisfies ShikiOptions,
        },
      },
    }),
    starlight({
      title: 'Fundament Docs',
      logo: {
        src: '/public/img/favicon.svg',
      },
      head: [
        {
          tag: 'link',
          attrs: {
            rel: 'icon',
            type: 'image/png',
            href: '/img/favicon-96x96.png',
            sizes: '96x96',
          },
        },
        {
          tag: 'link',
          attrs: {
            rel: 'icon',
            type: 'image/svg+xml',
            href: '/img/favicon.svg',
          },
        },
        {
          tag: 'link',
          attrs: {
            rel: 'shortcut icon',
            href: '/img/favicon.ico',
          },
        },
        {
          tag: 'link',
          attrs: {
            rel: 'apple-touch-icon',
            sizes: '180x180',
            href: '/img/apple-touch-icon.png',
          },
        },
        {
          tag: 'link',
          attrs: {
            rel: 'manifest',
            href: '/site.webmanifest',
          },
        },
      ],
      components: {
        PageTitle: './src/components/overrides/PageTitle.astro',
      },
      sidebar: [
        {
          label: 'Documentation',
          autogenerate: { directory: 'docs' },
        },
        {
          label: 'FUNs (Fundament Update Notes)',
          autogenerate: { directory: 'funs' },
        },
      ],
      customCss: [
        './src/styles/starlight-custom.css',
        // Icon fonts for Asciidoctor Font Icons
        '@fortawesome/fontawesome-free/css/fontawesome.css',
        '@fortawesome/fontawesome-free/css/solid.css',
      ],
    }),
  ],
});
