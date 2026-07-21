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
