/**
 * Whether a click asks for the link to open somewhere other than here — another
 * tab, another window, a download — and so belongs to the browser rather than to
 * the router.
 *
 * A row that routes in-app while staying a real `<a href>` (an nldd-list-item
 * with `href`, whose anchor lives in its shadow root and so cannot carry a
 * routerLink) has to make this call before preventing the default. A non-mouse
 * event carries no modifiers and is always ours.
 *
 * The console has the same helper (console-frontend/src/app/opens-elsewhere.ts).
 */
export default function opensElsewhere(event: Event): boolean {
  if (!(event instanceof MouseEvent)) return false;
  return event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0;
}
