import { timestampFromMs } from '@bufbuild/protobuf/wkt';
import { alignToRange } from './time-series';

const HOUR = 3_600_000;
const START = Date.UTC(2026, 8, 1);

const sample = (ms: number, value: number) => ({ timestamp: timestampFromMs(ms), value });

describe('alignToRange', () => {
  it('pads the start of the range when data only covers its end', () => {
    const end = START + 10 * HOUR;
    const { times, values } = alignToRange(
      [[sample(end - HOUR, 1), sample(end, 2)]],
      START,
      end,
      HOUR,
    );
    expect(times.length).toBe(11);
    expect(times[0]).toBe(START);
    expect(times[10]).toBe(end);
    expect(values[0]).toEqual([null, null, null, null, null, null, null, null, null, 1, 2]);
  });

  it('leaves a gap where steps are missing in the middle', () => {
    const { values } = alignToRange(
      [[sample(START, 1), sample(START + 3 * HOUR, 4)]],
      START,
      START + 3 * HOUR,
      HOUR,
    );
    expect(values[0]).toEqual([1, null, null, 4]);
  });

  it('puts series of different lengths on the same slots', () => {
    const { times, values } = alignToRange(
      [
        [sample(START, 1), sample(START + HOUR, 2), sample(START + 2 * HOUR, 3)],
        [sample(START + 2 * HOUR, 30)],
      ],
      START,
      START + 2 * HOUR,
      HOUR,
    );
    expect(times.length).toBe(3);
    expect(values[0]).toEqual([1, 2, 3]);
    expect(values[1]).toEqual([null, null, 30]);
  });

  it('returns nothing when there are no samples at all', () => {
    const { times, values } = alignToRange([[], []], START, START + 10 * HOUR, HOUR);
    expect(times).toEqual([]);
    expect(values).toEqual([[], []]);
  });

  it('keeps the slots on the samples when the range is not aligned to them', () => {
    // The backend's grid starts 20 minutes past the hour; the range asked for does not.
    const offset = 20 * 60_000;
    const { times, values } = alignToRange(
      [[sample(START + 2 * HOUR + offset, 5)]],
      START,
      START + 3 * HOUR,
      HOUR,
    );
    expect(times).toEqual([START + offset, START + HOUR + offset, START + 2 * HOUR + offset]);
    expect(values[0]).toEqual([null, null, 5]);
  });

  it('keeps a sample that falls just outside the range', () => {
    const { times, values } = alignToRange(
      [[sample(START + 2 * HOUR + 5_000, 7)]],
      START,
      START + 2 * HOUR,
      HOUR,
    );
    expect(times.length).toBe(3);
    expect(values[0]).toEqual([null, null, 7]);
  });
});
