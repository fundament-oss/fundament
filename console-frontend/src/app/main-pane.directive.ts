import { Directive, ElementRef, OnDestroy, OnInit, inject } from '@angular/core';

/** What `nldd-page` reads `landmarks="auto"` off: a page inside one of these renders a region. */
const PAGE_CONTAINERS =
  'nldd-split-view-pane, nldd-sheet, nldd-modal-dialog, nldd-window, nldd-popover';

const holdsPage = (node: Node): boolean =>
  node instanceof Element &&
  (node.localName === 'nldd-page' || node.getElementsByTagName('nldd-page').length > 0);

/**
 * Makes the page in this pane the document's own: its header the banner, its content the main
 * landmark.
 *
 * A page in a pane of a split view renders a region, because the panes put pages beside each
 * other, and only the app knows which pane holds the primary content. The pages that fill this
 * pane come from routed components, which do not know where they are rendered, so the pane marks
 * them instead: every page whose nearest pane or overlay is this one. A page in an overlay inside
 * it stays a region, and so does a page that already says what it is.
 */
@Directive({
  selector: 'nldd-split-view-pane[appMainPane]',
})
export default class MainPaneDirective implements OnInit, OnDestroy {
  private el = inject<ElementRef<HTMLElement>>(ElementRef);

  // A route swaps the page without the pane noticing, so it has to watch the whole subtree. Most
  // of what changes there is a page's own content, like a log streaming in, so only a batch that
  // brings a page is worth looking through the pane for.
  private observer = new MutationObserver((records) => {
    if (records.some((record) => Array.from(record.addedNodes).some(holdsPage))) this.markPages();
  });

  ngOnInit(): void {
    this.observer.observe(this.el.nativeElement, { childList: true, subtree: true });
    this.markPages();
  }

  ngOnDestroy(): void {
    this.observer.disconnect();
  }

  private markPages(): void {
    const pane = this.el.nativeElement;
    pane.querySelectorAll('nldd-page:not([landmarks])').forEach((page) => {
      if (page.parentElement?.closest(PAGE_CONTAINERS) === pane) {
        page.setAttribute('landmarks', 'page');
      }
    });
  }
}
