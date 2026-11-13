// Outside console/, which console.go embeds and serves to the browser.

import { describe, expect, test } from 'bun:test';
import { latestRead, selectedDiskCount } from '../console/_shared.js';

describe('selectedDiskCount', () => {
  test.each([
    // Not reconciled yet: nothing is counted, which is not zero.
    [undefined, undefined],
    [{}, undefined],
    // omitempty drops a zero count, so a reconciled pool without the field has none.
    [{ phase: 'Degraded' }, 0],
    [{ phase: 'Ready', selectedDiskCount: 2 }, 2],
  ])('%p -> %p', (status, want) => {
    expect(selectedDiskCount(status)).toBe(want);
  });
});

describe('latestRead', () => {
  test('only the newest read is current', () => {
    const nextRead = latestRead();
    const first = nextRead();
    expect(first()).toBe(true);
    const second = nextRead();
    expect(first()).toBe(false);
    expect(second()).toBe(true);
  });

  test('each page keeps its own count', () => {
    const a = latestRead();
    const b = latestRead();
    const readA = a();
    b();
    expect(readA()).toBe(true);
  });
});
