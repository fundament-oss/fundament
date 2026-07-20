/**
 * True while the walkthrough overlay is running. Read from the `presenting` class
 * that PresentationService puts on <html> rather than from the service itself, so
 * console code can check it without pulling the demo-only feature into its bundle.
 *
 * The overlay owns the keyboard (← → Esc), so anything that would steal focus into
 * a field on render must stay put while this is true.
 */
export function isPresenting(): boolean {
  return document.documentElement.classList.contains('presenting');
}
