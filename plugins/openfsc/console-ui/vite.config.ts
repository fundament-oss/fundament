// `vitest/config` re-exports Vite's defineConfig with the `test` block typed.
import { defineConfig } from 'vitest/config';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('.', import.meta.url));
const entry = (name: string) => fileURLToPath(new URL(`${name}.html`, import.meta.url));

// The plugin serves this app same-origin under /console/, and console.go's
// `go:embed console/*` compiles the build output into the plugin binary. NLDS is
// intentionally NOT bundled: the app uses <nldd-*> tags whose registrations come
// from the shared /plugin-ui/nldd.js, loaded at runtime via loadNlds(). See
// docs/funs/FUN-18.adoc.
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
      // match definition.yaml's customComponents (fscinstallations-<view>.html).
      input: {
        'fscinstallations-create': entry('fscinstallations-create'),
        'fscinstallations-list': entry('fscinstallations-list'),
        'fscinstallations-detail': entry('fscinstallations-detail'),
      },
    },
  },
  server: {
    // `just openfsc::console-dev` runs the console-preview backend (stand-in SDK
    // + /api/* + the shared /plugin-ui bundle) on :4319; Vite proxies to it so
    // HMR previews against the live cluster, exactly like the built preview.
    proxy: {
      '/api': 'http://localhost:4319',
      '/plugin-ui': 'http://localhost:4319',
    },
  },
  test: {
    // The form logic in src/form.ts is pure DOM work (no NLDS runtime, no network),
    // so a lightweight DOM is all it needs.
    environment: 'happy-dom',
    include: ['src/**/*.test.ts'],
  },
});
