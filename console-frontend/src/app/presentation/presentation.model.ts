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

/** A named character whose tour walks the console from one role's point of view. */
export interface Persona {
  name: string;
  role: string;
  /** One line on the chooser card, addressing the viewer as "je". */
  blurb: string;
}

export interface Tour {
  id: string;
  title: string;
  lead?: string;
  /** Icon on the chooser card: an SVG path `d`, stroked in a 24×24 viewBox. */
  icon?: string;
  /** Set when the tour is told through a character; groups it under "word een rol". */
  persona?: Persona;
  slides: Slide[];
}
