import { InjectionToken } from '@angular/core';

/**
 * True only in the demo/presentation build. Production never provides this token,
 * so it defaults to false and the presentation feature stays inert.
 */
export const PRESENTATION_ENABLED = new InjectionToken<boolean>('PRESENTATION_ENABLED', {
  factory: () => false,
});
