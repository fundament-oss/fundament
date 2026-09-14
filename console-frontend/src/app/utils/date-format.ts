import { timestampDate, type Timestamp } from '@bufbuild/protobuf/wkt';

/**
 * Formats a date (from Timestamp or string) to a localized date string.
 * @param value - The date value (Timestamp, string, or undefined)
 * @param fallback - The fallback string to return if value is undefined or formatting fails (default: empty string for undefined/Timestamp, original value for strings)
 * @returns Formatted date string (e.g., "January 15, 2024")
 */
export function formatDate(value: Timestamp | string | undefined, fallback?: string): string {
  if (!value) return fallback ?? '';

  try {
    const date = typeof value === 'string' ? new Date(value) : timestampDate(value);
    return date.toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    });
  } catch {
    // Return original string value if formatting fails, otherwise use fallback
    return typeof value === 'string' ? value : (fallback ?? '');
  }
}

/**
 * Formats a date with time (from Timestamp or string) to a localized date-time string.
 * @param value - The date value (Timestamp, string, or undefined)
 * @param fallback - The fallback string to return if value is undefined or formatting fails (default: empty string for undefined/Timestamp, original value for strings)
 * @returns Formatted date-time string (e.g., "January 15, 2024, 02:30 PM")
 */
/**
 * Returns a human-readable relative time duration string (e.g., "3 days", "2 years").
 * @param date - The date to compute the duration from
 * @returns Duration string, or empty string if date is undefined
 */
export function formatTimeAgo(date: Date | undefined): string {
  if (!date) return '';

  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMinutes = Math.floor(diffMs / (1000 * 60));
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));
  const diffYears = Math.floor(diffDays / 365);

  if (diffMinutes < 1) return 'just now';
  if (diffMinutes === 1) return '1 minute ago';
  if (diffMinutes < 60) return `${diffMinutes} minutes ago`;
  if (diffHours === 1) return '1 hour ago';
  if (diffHours < 24) return `${diffHours} hours ago`;
  if (diffDays === 1) return '1 day ago';
  if (diffDays < 365) return `${diffDays} days ago`;
  if (diffYears === 1) return '1 year ago';
  return `${diffYears} years ago`;
}

/** Date and time with the month abbreviated: "Aug 9, 2026 at 10:48 AM". For a
 *  column of timestamps, where a full month name makes every row a different
 *  width and the eye has nothing to line up on. */
export function formatShortDateTime(
  value: Timestamp | string | undefined,
  fallback?: string,
): string {
  if (!value) return fallback ?? '';

  try {
    const date = typeof value === 'string' ? new Date(value) : timestampDate(value);
    return date.toLocaleString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  } catch {
    return fallback ?? '';
  }
}

/** Just the clock time, for a timeline that already carries the date in its own
 *  column. */
export function formatTime(value: Timestamp | string | undefined, fallback?: string): string {
  if (!value) return fallback ?? '';

  try {
    const date = typeof value === 'string' ? new Date(value) : timestampDate(value);
    return date.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
  } catch {
    return fallback ?? '';
  }
}

export function formatDateTime(value: Timestamp | string | undefined, fallback?: string): string {
  if (!value) return fallback ?? '';

  try {
    const date = typeof value === 'string' ? new Date(value) : timestampDate(value);
    return date.toLocaleString('en-US', {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  } catch {
    // Return original string value if formatting fails, otherwise use fallback
    return typeof value === 'string' ? value : (fallback ?? '');
  }
}
