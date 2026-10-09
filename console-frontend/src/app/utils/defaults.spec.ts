import { INT32_MAX, pairSet, positive, toInt } from './defaults';

describe('defaults helpers', () => {
  it('reads a proto zero as "no default set"', () => {
    expect(positive(0)).toBeUndefined();
    expect(positive(undefined)).toBeUndefined();
    expect(positive(4)).toBe(4);
  });

  it('keeps only whole positive numbers from a field', () => {
    expect(toInt('12')).toBe(12);
    expect(toInt('12.7')).toBe(12);
    expect(toInt(INT32_MAX)).toBe(INT32_MAX);
    expect(toInt(INT32_MAX + 1)).toBeUndefined();
    expect(toInt('')).toBeUndefined();
    expect(toInt('0')).toBeUndefined();
    expect(toInt('-3')).toBeUndefined();
    expect(toInt('abc')).toBeUndefined();
  });

  it('counts a pair as set when either half is set', () => {
    expect(pairSet(undefined, undefined)).toBe(false);
    expect(pairSet(64, undefined)).toBe(true);
    expect(pairSet(undefined, 128)).toBe(true);
    expect(pairSet(64, 128)).toBe(true);
  });
});
