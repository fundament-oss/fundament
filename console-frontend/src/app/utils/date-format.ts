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
