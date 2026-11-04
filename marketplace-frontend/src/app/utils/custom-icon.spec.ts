import { describe, expect, it } from 'vitest';
import { isCustomIcon } from './custom-icon';

describe('isCustomIcon', () => {
  it('accepts svg and png data URIs', () => {
    expect(isCustomIcon('data:image/svg+xml;base64,AA')).toBe(true);
    expect(isCustomIcon('data:image/png;base64,AA')).toBe(true);
  });

  it('rejects other values', () => {
    expect(isCustomIcon('data:image/svg+xml,%3Csvg%3E')).toBe(false);
    expect(isCustomIcon('data:image/png,AA')).toBe(false);
    expect(isCustomIcon('data:image/svg+xmlfoo')).toBe(false);
    expect(isCustomIcon('data:image/pngfoo;base64,AA')).toBe(false);
    expect(isCustomIcon('data:text/html;base64,AA')).toBe(false);
    expect(isCustomIcon('https://x/y.png')).toBe(false);
    expect(isCustomIcon('globe')).toBe(false);
    expect(isCustomIcon('')).toBe(false);
    expect(isCustomIcon(undefined)).toBe(false);
  });
});
