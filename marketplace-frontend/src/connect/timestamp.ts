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
