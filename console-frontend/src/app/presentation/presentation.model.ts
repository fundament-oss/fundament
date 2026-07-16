// Data model for the walkthrough/presentation overlay (demo build only).

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
  /** CSS selector of an element to click. */
  click?: string;
  /** CSS selector of a form to submit (dispatches a native `submit` event). */
  submit?: string;
}

export interface Slide {
  id: string;
  kind?: 'opening' | 'closing' | 'normal';
  title: string;
  lead?: string;
  bullets?: string[];
  aside?: string;
  /** Route the app pane navigates to for this slide. Omit for full-bleed slides. */
  route?: string;
  /** Full-bleed slide: hide the app and let the narration panel fill the screen. */
  full?: boolean;
  /** Optional slide: skipped during navigation when "skip optional" (o) is on. */
  skippable?: boolean;
  /** Prominent link rendered under the slide body (e.g. on a closing slide). */
  link?: { url: string; label?: string };
  /** Auto-drive script executed after the slide's route has rendered. */
  drive?: DriveStep[];
}

export interface Tour {
  id: string;
  title: string;
  lead?: string;
  slides: Slide[];
}
