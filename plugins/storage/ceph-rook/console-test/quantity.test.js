// Outside console/, which console.go embeds and serves to the browser.

import { describe, expect, test } from 'bun:test';
import { humanizeQuantity } from '../console/_shared.js';

describe('humanizeQuantity', () => {
  test.each([
    // What the controller writes: BinarySI from an int64 byte count.
    ['21472739328', '19.9 GiB'],
    ['20478Mi', '19.9 GiB'],
    ['61434Mi', '59.9 GiB'],
    ['937692504Ki', '894.2 GiB'],
    ['20Gi', '20 GiB'],
    ['1.5Gi', '1.5 GiB'],
    ['0', '0 GiB'],
    // One decimal, truncated: 0.1 GiB is 107374182.4 bytes.
    ['107374182', '0 GiB'],
    ['107374183', '0.1 GiB'],
    // GiB below 1 TiB, TiB below 1 PiB, PiB above, all truncated.
    ['1099511627775', '1023.9 GiB'],
    ['1Ti', '1 TiB'],
    ['4000787030016', '3.6 TiB'], // "4 TB" HDD
    ['22000969973760', '20 TiB'], // "22 TB" HDD
    ['122880000000000', '111.7 TiB'], // "122.88 TB" NVMe
    ['1125899906842623', '1023.9 TiB'],
    ['1Pi', '1 PiB'],
    ['2949120000000000', '2.6 PiB'], // 24 x 122.88 TB
    ['1Ei', '1024 PiB'],
    // The rest of the Quantity grammar.
    ['500M', '0.4 GiB'],
    ['1e9', '0.9 GiB'],
    ['1E3', '0 GiB'],
    ['1.5e10', '13.9 GiB'],
    ['+1Gi', '1 GiB'],
    ['.5Gi', '0.5 GiB'],
    ['-1Gi', '-1 GiB'],
    ['-512', '0 GiB'],
    ['1500m', '0 GiB'],
    ['5u', '0 GiB'],
    ['5n', '0 GiB'],
    ['2147483648000m', '2 GiB'],
  ])('%s -> %s', (quantity, want) => {
    expect(humanizeQuantity(quantity)).toBe(want);
  });

  test('accepts a number', () => {
    expect(humanizeQuantity(1073741824)).toBe('1 GiB');
  });

  test.each(['20 Gi', '20GB', '1K', '1Ki1', '1e', 'garbage', '1.2.3'])('shows %p as written', (quantity) => {
    expect(humanizeQuantity(quantity)).toBe(quantity);
  });

  test.each([undefined, null, ''])('shows %p as absent', (quantity) => {
    expect(humanizeQuantity(quantity)).toBe('—');
  });
});
