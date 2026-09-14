// Data model for the walkthrough/presentation overlay (demo build only).

import { Localized } from './i18n';

export interface DriveStep {
  /** Pause this many milliseconds before continuing. */
  wait?: number;
  /** CSS selector (within the app pane) of the element to act on. */
  set?: string;
  /** Value to apply. */
  value?: string;
  /** Type `value` into `set` character by character (visible typing). */
  type?: boolean;
  /** Treat `set` as a native <select>: assign value and dispatch `change`. */
  select?: boolean;
  /** Treat `set` as an nldd checkbox: dispatch `change` with `detail.checked`. */
  check?: boolean;
  /** CSS selector of an element to click. */
  click?: string;
  /** CSS selector of a form to submit (dispatches a native `submit` event). */
  submit?: string;
  /** Dispatch a bubbling CustomEvent of this name on `document` (demo services listen). */
  emit?: string;
}

export interface Slide {
  id: string;
  kind?: 'opening' | 'closing' | 'normal';
  title: Localized;
  lead?: Localized;
  bullets?: Localized[];
  aside?: Localized;
  /** Route the app pane navigates to for this slide. Omit for full-bleed slides. */
  route?: string;
  /** Full-bleed slide: hide the app and let the narration panel fill the screen. */
  full?: boolean;
  /** Optional slide: skipped during navigation when "skip optional" (o) is on. */
  skippable?: boolean;
  /** Prominent link rendered under the slide body (e.g. on a closing slide). */
  link?: { url: string; label?: Localized };
  /**
   * Path inside the embedded marketplace demo, e.g. '/plugins/pl-cert-manager'.
   * Mutually exclusive with `route`: the app pane frames the marketplace rather
   * than the console, and the drive script runs against the frame's document.
   */
  embed?: string;
  /** Auto-drive script executed after the slide's route has rendered. */
  drive?: DriveStep[];
}

/** A named character whose tour walks the console from one role's point of view. */
export interface Persona {
  /** Proper noun — the same in every locale. */
  name: string;
  role: Localized;
  /** One line on the chooser card, addressing the viewer directly ("je"/"you"). */
  blurb: Localized;
}

export interface Tour {
  id: string;
  title: Localized;
  lead?: Localized;
  /** Icon on the chooser card: an SVG path `d`, stroked in a 24×24 viewBox. */
  icon?: string;
  /** Set when the tour is told through a character; groups it under "word een rol". */
  persona?: Persona;
  slides: Slide[];
}
