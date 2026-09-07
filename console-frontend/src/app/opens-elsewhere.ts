/**
 * Whether a click asks for the link to open somewhere other than here — another
 * tab, another window, a download — and so belongs to the browser rather than to
 * the router.
 *
 * Every row and menu item that routes in-app while staying a real `<a href>` has
 * to make this call, and each one that spelled the condition out was a chance to
 * forget a modifier: several had dropped alt, one had dropped the middle button.
 * A non-mouse event (a menu item reports a plain Event, and so does the keyboard)
 * carries no modifiers and is always ours.
 */
export default function opensElsewhere(event: Event): boolean {
  if (!(event instanceof MouseEvent)) return false;
  return (
    event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0
  );
}
