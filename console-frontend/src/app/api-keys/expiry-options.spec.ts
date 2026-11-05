import { defaultExpiry, expiryOptions, parseDuration } from './expiry-options';

const values = (maxExpiresIn: string) => expiryOptions(maxExpiresIn).map((option) => option.value);

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
    expect(values('8760h')).toEqual(['168h', '720h', '2160h', '8760h', 'custom']);
    expect(defaultExpiry('8760h')).toBe('8760h');
  });

  it('leaves out the presets above the maximum', () => {
    expect(values('720h')).toEqual(['168h', '720h', 'custom']);
    expect(defaultExpiry('720h')).toBe('720h');
  });

  it('adds a maximum that is no preset, in days when it is whole days', () => {
    expect(expiryOptions('2400h')).toContainEqual({ value: '2400h', label: '100 days' });
    expect(expiryOptions('1000h')).toContainEqual({ value: '1000h', label: '1000h' });
    expect(defaultExpiry('1000h')).toBe('1000h');
  });

  it('still offers the maximum when it is shorter than every preset', () => {
    expect(values('1h30m')).toEqual(['1h30m', 'custom']);
    expect(defaultExpiry('1h30m')).toBe('1h30m');
  });

  it('offers never, and defaults to it, when there is no maximum', () => {
    expect(values('')).toEqual(['168h', '720h', '2160h', '8760h', 'never', 'custom']);
    expect(defaultExpiry('')).toBe('never');
  });
});
