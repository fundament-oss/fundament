// `vitest/config` re-exports Vite's defineConfig with the `test` block typed.
import { defineConfig } from 'vitest/config';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('.', import.meta.url));
const entry = (name: string) => fileURLToPath(new URL(`${name}.html`, import.meta.url));

// The plugin serves this app same-origin under /console/, and console.go's
// `go:embed console/*` compiles the build output into the plugin binary. The NLDD
// Design System is intentionally NOT bundled: the app uses <nldd-*> tags whose
// registrations come from the shared /plugin-ui/nldd-design-system.js, loaded at
// runtime via loadNlddDesignSystem(). See docs/funs/FUN-18.adoc.
export default defineConfig({
  root,
  // Relative asset URLs so the built HTML resolves ./assets/* under /console/,
  // independent of the mount path.
  base: './',
  build: {
    // Output into the go:embed'd dir. It lives outside the Vite root, so
    // emptyOutDir must be explicit. The build script re-creates .gitkeep after,
    // so `go build`/`go test` still find a file when the UI hasn't been built.
    outDir: fileURLToPath(new URL('../console', import.meta.url)),
    emptyOutDir: true,
    rollupOptions: {
      // Multi-page: one entry per host-navigated view. Output filenames must
      // match definition.yaml's customComponents (widgets-<view>.html).
      input: {
        'widgets-list': entry('widgets-list'),
        'widgets-detail': entry('widgets-detail'),
      },
    },
  },
  test: {
    environment: 'happy-dom',
    include: ['src/**/*.test.ts'],
  },
});
