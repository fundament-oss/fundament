import { InjectionToken } from '@angular/core';

/**
 * True only in the demo/presentation build. Production never provides this token,
 * so it defaults to false and the presentation feature stays inert.
 */
export const PRESENTATION_ENABLED = new InjectionToken<boolean>('PRESENTATION_ENABLED', {
  factory: () => false,
});

/**
 * Demo-only event dispatched on `document` to reset the in-memory plugin installs
 * back to their seeded state. The fake install service reseeds on it and the plugins
 * page re-fetches, so the walkthrough's install slide can be replayed even after Cert
 * Manager was already installed on an earlier pass. Never fires in production.
 */
export const PLUGIN_INSTALLS_RESET_EVENT = 'demo:reset-plugin-installs';

/**
 * Demo-only event dispatched on `document` to make sure every plugin the console
 * has a UI for is installed and running. The slides that show an installed plugin
 * come after the install slide, but a deck is also deep-linked (`?slide=`),
 * restarted and stepped through backwards — so those slides put the state they
 * describe in place themselves instead of depending on an earlier slide's drive.
 * A no-op when the plugin is already installed. Never fires in production.
 */
export const PLUGIN_INSTALLS_ENSURE_EVENT = 'demo:ensure-plugin-installs';

/**
 * Base path of the marketplace's demo bundle inside the console demo's own
 * output. `bun scripts/build-marketplace-demo.ts` writes it to
 * public/marketplace/, which Angular copies into the build, so the walkthrough
 * can frame it same-origin: the demo is served with `frame-ancestors 'self'`
 * and drive scripts reach into the frame's document.
 */
export const MARKETPLACE_EMBED_BASE = '/marketplace/';

/**
 * postMessage contract with the embedded marketplace demo, whose other half
 * lives in marketplace-frontend/src/main.demo.ts. The two apps share no code,
 * so keep the strings in step.
 *
 * `navigate` moves the frame to another marketplace route without reloading it;
 * `ready` is the frame telling the deck that Angular has bootstrapped, which
 * happens well after the iframe's own `load` event.
 */
export const EMBED_NAVIGATE_MESSAGE = 'fundament-demo:navigate';

export const EMBED_READY_MESSAGE = 'fundament-demo:ready';
