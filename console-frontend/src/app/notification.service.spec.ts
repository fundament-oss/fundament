import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { NotificationService } from './notification.service';

/**
 * The region is the design system's, built by the first notification that moves
 * into it — a tick after the notification is connected, not while show() runs.
 * These tests are about the promotion into the top layer landing on it anyway,
 * which is what keeps a message in front of an open sheet.
 */

const REGION_ID = 'nldd-notification-region';

interface PopoverStub {
  showPopover(this: HTMLElement): void;
  hidePopover(this: HTMLElement): void;
  matches(this: HTMLElement, selector: string): boolean;
}

let shown: string[] = [];

/**
 * happy-dom has no top layer. The stub records the calls the browser would act
 * on and tracks open-ness itself, because `:popover-open` is not a selector it
 * can match either.
 */
function stubPopover() {
  const proto = HTMLElement.prototype as unknown as PopoverStub;
  const open = new WeakSet<HTMLElement>();
  const originalMatches = proto.matches;
  proto.showPopover = function showPopover(this: HTMLElement) {
    open.add(this);
    shown.push(this.id);
  };
  proto.hidePopover = function hidePopover(this: HTMLElement) {
    open.delete(this);
  };
  proto.matches = function matches(this: HTMLElement, selector: string) {
    if (selector === ':popover-open') return open.has(this);
    return originalMatches.call(this, selector);
  };
  return () => {
    delete (proto as Partial<PopoverStub>).showPopover;
    delete (proto as Partial<PopoverStub>).hidePopover;
    proto.matches = originalMatches;
  };
}

function region(): HTMLElement | null {
  return document.getElementById(REGION_ID);
}

/** Waits for the design system's own move, which does not happen in show(). */
async function settled(): Promise<void> {
  await customElements.whenDefined('nldd-notification');
  for (let i = 0; i < 10; i += 1) {
    // eslint-disable-next-line no-await-in-loop
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });
    if (region()) return;
  }
}

describe('NotificationService', () => {
  let restore: () => void;
  let service: NotificationService;

  beforeEach(() => {
    shown = [];
    restore = stubPopover();
    service = new NotificationService();
  });

  afterEach(() => {
    service.dismissAll();
    region()?.remove();
    restore();
  });

  // The bug: the first "Token copied to clipboard" from inside a sheet was
  // raised before the region existed, so nothing was promoted and the message
  // sat behind the sheet. Copying a second time found the region the first one
  // had built, and was visible — which is how this surfaced.
  it('raises the region of the very first message', async () => {
    service.success('Token copied to clipboard');
    await settled();

    expect(region()).not.toBeNull();
    expect(region()?.getAttribute('popover')).toBe('manual');
    expect(shown).toContain(REGION_ID);
  });

  it('raises it again per message, so a sheet opened since is behind it', async () => {
    service.success('first');
    await settled();
    service.success('second');
    await settled();

    expect(shown.filter((id) => id === REGION_ID)).toHaveLength(2);
  });
});
