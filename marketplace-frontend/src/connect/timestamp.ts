import { type Timestamp, timestampDate } from '@bufbuild/protobuf/wkt';

/**
 * Formats a proto timestamp as an ISO date (YYYY-MM-DD), which is how every
 * date in this app is rendered and sorted.
 *
 * Timestamps are message fields, so they stay optional even under implicit
 * field presence: an unset one means "has not happened yet" (never published,
 * not yet reviewed) and renders as an empty string.
 */
export default function toIsoDate(timestamp?: Timestamp): string {
  return timestamp ? timestampDate(timestamp).toISOString().slice(0, 10) : '';
}

/**
 * The same instant as milliseconds since the epoch, for ordering.
 *
 * Sorting on the rendered `toIsoDate` string is what this exists to avoid: a
 * day is a coarse key, so records from the same day compare equal and the sort
 * silently falls back to whatever order they arrived in. An unset timestamp
 * sorts oldest, matching "has not happened yet".
 */
export function toEpochMs(timestamp?: Timestamp): number {
  return timestamp ? timestampDate(timestamp).getTime() : 0;
}
