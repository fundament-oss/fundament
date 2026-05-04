import type { StarlightPlugin } from '@astrojs/starlight/types';

export default function sidebarMetaPlugin(): StarlightPlugin {
  return {
    name: 'sidebar-meta',
    hooks: {
      'config:setup': ({ addRouteMiddleware, command }) => {
        if (command !== 'dev' && command !== 'build') return;
        addRouteMiddleware({ entrypoint: './src/sidebar-meta-middleware.ts', order: 'post' });
      },
    },
  };
}
