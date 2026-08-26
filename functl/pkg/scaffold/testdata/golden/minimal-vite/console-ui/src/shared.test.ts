import { describe, expect, it } from 'vitest';

import { escapeHtml, formatAge, phase } from './shared.ts';

describe('escapeHtml', () => {
  it('escapes HTML metacharacters', () => {
    expect(escapeHtml('<script>&"\'')).toBe('&lt;script&gt;&amp;&quot;&#39;');
  });

  it('renders null and undefined as empty', () => {
    expect(escapeHtml(null)).toBe('');
    expect(escapeHtml(undefined)).toBe('');
  });
});

describe('formatAge', () => {
  it('is empty without a timestamp', () => {
    expect(formatAge(undefined)).toBe('');
  });

  it('scales the unit with the age', () => {
    const ago = (ms: number) => new Date(Date.now() - ms).toISOString();
    expect(formatAge(ago(5_000))).toBe('5s');
    expect(formatAge(ago(5 * 60_000))).toBe('5m');
    expect(formatAge(ago(5 * 3_600_000))).toBe('5h');
    expect(formatAge(ago(5 * 86_400_000))).toBe('5d');
  });
});

describe('phase', () => {
  it('falls back to a dash when status is absent', () => {
    expect(phase({})).toBe('—');
  });
});
