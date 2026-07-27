import { defineConfig } from 'vitest/config';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('.', import.meta.url));
const entry = (name: string) => fileURLToPath(new URL(`${name}.html`, import.meta.url));

// The plugin serves this app same-origin under /console/, and console.go's
// go:embed console/* compiles the build output into the plugin binary. The NLDD
// Design System is NOT bundled: the app uses <nldd-*> tags whose registrations
// come from the shared /plugin-ui/nldd-design-system.js. See docs/funs/FUN-18.adoc.
export default defineConfig({
  root,
  base: './',
  build: {
    outDir: fileURLToPath(new URL('../console', import.meta.url)),
    emptyOutDir: true,
    rollupOptions: {
      // Output filename must match definition.yaml's customComponents.Gateway.create.
      input: {
        'gateways-create': entry('gateways-create'),
      },
    },
  },
  test: {
    environment: 'happy-dom',
    include: ['src/**/*.test.ts'],
  },
});
