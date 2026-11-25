import type { NotificationService } from '../notification.service';
import copyToClipboard from './clipboard';

/**
 * Copies a value and says so, naming what was copied ("User ID copied"): a
 * menu closes the moment it is used, so the notification is the only sign that
 * anything happened.
 */
export default async function copyWithNotice(
  notifications: NotificationService,
  text: string,
  what: 'User ID' | 'Email address',
): Promise<void> {
  try {
    await copyToClipboard(text);
    notifications.success(`${what} copied`);
  } catch {
    notifications.error(`${what} not copied`, 'Select it in the member details and copy it there.');
  }
}
