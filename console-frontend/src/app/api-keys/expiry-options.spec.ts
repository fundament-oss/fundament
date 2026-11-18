import {
  customExpiryMoment,
  defaultExpiry,
  durationUntil,
  expiryOptions,
  isoDate,
  lastExpiryDate,
  maximumLabel,
  parseDuration,
} from './expiry-options';

const values = (maxExpiresIn: string | null) =>
  expiryOptions(maxExpiresIn).map((option) => option.value);

const defaultFor = (maxExpiresIn: string | null) => defaultExpiry(expiryOptions(maxExpiresIn));

describe('parseDuration', () => {
  it('reads the durations the server writes', () => {
    expect(parseDuration('8760h')).toBe(8760 * 3_600_000);
    expect(parseDuration('1h30m')).toBe(90 * 60_000);
    expect(parseDuration('30s')).toBe(30_000);
    expect(parseDuration('500ms')).toBe(500);
    expect(parseDuration('1.5h')).toBe(90 * 60_000);
  });

  it('refuses what is not a duration', () => {
    expect(parseDuration('')).toBeNull();
    expect(parseDuration('10 days')).toBeNull();
    expect(parseDuration('7d')).toBeNull();
  });
});

describe('expiryOptions', () => {
  it('offers every preset up to a maximum of a year, and defaults to the year', () => {
    expect(values('8760h')).toEqual(['24h', '168h', '720h', '8760h', 'custom']);
    expect(defaultFor('8760h')).toBe('8760h');
  });

  it('leaves out the presets above the maximum', () => {
    expect(values('720h')).toEqual(['24h', '168h', '720h', 'custom']);
    expect(defaultFor('720h')).toBe('720h');
  });

  it('adds a maximum that is no preset, in days when it is whole days', () => {
    expect(expiryOptions('2400h')).toContainEqual({
      value: '2400h',
      label: '100 days',
      ms: 2400 * 3_600_000,
    });
    expect(expiryOptions('1000h')).toContainEqual({
      value: '1000h',
      label: '1000h',
      ms: 1000 * 3_600_000,
    });
    expect(defaultFor('1000h')).toBe('1000h');
  });

  it('still offers the maximum when it is shorter than every preset', () => {
    expect(values('1h30m')).toEqual(['1h30m', 'custom']);
    expect(defaultFor('1h30m')).toBe('1h30m');
  });

  it('offers never, and defaults to it, when there is no maximum', () => {
    expect(values('')).toEqual(['24h', '168h', '720h', '8760h', 'never', 'custom']);
    expect(defaultFor('')).toBe('never');
  });

  it('holds never back, and defaults to the longest preset, while the maximum is unknown', () => {
    expect(values(null)).toEqual(['24h', '168h', '720h', '8760h', 'custom']);
    expect(defaultFor(null)).toBe('8760h');
  });

  it('keeps the presets when the maximum cannot be read, and defaults to the longest', () => {
    expect(values('8760h0m0s0x')).toEqual(['24h', '168h', '720h', '8760h', 'custom']);
    expect(defaultFor('8760h0m0s0x')).toBe('8760h');
  });
});

describe('maximumLabel', () => {
  it('words the maximum like the options', () => {
    expect(maximumLabel('8760h')).toBe('1 year');
    expect(maximumLabel('2400h')).toBe('100 days');
    expect(maximumLabel('1h30m')).toBe('1h30m');
  });
});

describe('a custom expiry date', () => {
  // Local time, as the calendar is: the tests hold in any time zone.
  const now = new Date(2026, 10, 18, 14, 30, 0);

  it('reads and writes local calendar dates', () => {
    expect(isoDate(now)).toBe('2026-11-18');
    expect(isoDate(new Date(2027, 0, 5))).toBe('2027-01-05');
  });

  it('can fall no later than the day the maximum runs out', () => {
    expect(lastExpiryDate('8760h', now)).toBe('2027-11-18');
    expect(lastExpiryDate('720h', now)).toBe('2026-12-18');
    expect(lastExpiryDate('1h', now)).toBe('2026-11-18');
  });

  it('has no last day when there is no maximum, or none that can be read', () => {
    expect(lastExpiryDate('', now)).toBeNull();
    expect(lastExpiryDate(null, now)).toBeNull();
    expect(lastExpiryDate('7d', now)).toBeNull();
  });

  it('lasts until the end of the picked day', () => {
    expect(customExpiryMoment('2026-12-01', '8760h', now)).toEqual(
      new Date(2026, 11, 1, 23, 59, 59),
    );
    expect(customExpiryMoment('2030-01-01', '', now)).toEqual(new Date(2030, 0, 1, 23, 59, 59));
  });

  it('stops at the maximum when that runs out during the picked day', () => {
    expect(customExpiryMoment('2027-11-18', '8760h', now)).toEqual(
      new Date(now.getTime() + 8760 * 3_600_000),
    );
  });

  it('is sent as whole seconds from now, and not at all once it has passed', () => {
    expect(durationUntil(new Date(2026, 10, 18, 23, 59, 59), now)).toBe(
      `${9 * 3600 + 29 * 60 + 59}s`,
    );
    expect(durationUntil(new Date(now.getTime() + 8760 * 3_600_000), now)).toBe(`${8760 * 3600}s`);
    expect(durationUntil(now, now)).toBeNull();
    expect(durationUntil(new Date(2026, 10, 17, 23, 59, 59), now)).toBeNull();
  });
});
