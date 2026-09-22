import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { NotificationService } from './notification.service';

/**
 * The region is the design system's: a notification moves itself into it a tick
 * after it is connected, and the design system carries that region into an open
 * sheet or modal dialog on its own. What is left for the service is putting a
 * notification in the document and taking its own ones back out.
 */

function notifications(): HTMLElement[] {
  return Array.from(document.querySelectorAll('nldd-notification'));
}

describe('NotificationService', () => {
  let service: NotificationService;

  beforeEach(() => {
    service = new NotificationService();
  });

  afterEach(() => {
    service.dismissAll();
    notifications().forEach((notification) => notification.remove());
  });

  it('writes the message onto the notification it puts in the document', () => {
    service.success('Token copied to clipboard', 'It expires in an hour');

    const [notification] = notifications();
    expect(notification).toBeDefined();
    expect(notification.getAttribute('variant')).toBe('success');
    expect(notification.getAttribute('text')).toBe('Token copied to clipboard');
    expect(notification.getAttribute('supporting-text')).toBe('It expires in an hour');
  });

  it('takes back only the messages it raised itself', () => {
    service.info('mine');
    const stranger = document.createElement('nldd-notification');
    document.body.appendChild(stranger);

    service.dismissAll();

    expect(notifications()).toEqual([stranger]);
  });
});
