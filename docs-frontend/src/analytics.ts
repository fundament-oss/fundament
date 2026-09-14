// Umami, self-hosted on digikluster. Shared by the Starlight `head` config in
// astro.config.ts and by src/layouts/Landing.astro, which sits outside Starlight.
// data-domains keeps `bun start` and `bun run preview` from posting localhost
// hits into the live dashboard, so do not drop it.
export const umamiScriptAttrs = {
  defer: true,
  src: 'https://statistieken.projects.digilab.network/script.js',
  'data-website-id': 'e2f0ef18-4bf8-46ae-970a-e5ac426285b4',
  'data-domains': 'docs.fundament.projects.digilab.network',
};
