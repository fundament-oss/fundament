/// <reference path="../.astro/types.d.ts" />
/// <reference types="astro-asciidoc/types" />

// Defined by public/theme-cookie.js, loaded before first paint.
interface Window {
  fundamentTheme: {
    choice(): 'auto' | 'dark' | 'light';
    store(choice: 'auto' | 'dark' | 'light'): void;
    syncStarlight(): 'auto' | 'dark' | 'light';
  };
}
