// Presentation-only: keeps a sheet from drawing a keyboard focus ring when the
// deck, not the viewer, was the one holding the keyboard.
//
// nldd-sheet focuses its <dialog> on open — correct, and it stays that way here:
// a sheet nobody has focus inside is unreachable by keyboard and unannounced by a
// screen reader. What it also does is decide whether to *show* the ring, from the
// design system's global input modality: the last ← → Tab Enter Esc marks the
// session as keyboard-driven, a pointerdown marks it as pointer-driven.
//
// While presenting, the arrow keys are aimed at the narration deck, not at the
// console, but the modality tracker listens on document and cannot tell the
// difference. So stepping to a slide whose route opens a sheet rings it, and
// clicking "next" for that same slide does not. The ring is a true statement
// about the keyboard and a false one about the viewer.
//
// Only the ring is dropped, and only while presenting: focus still moves, so the
// sheet is still reachable and still announced.
import isPresenting from './presenting';
import adoptIntoShadowRoots from './shadow-styles';

const FOCUS_RING = new CSSStyleSheet();

// The design system's own rule is `.sheet:focus-visible:not(.is-pointer-focus)`.
// Matching it exactly rather than out-specifying it keeps this a straight
// override: same weight, adopted later, so it wins on order alone and there is
// nothing to re-tune if that rule is ever rewritten. box-shadow carries the
// sheet's drop shadow as well as the ring, so it is restored rather than dropped.
const SUPPRESS_RING = `
  .sheet:focus-visible:not(.is-pointer-focus) {
    outline: none;
    box-shadow: var(--semantics-overlays-box-shadow);
  }
`;

// Empty outside a walkthrough, so the plain console demo keeps the real ring.
function syncRule(): void {
  FOCUS_RING.replaceSync(isPresenting() ? SUPPRESS_RING : '');
}

/**
 * Suppress the sheet focus ring for the length of a walkthrough. Safe to call
 * once at demo boot; a no-op outside the demo build, which never calls it.
 */
export default function enableSheetFocusRingSuppression(): void {
  syncRule();
  // The deck can start and stop mid-session (Esc back to the chooser, a `?present=1`
  // link opened from the plain demo), and the class on <html> is what says which.
  new MutationObserver(syncRule).observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  });

  adoptIntoShadowRoots('nldd-sheet', FOCUS_RING);
}
