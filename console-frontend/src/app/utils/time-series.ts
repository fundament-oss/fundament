import { type Timestamp, timestampDate } from '@bufbuild/protobuf/wkt';

export interface TimeSeriesSample {
  timestamp?: Timestamp;
  value: number;
}

export interface AlignedTimeSeries {
  /** One moment per step across the whole range, in epoch milliseconds. */
  times: number[];
  /** Per input series, one value per moment; null where nothing was measured. */
  values: (number | null)[][];
}

/**
 * Lays the samples of one or more series out over the full range, one slot per step.
 *
 * Prometheus only returns the steps it has data for, so a 30-day window over a
 * cluster that is two days old comes back as two days of samples. Charting those
 * as-is stretches two days across the full width. Padding the missing steps with
 * null keeps the axis at the period that was asked for, and leaves a gap where
 * nothing was measured instead of drawing a line at 0 that was never there.
 *
 * The slots are anchored on the first sample rather than on rangeStart: the
 * samples sit on the backend's own step grid, whose start is not known here, and
 * anchoring on one of them keeps every sample exactly on a slot.
 */
export function alignToRange(
  series: TimeSeriesSample[][],
  rangeStartMs: number,
  rangeEndMs: number,
  stepMs: number,
): AlignedTimeSeries {
  const timed = series.map((samples) =>
    samples.flatMap((s) =>
      s.timestamp ? [{ t: timestampDate(s.timestamp).getTime(), value: s.value }] : [],
    ),
  );
  const firstTimes = timed.flatMap((samples) => (samples.length ? [samples[0].t] : []));
  if (firstTimes.length === 0 || stepMs <= 0) {
    return { times: [], values: series.map(() => []) };
  }

  const anchor = Math.min(...firstTimes);
  const slotOf = (t: number) => Math.round((t - anchor) / stepMs);
  const allTimes = timed.flatMap((samples) => samples.map((s) => s.t));
  // Widen to the samples themselves, so none falls off an edge when the range
  // and the backend's window disagree by a few seconds.
  const kMin = Math.min(Math.ceil((rangeStartMs - anchor) / stepMs), ...allTimes.map(slotOf));
  const kMax = Math.max(Math.floor((rangeEndMs - anchor) / stepMs), ...allTimes.map(slotOf));

  const count = kMax - kMin + 1;
  const times = Array.from({ length: count }, (_, i) => anchor + (kMin + i) * stepMs);
  const values = timed.map((samples) => {
    const slots: (number | null)[] = new Array(count).fill(null);
    samples.forEach((s) => {
      slots[slotOf(s.t) - kMin] = s.value;
    });
    return slots;
  });
  return { times, values };
}
