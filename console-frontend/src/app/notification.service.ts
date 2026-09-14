import { Injectable } from '@angular/core';
import '@nldd/design-system/notification';

type Variant = 'neutral' | 'accent' | 'success' | 'warning' | 'critical';

/**
 * Short messages that come over the interface and leave on their own: a save
 * that worked, a request that failed.
 *
 * There is nothing to render here. An nldd-notification moves itself into the
 * one region the design system keeps for them, counts itself down and asks to
 * be removed when it is done, so the only thing this service does is put one in
 * the document and take it out again.
 *
 * That region is a plain fixed element on document.body, and a sheet or modal
 * dialog is a <dialog> opened with showModal() — in the browser's top layer,
 * painting over everything outside it. So every message is raised through
 * raiseRegion, which promotes the region into the top layer as well and keeps a
 * notification visible from inside a sheet.
 */
@Injectable({
  providedIn: 'root',
})
// eslint-disable-next-line import-x/prefer-default-export
export class NotificationService {
  /** The ones this service put there, so clearing them cannot take away a
   *  notification somebody else is responsible for. */
  private readonly open = new Set<HTMLElement>();

  success(text: string, supportingText?: string) {
    this.show(text, 'success', supportingText);
  }

  info(text: string, supportingText?: string) {
    this.show(text, 'neutral', supportingText);
  }

  warning(text: string, supportingText?: string) {
    this.show(text, 'warning', supportingText);
  }

  /** Stays until it is dismissed: a failure is worth reading. */
  error(text: string, supportingText?: string) {
    this.show(text, 'critical', supportingText);
  }

  /** For a caller that is leaving the situation the messages belonged to, and
   *  would otherwise carry them into the next one. */
  dismissAll() {
    this.open.forEach((notification) => notification.remove());
    this.open.clear();
  }

  private show(text: string, variant: Variant, supportingText?: string) {
    const notification = document.createElement('nldd-notification');
    notification.setAttribute('variant', variant);
    notification.setAttribute('text', text);
    if (supportingText) {
      notification.setAttribute('supporting-text', supportingText);
    }
    notification.addEventListener('dismiss', () => {
      this.open.delete(notification);
      notification.remove();
    });
    this.open.add(notification);
    document.body.appendChild(notification);
    NotificationService.raiseRegionWhenItExists();
  }

  /**
   * Raises the region once there is one to raise.
   *
   * A notification moves itself into the region a tick after it is connected —
   * the design system waits deliberately, so it does not pull itself out of a
   * framework's hands mid-render — and the region is built by that move. So at
   * the moment show() returns there is nothing on the page to promote yet, and
   * raising straight away only ever worked for a message that arrived while an
   * earlier one still had the region standing: the first "Token copied" stayed
   * behind the sheet and a second copy was visible.
   *
   * Tried in order rather than waited for: a message that finds the region
   * already up is raised in the same task, and the tick after the move is where
   * the first one of a burst lands. The frame after that is only there so the
   * promotion does not depend on the move being a microtask.
   */
  private static raiseRegionWhenItExists() {
    if (NotificationService.raiseRegion()) return;
    queueMicrotask(() => {
      if (NotificationService.raiseRegion()) return;
      requestAnimationFrame(() => NotificationService.raiseRegion());
    });
  }

  /**
   * Puts the region the notification just moved into in front of an open sheet.
   *
   * The design system parks every notification in one fixed region on
   * document.body. An nldd-sheet is a <dialog> opened with showModal(), which
   * lives in the browser's top layer, and everything outside the top layer
   * paints behind it — so a message raised from inside a sheet was there all
   * along and nobody ever saw it. A popover is in the top layer too, and one
   * shown after a dialog opened stacks above it.
   *
   * Per message rather than once: the region comes and goes with the messages in
   * it, and a sheet opened since the last one would otherwise be on top again.
   * The UA's own popover styling comes along with the promotion and is undone in
   * styles.css.
   */
  private static raiseRegion(): boolean {
    const region = document.getElementById('nldd-notification-region');
    if (!region) return false;
    if (!('showPopover' in region)) return true;
    try {
      if (region.getAttribute('popover') !== 'manual') region.setAttribute('popover', 'manual');
      // Shown again from the top of the stack, not left where it was.
      if (region.matches(':popover-open')) region.hidePopover();
      region.showPopover();
    } catch {
      // A browser that refuses the promotion is left with the region where the
      // design system put it, which is what it was before this existed.
    }
    return true;
  }
}
