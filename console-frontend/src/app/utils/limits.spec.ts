import { pairLimited, positive, toInt } from './limits';

describe('limit helpers', () => {
  it('reads a proto zero as "no limit set"', () => {
    expect(positive(0)).toBeUndefined();
    expect(positive(undefined)).toBeUndefined();
    expect(positive(4)).toBe(4);
  });

  it('keeps only whole positive numbers from a field', () => {
    expect(toInt('12')).toBe(12);
    expect(toInt('12.7')).toBe(12);
    expect(toInt('')).toBeUndefined();
    expect(toInt('0')).toBeUndefined();
    expect(toInt('-3')).toBeUndefined();
    expect(toInt('abc')).toBeUndefined();
  });

  it('counts a pair as limited when either half is set', () => {
    expect(pairLimited(undefined, undefined)).toBe(false);
    expect(pairLimited(64, undefined)).toBe(true);
    expect(pairLimited(undefined, 128)).toBe(true);
    expect(pairLimited(64, 128)).toBe(true);
  });
});
